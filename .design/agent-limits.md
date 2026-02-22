# Design: Agent Limits (max_turns & max_duration)

## 1. Overview

Agents need configurable limits on how long they run and how many LLM turns they consume. These limits act as safety guardrails preventing runaway agents from consuming unbounded resources. When a limit is exceeded, the agent must transition to a distinct `LIMITS_EXCEEDED` state with clear logging, then exit cleanly.

**Scope:** This document covers the enforcement of `max_turns` and `max_duration` settings, the new `LIMITS_EXCEEDED` state, and the container exit behavior when limits are hit.

## 2. Current State

### Configuration

Both settings are already defined in the configuration layer:

- **Schema** (`agent-v1.schema.json`): `max_turns` (integer, min 1) and `max_duration` (string, pattern `^[0-9]+(s|m|h)$`), both gated on `schema_version: "1"`.
- **Go struct** (`pkg/api/types.go`): `ScionConfig.MaxTurns` and `ScionConfig.MaxDuration` with a `ParseMaxDuration()` helper.
- **Environment injection** (`pkg/agent/run.go:280-286`): Both are injected into the container as `SCION_MAX_TURNS` and `SCION_MAX_DURATION`.

### Duration Enforcement (Host-Side, Incomplete)

A rudimentary duration timer exists in `pkg/agent/run.go:542-558`. It spawns a goroutine on the host that calls `rt.Stop()` after the configured duration. This approach has significant drawbacks:

1. **No status update** -- the agent's state is not set before the container is killed, so the exit reason is opaque to users and the Hub.
2. **No agent-side logging** -- nothing is written to `agent.log` explaining why the agent stopped.
3. **No Hub notification** -- the Hub sees the agent transition from `running`/`idle` directly to `stopped`, with no indication that a limit was the cause.
4. **Host-only** -- only works when the host process is alive; does not work in hosted/Kubernetes environments where the runtime broker may not keep the goroutine running.

### Turn Enforcement (Not Implemented)

No turn counting or enforcement exists. The session parser (`pkg/sciontool/hooks/session/parser.go`) tracks `TurnCount` for metrics, but this is a post-hoc analysis tool, not a live enforcement mechanism.

## 3. Design

### 3.1. Principle: Enforce Inside the Container

All limit enforcement moves into `sciontool`, which runs as PID 1 inside the container. This is the correct enforcement point because:

- **sciontool is already the supervisor**: It manages the child process lifecycle, handles signals, and controls container exit. Limit enforcement is a natural extension of this role.
- **sciontool already receives hook events**: The harness (Claude Code, Gemini CLI) sends events to `sciontool hook` on every turn, tool call, and session event. Turn counting piggybacks on this existing event stream.
- **sciontool already manages status**: It writes `agent-info.json`, logs to `agent.log`, and reports to the Hub. Limit-exceeded reporting uses the same channels.
- **Works everywhere**: Inside the container, enforcement is runtime-agnostic. It works identically on Docker, Kubernetes, and Apple Virtualization.

The existing host-side `startDurationTimer` in `pkg/agent/run.go` should be removed once sciontool enforcement is in place.

### 3.2. New Agent State: `LIMITS_EXCEEDED`

A new terminal state is added alongside `COMPLETED` and `ERROR`:

```go
// In pkg/sciontool/hooks/types.go
StateLimitsExceeded AgentState = "LIMITS_EXCEEDED"
```

```go
// In pkg/sciontool/hub/client.go
StatusLimitsExceeded AgentStatus = "limits_exceeded"
```

This state is **sticky** (like `COMPLETED`), meaning subsequent events from the dying harness process cannot overwrite it. The `isStickyStatus` function in `handlers/status.go` must include `LIMITS_EXCEEDED`.

### 3.3. Duration Enforcement

#### Mechanism

When `sciontool init` starts, it reads `SCION_MAX_DURATION` from the environment and starts an internal timer. When the timer fires:

1. Log the event to `agent.log`.
2. Set agent status to `LIMITS_EXCEEDED` in `agent-info.json`.
3. Report `limits_exceeded` to the Hub (if configured) with a descriptive message.
4. Send `SIGTERM` to the child process group (the harness).
5. Wait for the configured grace period (same as normal shutdown).
6. If the child has not exited, send `SIGKILL`.
7. `sciontool` exits with a **distinct exit code** (see Section 3.5).

#### Timer Start Point

The timer starts when the child process (harness) is successfully launched -- after `post-start` hooks complete. This means `max_duration` measures the agent's active working time, not container boot overhead.

#### Implementation Location

The duration timer lives in `cmd/sciontool/commands/init.go`, integrated into the existing `runInit` flow. It is implemented as an additional case in the `select` that currently waits on `exitChan`:

```go
// Pseudocode for the wait loop in runInit()
var durationTimer <-chan time.Time
if maxDur := parseDurationEnv(); maxDur > 0 {
    t := time.NewTimer(maxDur)
    defer t.Stop()
    durationTimer = t.C
}

select {
case result := <-exitChan:
    // Normal child exit (existing behavior)
case <-durationTimer:
    // Max duration exceeded -- initiate limit-exceeded shutdown
    handleLimitsExceeded("duration", fmt.Sprintf("max_duration of %s exceeded", os.Getenv("SCION_MAX_DURATION")))
}
```

### 3.4. Turn Enforcement

#### What Counts as a Turn

A "turn" is one full cycle of the LLM receiving input and producing a response. In the normalized event model, this corresponds to a pair of `agent-start` / `agent-end` events. The turn counter increments on each `agent-end` event (indicating the LLM has completed a response).

For harnesses that don't emit `agent-start`/`agent-end` (or emit them inconsistently), `model-end` is used as a fallback signal, since it directly indicates an LLM inference has completed.

#### Mechanism

Turn counting is implemented as a new handler registered in the hook event pipeline. Since `sciontool hook` is invoked as a separate process for each event (it is not a long-running daemon), the turn count must be persisted to disk between invocations.

**Turn state file**: `~/agent-limits.json`

```json
{
  "turn_count": 17,
  "max_turns": 50,
  "started_at": "2026-02-22T10:30:00Z"
}
```

When a turn-incrementing event is received and the count meets or exceeds `max_turns`:

1. Write a clear log entry to `agent.log`.
2. Set agent status to `LIMITS_EXCEEDED` in `agent-info.json`.
3. Report `limits_exceeded` to the Hub (if configured).
4. Send `SIGTERM` to the harness process by signaling PID 1 (sciontool init).

#### Signaling the Init Process

The `sciontool hook` process needs to tell the init process (PID 1) to begin shutdown. This is done by sending `SIGUSR1` to PID 1. The init process registers a `SIGUSR1` handler that initiates the same limit-exceeded shutdown sequence used for duration limits.

The reason for using a signal rather than having the hook process directly kill the harness: the init process owns the child lifecycle and the graceful shutdown sequence. The hook process should only request shutdown, not perform it.

```
┌──────────────────┐     SIGUSR1      ┌──────────────────┐
│  sciontool hook   │ ──────────────▶  │  sciontool init   │
│  (turn counter)   │                  │  (PID 1)          │
│                   │                  │                    │
│  Detects limit    │                  │  Receives signal   │
│  Sets status      │                  │  Logs reason       │
│  Logs event       │                  │  SIGTERM → child   │
│  Reports to Hub   │                  │  Waits grace       │
│  Sends SIGUSR1    │                  │  Exits             │
└──────────────────┘                  └──────────────────┘
```

#### Initialization

On `pre-start` or `post-start`, the init process initializes `agent-limits.json` with the configured values from the environment, setting `turn_count` to 0. This ensures the state file exists before the first hook event arrives.

### 3.5. Exit Codes

When sciontool exits due to a limit being exceeded, it uses a distinct exit code so that the host-side orchestrator (scion CLI or runtime broker) can distinguish limit-exceeded exits from normal completion or errors:

| Exit Code | Meaning |
|-----------|---------|
| 0         | Normal exit (harness exited successfully) |
| 1         | Error (harness crashed or sciontool error) |
| 10        | Limits exceeded (max_turns or max_duration) |

The exit code is propagated through the container runtime and is available to the host via `scion list` or the Hub API.

### 3.6. Logging

All limit-related events produce structured log entries in `agent.log` using the existing `log` package. The log entries must be unambiguous about what happened and why.

#### When Duration Limit is Hit

```
2026-02-22 14:30:00 [sciontool] [INFO] [LIMITS_EXCEEDED] Agent stopped: max_duration of 2h exceeded (started at 2026-02-22 12:30:00)
```

#### When Turn Limit is Hit

```
2026-02-22 14:30:00 [sciontool] [INFO] [LIMITS_EXCEEDED] Agent stopped: max_turns of 50 exceeded (completed 50 turns)
```

#### When Neither Limit is Configured

No limit-related logging occurs. The agent runs until the harness exits naturally.

### 3.7. Hub Reporting

When reporting to the Hub, the status update includes both the new status and a descriptive message:

```go
hubClient.UpdateStatus(ctx, hub.StatusUpdate{
    Status:  hub.StatusLimitsExceeded,
    Message: "max_duration of 2h exceeded",
})
```

This allows the Hub UI and API consumers to display the reason the agent stopped. A new `ReportLimitsExceeded` convenience method is added to the Hub client alongside the existing `ReportError`, `ReportStopped`, etc.

### 3.8. Interaction with Existing States

The `LIMITS_EXCEEDED` state has specific interactions with the status system:

- **Sticky**: Once set, it cannot be overwritten by normal event-driven updates (same as `COMPLETED`). This prevents the harness's dying events from overwriting the limit status.
- **Overrides `COMPLETED`**: If a harness happens to report task completion in the same moment a limit fires, `LIMITS_EXCEEDED` takes priority. The limit is the authoritative reason for shutdown.
- **Does not override `ERROR`**: If the agent is already in an error state, the limit status is not applied. The original error is more important to preserve.
- **Hub shutdown sequence**: After `LIMITS_EXCEEDED` is set and the child exits, the normal shutdown sequence (`shutting_down` → `stopped`) still runs on the Hub side. The `LIMITS_EXCEEDED` status is preserved in `agent-info.json` for the local `scion list` display.

## 4. Implementation Plan

### Phase 1: State and Status Infrastructure

1. Add `StateLimitsExceeded` to `pkg/sciontool/hooks/types.go`.
2. Add `StatusLimitsExceeded` to `pkg/sciontool/hub/client.go` with a `ReportLimitsExceeded` method.
3. Update `isStickyStatus` in `handlers/status.go` to include `LIMITS_EXCEEDED`.
4. Add a `sciontool status limits_exceeded` subcommand (for manual testing and potential future use by custom harnesses).

### Phase 2: Duration Enforcement in sciontool init

1. In `cmd/sciontool/commands/init.go`, read `SCION_MAX_DURATION` and start a timer after post-start hooks complete.
2. Add `SIGUSR1` handler to `init.go` that triggers the limit-exceeded shutdown path.
3. Implement `handleLimitsExceeded(limitType, message string)` that performs the status update, logging, Hub reporting, and child termination sequence.
4. Exit with code 10 on limit-exceeded shutdown.

### Phase 3: Turn Enforcement via Hook Handler

1. Create `pkg/sciontool/hooks/handlers/limits.go` containing a `LimitsHandler` that:
   - Reads `SCION_MAX_TURNS` from the environment on construction.
   - Maintains turn count in `~/agent-limits.json`.
   - Increments on `agent-end` or `model-end` events.
   - When the limit is reached: updates status, logs, reports to Hub, sends `SIGUSR1` to PID 1.
2. Register `LimitsHandler` in the hook event pipeline in `cmd/sciontool/commands/hook.go`.
3. Initialize `agent-limits.json` during `post-start` in `init.go`.

### Phase 4: Remove Host-Side Timer

1. Remove `startDurationTimer` from `pkg/agent/run.go`.
2. Remove the call site in the `Run` function.
3. The env var injection (`SCION_MAX_TURNS`, `SCION_MAX_DURATION`) remains -- these are how the configuration reaches sciontool.

### Phase 5: Host-Side Status Display

1. Update `scion list` / `scion look` to recognize and display the `LIMITS_EXCEEDED` state clearly (distinct from `COMPLETED` or `ERROR`).
2. Update the Hub UI (if applicable) to display limit-exceeded agents with appropriate messaging.

## 5. Configuration Examples

### In scion-agent.yaml (Template)

```yaml
schema_version: "1"
max_turns: 100
max_duration: "4h"
```

### Per-Agent Override

```yaml
# .scion/agents/my-agent/scion-agent.yaml
schema_version: "1"
max_turns: 25
max_duration: "30m"
```

### No Limits (Default)

When neither `max_turns` nor `max_duration` is configured, no enforcement occurs. The agent runs until the harness exits naturally or is stopped manually.

## 6. Testing Strategy

### Unit Tests

- **LimitsHandler**: Test turn counting, file persistence, limit detection, and SIGUSR1 signaling (mock the signal send).
- **Status updates**: Verify `LIMITS_EXCEEDED` is sticky and interacts correctly with other states.
- **Duration parsing**: Verify `SCION_MAX_DURATION` env var is parsed correctly for various formats.
- **Exit codes**: Verify sciontool exits with code 10 on limit-exceeded.

### Integration Tests

- **Duration limit**: Start an agent with `max_duration: "5s"`, verify it stops with `LIMITS_EXCEEDED` status and exit code 10.
- **Turn limit**: Start an agent with `max_turns: 3`, verify it stops after 3 turns with correct status.
- **No limits**: Start an agent with no limits configured, verify it runs and exits normally.
- **Hub reporting**: With a mock Hub endpoint, verify the `limits_exceeded` status is reported with the correct message.

### Manual Verification

- `scion start --max-duration 1m <agent>` → agent stops after 1 minute, `scion list` shows `LIMITS_EXCEEDED`.
- `scion start --max-turns 5 <agent>` → agent stops after 5 turns, `agent.log` contains the limits-exceeded entry.
- `scion look <agent>` shows the limit-exceeded reason clearly.

## 7. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Hook process crashes before sending SIGUSR1 (turn limit not enforced) | Duration limit acts as a backstop. Both limits should be configured for defense-in-depth. |
| Race between turn limit and duration limit firing simultaneously | The `handleLimitsExceeded` function is idempotent -- multiple calls result in the same outcome. The first to set `LIMITS_EXCEEDED` wins (sticky status). |
| `agent-limits.json` gets corrupted | Use atomic writes (write-to-temp + rename), matching the pattern used by `agent-info.json`. |
| Harness doesn't emit `agent-end` events consistently | Fall back to `model-end` events for turn counting. Log a warning if neither event type is received after the session starts. |
| Removing host-side timer before sciontool enforcement is deployed | Phase 4 (removal) depends on Phase 2 and 3 being deployed first. Both approaches can coexist temporarily -- the host-side timer acts as a fallback. |

## 8. Open Questions

### OQ1: What counts as a "turn"?

The current design increments the turn counter on `agent-end` events, where one "turn" is one full LLM response cycle (which may include many tool calls and sub-invocations). An alternative is counting `model-end` events, which correspond to raw LLM API calls.

- **`agent-end` (proposed):** Coarser granularity. A single turn in Claude Code can involve dozens of tool calls and sub-agent spawns. Better maps to the user-facing concept of "the agent did one thing." Gives the agent room to work within each turn.
- **`model-end`:** Finer granularity. Directly correlates with API cost. A `max_turns: 50` limit on `model-end` would be much more restrictive than the same limit on `agent-end`.

The right choice depends on whether the goal is **controlling cost** (prefer `model-end`) or **controlling autonomy scope** (prefer `agent-end`). A hybrid approach (expose both as `max_turns` and `max_model_calls`) adds configuration surface area.

**Decision needed:** Which event should increment the counter? Or should both be configurable?

### OQ2: Resume behavior -- do counters reset?

The design does not address what happens to `turn_count` and the duration timer when an agent is stopped and later resumed via `scion attach` or `scion start --resume`.

- **Option A: Reset on resume.** Each "run" gets a fresh budget. Simple, but allows indefinite total resource consumption across multiple resumes.
- **Option B: Accumulate across resumes.** The limits apply to the agent's total lifetime. Requires persisting the elapsed duration and turn count across container restarts. More complex, but provides a true total budget.
- **Option C: Reset turns, accumulate duration.** Turns reset because each resume typically brings a new task. Duration accumulates because wall-clock cost is cumulative. Pragmatic middle ground.

**Decision needed:** What is the expected lifecycle model for limits when agents are resumed?

### OQ3: Turn state storage -- separate file or extend agent-info.json?

The design introduces `~/agent-limits.json` for persisting turn count. An alternative is adding `turn_count` and `max_turns` fields directly to the existing `~/agent-info.json`.

- **Separate file (proposed):** Avoids lock contention between the status handler and limits handler (both do atomic read-modify-write). Clear separation of concerns.
- **Extend `agent-info.json`:** One fewer file to manage. Turn count is visible alongside status in a single read. But requires coordinating writes between two independent code paths, risking lost updates.

**Decision needed:** Separate file or shared file?

### OQ4: Exit code selection

The design proposes exit code `10` for limit-exceeded exits. This is arbitrary and should be validated against:

- Container runtime conventions (Docker uses 125-127 for internal errors, 128+N for signal exits).
- Harness exit codes (Claude Code and Gemini CLI may use specific codes).
- Any existing scion conventions for non-zero exits.

Codes 1-9 are generally safe for application use. Code 10 is unlikely to collide but has no particular semantic convention behind it.

**Decision needed:** Is exit code 10 acceptable, or should a different code be used?

### OQ5: Graceful vs immediate turn limit enforcement

When the turn limit is hit, the event arrives during a `sciontool hook` invocation -- meaning the harness is actively mid-execution. The current design sends `SIGUSR1` immediately, triggering `SIGTERM` to the harness.

- **Immediate (proposed):** Simple. The harness gets SIGTERM and has the grace period to clean up. But the current response is interrupted mid-stream.
- **Deferred/graceful:** Instead of signaling immediately, write a "limit reached" flag. The harness checks this flag (or sciontool injects a stop signal) at the next natural pause point (e.g., before the next prompt submission). The current response completes fully. Requires harness cooperation or a hook-level gate.

The immediate approach is simpler and sufficient for a first implementation. Graceful enforcement could be added later if mid-response termination causes problems (e.g., uncommitted work).

**Decision needed:** Is immediate termination acceptable for v1, with graceful as a future enhancement?
