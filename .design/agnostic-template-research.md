# Harness-Agnostic Templates: Feasibility Research

## Status: Research
## Date: 2026-02-16
## Related: `decouple-templates.md`, `versioned-settings-refactor.md`

---

## 1. Executive Summary

This document explores the feasibility, challenges, and approaches to transforming Scion's template system from a harness-coupled model (each template is 1:1 with a harness) to a harness-agnostic model where a template defines the *role* of an agent (agent instructions, skills, resources) and is combined with a harness-config at creation time.

The refactor is feasible but touches many layers of the system. The key challenges are: (1) separating harness-specific home directory content from portable template content, (2) rethinking how embedded defaults work, (3) adapting the provisioning flow to a two-phase "compose template + apply harness-config" model, and (4) organizing harness-config directories on disk to co-locate configuration and home directory base files.

Three approaches are evaluated. The recommended path is a **Hybrid of Approaches A and B**: harness-configs stored on disk at `~/.scion/harness-configs/<name>/` provide the default base layer, while templates may optionally include a `harness-configs/` directory for template-specific overrides or defaults.

---

## 2. Current State Analysis

### 2.1 The Template ↔ Harness Coupling

Today, templates and harnesses are tightly coupled at every level:

**Source embeds** (`pkg/config/embeds/`): Each harness has its own embed directory containing a complete agent home layout. There is no shared "template" concept that spans harnesses. Some common files exist (`.tmux.conf`, `.bashrc`, `.zshrc`) but they are incomplete — `.zshrc` needs to be added to the common set.

```
pkg/config/embeds/
├── common/                    # Shared shell/terminal config
│   ├── .tmux.conf
│   ├── bashrc                 # Common shell setup
│   └── zshrc                  # Common zsh setup (needs adding)
├── claude/                    # Claude-coupled template
│   ├── scion-agent.yaml       # declares harness: claude
│   ├── .claude.json           # Claude Code state file
│   ├── settings.json          # Claude Code settings (hooks, env, permissions)
│   ├── claude.md              # Agent instructions (sciontool hooks)
│   └── bashrc                 # harness-specific aliases
├── gemini/                    # Gemini-coupled template
│   ├── scion-agent.yaml       # declares harness: gemini + auth config
│   ├── settings.json          # Gemini CLI settings (hooks, model, security)
│   ├── gemini.md              # Agent instructions (sciontool hooks)
│   ├── system_prompt.md       # Placeholder
│   └── bashrc                 # harness-specific aliases
├── codex/                     # Codex-coupled template
│   ├── scion-agent.yaml       # declares harness: codex
│   ├── config.toml            # Codex config (model, approval policy)
│   └── bashrc                 # harness-specific aliases
└── opencode/                  # OpenCode-coupled template
    ├── scion-agent.yaml       # declares harness: opencode
    └── opencode.json          # OpenCode config (theme)
```

**Template seeding** (`Harness.SeedTemplateDir()`): Each harness implementation controls its own template directory layout. The `SeedCommonFiles()` function in `pkg/config/init.go` provides minimal shared infrastructure but the harness drives the process.

**Template discovery** (`pkg/config/templates.go`): Templates are discovered by name (e.g., `gemini`, `claude`) and their `scion-agent.yaml` declares which harness they bind to. The template name conventionally matches the harness name.

**Agent provisioning** (`pkg/agent/provision.go`): The provisioning flow copies `template/home/` into `agent/home/`, then calls `harness.Provision()` to do harness-specific setup (e.g., Claude updates `.claude.json` with workspace paths).

### 2.2 What Is Actually Harness-Specific vs Portable

To determine feasibility, we must classify every piece of template content:

#### Harness-Specific Content (must remain with harness)

| Content | Harness | Purpose |
|---------|---------|---------|
| `.claude.json` | Claude | CLI state file (onboarding, project trust, MCP servers) |
| `.claude/settings.json` | Claude | Claude Code settings (env vars, hooks, permissions, auto-updater) |
| `.gemini/settings.json` | Gemini | Gemini CLI settings (hooks, model, yolo mode, auth, security) |
| `.codex/config.toml` | Codex | Codex config (model, approval policy, workspace trust) |
| `.config/opencode/opencode.json` | OpenCode | OpenCode config (theme) |
| Auth discovery logic | All | Each harness finds credentials differently |
| Container command | All | Each harness has different CLI invocation (`gemini --yolo`, `claude --no-chrome`, etc.) |
| Interrupt key | All | Claude uses Escape, others use C-c |
| Hook dialect | All | `sciontool hook --dialect=claude` vs `--dialect=gemini` |

#### Abstract Harness Capabilities

While many commands are harness-specific, several common behaviors can be defined at an abstract interface level when harnesses support them:

| Capability | Description |
|-----------|-------------|
| Resume/Continue | Mechanism to resume the harness after it pauses or completes a turn |
| Message delivery | Getting the harness a message, with optional interrupt of current work |
| Telemetry on/off | Controlling whether the harness sends telemetry, with potential sub-settings |
| Supported dialect | Which sciontool hook dialect the harness uses |

These abstract capabilities allow the template and agent system to reason about harness behavior without coupling to specific implementation details.

#### Portable Content (should belong to template)

| Content | Current Location | Purpose |
|---------|-----------------|---------|
| Agent instructions | `.claude/claude.md`, `.gemini/gemini.md` | Agent role definition and sciontool integration instructions |
| `.bashrc` (common portion) | Per-harness embeds | Common shell aliases and setup |
| `.zshrc` | Needs adding to common | Common zsh setup |
| `.tmux.conf` | `common/` | Terminal multiplexer config (already shared) |
| `scion-agent.yaml` fields: `env`, `volumes`, `resources`, `services`, `max_turns`, `max_duration` | Template | Agent runtime configuration |
| Telemetry settings | Per-harness | Abstract on/off control that maps to harness-specific config |

#### Resolved: Agent Instructions File

All harnesses now support `agents.md` as a common agent instruction file. This content should live in the template as the portable agent instructions. At agent creation time, the harness either:
- **(Preferred)** Uses a harness-specific setting to read `agents.md` from the standard location
- Falls back to a symlink from the harness-expected location to `agents.md`

This resolves the previous ambiguity around `claude.md` / `gemini.md` — these files are **not** system prompts but agent instruction files. The content is identical across harnesses and belongs in the template.

**Important distinction:** Agent instructions (`agents.md`) are not the same as system prompts. Agent instructions are Scion-specific guidance (sciontool hooks, status reporting). System prompts define the agent's role and capabilities. Both are portable, but they serve different purposes and may be delivered via different mechanisms.

### 2.3 Key Observation

The current `claude.md` and `gemini.md` files are **identical** — they both contain the same sciontool status-reporting instructions. This validates the premise that templates should be harness-agnostic: the actual agent instructions are already portable, only the delivery mechanism differs. With `agents.md` support across all harnesses, this is now a solved problem.

---

## 3. What a Harness-Agnostic Template Looks Like

### 3.1 Target Template Structure

A harness-agnostic template defines the agent's *purpose*, not its *execution mechanics*:

```yaml
# .scion/templates/code-reviewer/scion-agent.yaml
schema_version: "1"
name: code-reviewer
description: "Thorough code review agent with security focus"

# Portable agent configuration
agent_instructions: agents.md    # path relative to template dir
system_prompt: system-prompt.md  # path relative to template dir (optional)

# Optional: default harness-config to use if user doesn't specify one
default_harness_config: gemini

# Optional: template-specific harness-config overrides
# (see harness-configs/ directory below)

env:
  REVIEW_STRICTNESS: high

resources:
  requests:
    cpu: "500m"
    memory: "512Mi"
  limits:
    cpu: "2"
    memory: "4Gi"

max_turns: 100
max_duration: "4h"

services:
  - name: browser
    command: ["chromium", "--headless"]
    ready_check:
      type: tcp
      target: "localhost:9222"
```

The YAML format should support inline plain text content where appropriate, allowing template authors to embed short prompts or instructions directly rather than requiring separate files for everything.

```
.scion/templates/code-reviewer/
├── scion-agent.yaml             # Agent/template configuration (harness-agnostic)
├── agents.md                    # Portable agent instructions (sciontool hooks etc.)
├── system-prompt.md             # Portable system prompt (optional)
├── home/                        # Portable home directory content (optional)
│   └── .config/
│       └── lint-rules/          # Custom config files
└── harness-configs/             # Optional: template-specific harness-config overrides
    ├── gemini/
    │   └── config.yaml          # Override model, hooks, etc. for this template
    └── claude/
        └── config.yaml          # Override settings for this template
```

**Note on naming:** We retain `scion-agent.yaml` rather than introducing `scion-template.yaml`. A template defines an agent, and after provisioning, the `scion-agent.yaml` in the agent directory represents the composed configuration (template settings + profile env vars + harness-config). Keeping the same filename creates a natural lineage from template to provisioned agent.

### 3.2 How It Combines with Harness-Config

At `scion create`, the user specifies a template and optionally a harness-config:

```bash
# Explicit harness-config
scion create my-agent --template code-reviewer --harness-config gemini

# Uses template's default_harness_config if set, otherwise falls back to
# the default_harness_config from settings.yaml
scion create my-agent --template code-reviewer
```

Settings must include a `default_harness_config` field to replace the notion of "default_template" implying a harness. The `default_template` setting is retained but no longer implies a harness — it only selects which portable template to use.

The system resolves:
1. **Template** → `code-reviewer` (provides agent instructions, system prompt, env, resources, services)
2. **Harness-config** → resolved from: CLI `--harness-config` flag → template's `default_harness_config` → settings `default_harness_config`
3. **Harness-config on disk** → `~/.scion/harness-configs/<name>/` (provides home dir base files, runtime config)
4. **Harness** → derived from harness-config's `harness` field (provides behavior, auth, command, hooks)

The agent home directory is assembled by composing:
1. Harness-config base layer (harness-specific config files from `harness-configs/<name>/home/`)
2. Template-specific harness-config overrides (if present in template's `harness-configs/` dir)
3. Template home content (portable files)
4. Agent instructions injection
5. Common files (`.tmux.conf`, `.bashrc`, `.zshrc`)

---

## 4. Challenges

### 4.1 Harness-Specific Home Directory Files

**The core challenge.** Each harness requires specific files in the agent home directory:

- Claude needs `.claude.json` and `.claude/settings.json`
- Gemini needs `.gemini/settings.json`
- Codex needs `.codex/config.toml`
- OpenCode needs `.config/opencode/opencode.json`

These files are currently embedded in templates. In a harness-agnostic world, these files must come from somewhere else.

**Decision:** These base files are associated with the **harness-config**, not the harness itself. A harness is a convenience label for common behavior and code-level defaults; the harness-config is the user-facing, customizable entity that carries both runtime parameters and home directory base files.

Storage on disk:

```
~/.scion/harness-configs/<name>/
├── config.yaml          # Runtime parameters (image, user, model, args, env, volumes, auth)
└── home/                # Base home directory files
    ├── .claude.json     # (for claude harness-configs)
    ├── .claude/
    │   └── settings.json
    └── .bashrc          # Harness-specific shell additions
```

This co-locates configuration and base files, keeping them associated with each other under a single named reference. The `config.yaml` replaces what was previously stored inline in `settings.yaml` under `harness_configs`.

### 4.2 Agent Instructions Delivery

All harnesses now support `agents.md` as a common agent instruction file. The delivery mechanism is:

1. **(Preferred)** The harness-specific configuration includes a setting that tells the harness CLI to read `agents.md` from a known location (e.g., the agent home root or workspace root)
2. **(Fallback)** At agent creation, a symlink is created from the harness-expected location to the canonical `agents.md` location

For system prompts (distinct from agent instructions), each harness reads from different locations:
- Claude: Via CLI flags or configuration
- Gemini: `.gemini/system_prompt.md`
- Codex: No system prompt mechanism currently
- OpenCode: No system prompt mechanism currently

A harness-agnostic template may have a `system-prompt.md`. The harness must know how to deliver it to the right location. This is a **transform** operation: take portable content and place it where the harness expects it.

### 4.3 Settings/Hooks Configuration

The `settings.json` files for Claude and Gemini contain critical integration hooks:

**Claude** (`settings.json`):
```json
{
  "hooks": {
    "SessionStart": [{"command": "sciontool hook --dialect=claude"}],
    "PostToolUse": [{"command": "sciontool hook --dialect=claude"}],
    ...
  },
  "env": { "CLAUDE_CODE_USE_VERTEX": "1", ... },
  "permissions": { "allow": ["*"] }
}
```

**Gemini** (`settings.json`):
```json
{
  "hooks": {
    "SessionStart": [{"command": "sciontool hook --dialect=gemini"}],
    "BeforeAgent": [{"command": "sciontool hook --dialect=gemini"}],
    ...
  },
  "yolo": true,
  "model": { "name": "gemini-3-flash-preview" }
}
```

These are harness-specific in format and content. They cannot be made portable. They live in the harness-config's `home/` directory.

**Additional complication:** Harness-configs need to be able to inject additional custom hooks. At agent creation time, the harness-specific settings files will need a **deep merge** for hooks — combining the base hooks from the harness-config's home files with any additional hooks specified in the harness-config's `config.yaml` or the template's harness-config overrides.

### 4.4 SeedTemplateDir Inversion

Currently the harness controls template creation via `SeedTemplateDir()`. With harness-agnostic templates, there are two distinct operations:

1. **Template creation**: Creates the portable template structure (agent instructions, system prompt, env, resources). This is harness-independent.
2. **Harness-config setup**: Creates the harness-config directory with home files and `config.yaml`. This is template-independent.

The current `SeedTemplateDir()` conflates both. It must be split.

### 4.5 Keeping `scion-agent.yaml`

The template configuration file remains `scion-agent.yaml`. The `harness` field is removed entirely — harness binding is determined exclusively by the resolved harness-config at agent creation time.

A `scion-agent.yaml` without a `harness` field is a harness-agnostic template. Templates that previously declared `harness: claude` (or similar) with that field present are invalid under the new format and will produce an error prompting the user to update their template.

The `scion-agent.yaml` file serves as the base for the `scion-agent.yaml` in the provisioned agent directory, which represents the full composed configuration (template + harness-config + profile overrides).

### 4.6 Embed Restructuring

The current `pkg/config/embeds/` directory is organized by harness. In a harness-agnostic world, we need to separate:

- **Harness-config base files**: Moved to `pkg/harness/` package — each harness owns its embedded default home files and config
- **Default templates**: Harness-agnostic templates that ship with the binary, remain in `pkg/config/embeds/`
- **Common files**: Shared across all (`.tmux.conf`, `.bashrc`, `.zshrc`), remain in `pkg/config/embeds/common/`

### 4.7 Harness.Provision() Must Handle Missing Files

Currently `Provision()` assumes the template has already placed harness-specific files (e.g., Claude's `Provision()` reads an existing `.claude.json` to update workspace paths). In the new model, `Provision()` must handle the case where these files don't exist yet and create them from embedded defaults.

### 4.8 Hub Template Storage

In hosted mode, templates are stored in the Hub. The Hub API and storage must support both harness-agnostic templates and harness-config definitions. Templates stored in the Hub include their optional `harness-configs/` overrides directory.

---

## 5. Approaches

### Approach A: Template Composition with Adapter Directories

**Already explored in `decouple-templates.md`.** Each template contains a `base/` directory and per-harness adapter directories.

```
code-reviewer/
├── scion-agent.yaml
├── base/
│   ├── home/
│   │   └── .config/...
│   └── agents.md
└── adapters/
    ├── claude/
    │   ├── home/.claude/...
    │   └── adapter.yaml
    └── gemini/
        ├── home/.gemini/...
        └── adapter.yaml
```

**Pros:**
- Self-contained: everything needed is in the template directory
- Explicit harness compatibility declared via adapter presence
- Transform system allows deriving harness-specific files from base content

**Cons:**
- Duplicates harness-specific files across every template
- Each new template must include adapters for every harness it supports
- Maintaining adapters becomes a burden — updating Claude's `settings.json` format requires updating it in every template
- Template authors need to understand harness internals to write adapters

**Verdict:** Attractive in theory but scales poorly. The adapter content (`.claude.json`, `settings.json`, hooks config) is effectively identical across all templates. Duplicating it in every template creates a maintenance burden.

### Approach B: Harness-Config Base Layers + Agnostic Templates

Separate the harness-specific home directory content into **harness-config directories** that are independent of templates. Templates contain only portable content. At agent creation, the system composes: harness-config base + template overlay.

**Pros:**
- Harness-specific content maintained once, not duplicated per template
- Templates are purely about agent purpose — easy to author, share, version
- Clean separation of concerns
- Harness-config files can be updated independently

**Cons:**
- Templates cannot customize harness-specific behavior (e.g., a "researcher" template that wants a specific model)
- No way to ship template-specific harness defaults

**Verdict:** Good foundation but lacks the ability for templates to influence harness-config settings.

### Approach A+B Hybrid: Harness-Config Base Layers with Optional Template Overrides (Recommended)

Combine the strengths of both approaches. Harness-configs stored on disk at `~/.scion/harness-configs/<name>/` provide the default base layer. Templates may **optionally** include a `harness-configs/` directory that provides template-specific overrides or defaults for specific harnesses.

#### Harness-Config Directories on Disk

Each harness-config is a named directory containing both its runtime configuration and base home files:

```
~/.scion/
├── harness-configs/              # Harness-config base layers
│   ├── claude/                   # Default claude harness-config
│   │   ├── config.yaml           # Runtime params: image, user, model, env, auth
│   │   └── home/                 # Base home directory files
│   │       ├── .claude.json
│   │       ├── .claude/
│   │       │   └── settings.json
│   │       └── .bashrc
│   ├── gemini/                   # Default gemini harness-config
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .gemini/
│   │       │   └── settings.json
│   │       └── .bashrc
│   ├── codex/
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .codex/
│   │       │   └── config.toml
│   │       └── .bashrc
│   ├── opencode/
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .config/
│   │       │   └── opencode/
│   │       │       └── opencode.json
│   │       └── .bashrc
│   └── gemini-experimental/      # Custom user-defined harness-config variant
│       ├── config.yaml           # harness: gemini, but with different model/settings
│       └── home/
│           └── .gemini/
│               └── settings.json
├── templates/                    # Harness-agnostic templates
│   ├── code-reviewer/
│   │   ├── scion-agent.yaml
│   │   ├── agents.md
│   │   ├── system-prompt.md
│   │   └── harness-configs/      # Optional: template-specific overrides
│   │       └── gemini/
│   │           └── config.yaml   # e.g., set a specific model for this template
│   └── researcher/
│       ├── scion-agent.yaml
│       ├── agents.md
│       └── system-prompt.md
└── settings.yaml                 # References harness-configs by name, no longer inline
```

Grove-level overrides are also supported: `.scion/harness-configs/<name>/` at the project level takes precedence over the global `~/.scion/harness-configs/<name>/`.

#### Template-Specific Harness-Config Overrides

Templates can optionally include a `harness-configs/` directory. This preserves one of the key advantages of the current 1:1 model — a template author can specify a particular model, custom hooks, or other harness-specific settings that make sense for that template's purpose:

```
templates/researcher/
├── scion-agent.yaml
│   # default_harness_config: gemini
├── agents.md
├── system-prompt.md
└── harness-configs/
    └── gemini/
        └── config.yaml     # Override: model: gemini-3-pro (research needs stronger model)
```

When the template specifies a `default_harness_config` and also has a matching override in `harness-configs/`, the template's override is deep-merged on top of the base harness-config during composition.

#### Composition at Agent Creation

```
Agent Home = Harness-Config Base Layer (from ~/.scion/harness-configs/<name>/home/)
           + Template Harness-Config Overrides (if template has harness-configs/<name>/)
           + Template Home (if any)
           + Agent Instructions Injection
           + Common Files (.tmux.conf, .bashrc, .zshrc)
           + Settings/Profile Overrides
```

Steps:
1. Copy harness-config base home → agent home
2. Deep-merge template harness-config overrides (if present) — hooks are merged, not replaced
3. Copy template home → agent home (overlay, template files win on conflict)
4. Inject agent instructions into harness-specific location (or configure harness to read `agents.md`)
5. Inject system prompt if provided
6. Copy common files (`.tmux.conf`, `.bashrc`, `.zshrc`)
7. Call `Harness.Provision()` for dynamic setup (e.g., workspace paths in `.claude.json`)

#### Agent Instruction & System Prompt Injection

The harness interface gains new methods:

```go
type Harness interface {
    // ... existing methods ...

    // InjectAgentInstructions configures the harness to read agent instructions
    // from the standard agents.md location, or creates the necessary symlinks/copies.
    InjectAgentInstructions(agentHome string, content []byte) error

    // InjectSystemPrompt writes the system prompt content to the
    // harness-specific location in the agent home directory.
    InjectSystemPrompt(agentHome string, content []byte) error
}
```

Implementations for `InjectAgentInstructions`:
- **Claude**: Configures `.claude/settings.json` to read `agents.md`, or writes to `{agentHome}/AGENTS.md`
- **Gemini**: Configures setting to read `agents.md`, or symlinks `{agentHome}/.gemini/gemini.md` → `agents.md`
- **Generic**: Writes to `{agentHome}/agents.md`

Implementations for `InjectSystemPrompt`:
- **Claude**: Writes to `{agentHome}/.claude/CLAUDE.md`
- **Gemini**: Writes to `{agentHome}/.gemini/system_prompt.md`
- **Codex**: No-op (or future: write to `.codex/instructions.md`)
- **OpenCode**: No-op (or future: writes config section)
- **Generic**: Writes to `{agentHome}/.scion/system_prompt.md`

#### Embed Restructuring

```
pkg/config/embeds/
├── common/
│   ├── .tmux.conf
│   ├── bashrc                     # Common shell setup
│   └── zshrc                      # Common zsh setup
├── templates/                     # Default agnostic templates
│   ├── default/
│   │   ├── scion-agent.yaml
│   │   ├── agents.md
│   │   └── system-prompt.md
│   └── (future: code-reviewer/, researcher/, etc.)
└── default_settings.yaml

pkg/harness/
├── claude/
│   └── embeds/                    # Claude harness-config defaults
│       ├── config.yaml
│       ├── .claude.json
│       ├── settings.json          # Goes to home/.claude/settings.json
│       └── bashrc                 # Harness-specific shell additions
├── gemini/
│   └── embeds/
│       ├── config.yaml
│       ├── settings.json          # Goes to home/.gemini/settings.json
│       └── bashrc
├── codex/
│   └── embeds/
│       ├── config.yaml
│       ├── config.toml            # Goes to home/.codex/config.toml
│       └── bashrc
└── opencode/
    └── embeds/
        ├── config.yaml
        └── opencode.json          # Goes to home/.config/opencode/opencode.json
```

**Pros:**
- Harness-specific content maintained once per harness-config, not duplicated per template
- Templates are purely about agent purpose — easy to author, share, version
- Templates can optionally customize harness behavior for their specific role
- Clean separation: harness embeds live with harness code in `pkg/harness/`
- Co-located config + home files under `harness-configs/` on disk
- Harness-config variants are easy to create (e.g., `gemini-experimental`)

**Cons:**
- More concepts: harness-configs (on disk), templates, composition
- Deep merge logic for hooks adds complexity
- Template authors who want harness-specific overrides still need some harness knowledge

### Approach C: Harness-Generated Home (No Persistent Base Layer)

Instead of persisting harness base files on disk, the harness generates them on-the-fly during provisioning. The `Provision()` method becomes the sole source of harness-specific files.

```go
// Harness.Provision() now creates all harness-specific files
func (c *ClaudeCode) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
    // 1. Create .claude.json from embedded defaults
    claudeJSON := generateClaudeJSON(agentWorkspace)
    os.WriteFile(filepath.Join(agentHome, ".claude.json"), claudeJSON, 0644)

    // 2. Create .claude/settings.json from embedded defaults
    settings := generateClaudeSettings()
    os.WriteFile(filepath.Join(agentHome, ".claude", "settings.json"), settings, 0644)

    // 3. Create .bashrc
    os.WriteFile(filepath.Join(agentHome, ".bashrc"), bashrcContent, 0644)

    return nil
}
```

**Pros:**
- Simplest model: no new directories, no harness-configs to manage on disk
- Harness code is the single source of truth for its files
- Users never need to know about harness base files
- Updating harness defaults = updating Go code, automatically picked up

**Cons:**
- Harness-specific files are not user-customizable (they're generated from Go code, not disk files)
- Users can't customize the Claude `settings.json` hooks or Gemini `settings.json` model without post-provisioning edits
- Breaks the current model where users can edit template files to customize harness behavior
- No way for templates to influence harness-specific settings

---

## 6. Detailed Comparison

| Aspect | A: Adapters in Template | B: Harness-Config Base Layers | A+B Hybrid | C: Generated |
|--------|------------------------|------------------------------|------------|-------------|
| Template authoring complexity | High (must write adapters) | Low (portable only) | Low (portable, optional overrides) | Low (portable only) |
| Harness file duplication | Per-template duplication | Single copy per harness-config | Single copy + optional template overrides | None (in code) |
| User customizability | Per-template customization | Per-harness-config customization | Per-harness-config + per-template | Not customizable |
| Template-specific harness tuning | Full control | None | Selective override via deep merge | None |
| Maintenance burden | High (N templates × M harnesses) | Low (M harness-configs) | Low (M base + optional overrides) | Low (in code) |
| Disk footprint | Large | Moderate | Moderate | Minimal |
| New template creation | Must create adapters | Just write portable content | Write portable + optional overrides | Just write portable content |
| Updating harness defaults | Update in every template | Update once in harness-config | Update once, overrides preserved | Update Go code |
| Sharing templates | Self-contained but large | Small, portable | Small, portable + optional config | Small, portable |

---

## 7. Implementation Sketch (Hybrid A+B)

### 7.1 Phase 1: Harness-Config Directory Infrastructure

**Goal:** Create the on-disk `harness-configs/` structure and move harness embeds to `pkg/harness/`.

1. Create `pkg/harness/<harness>/embeds/` directories by extracting harness-specific home files from current `pkg/config/embeds/<harness>/` directories.

2. Define `config.yaml` schema for harness-configs:
   ```yaml
   harness: string           # Which harness implementation to use
   image: string             # Container image
   user: string              # Container user
   model: string             # Default model
   args: []string            # Additional CLI arguments
   env: map[string]string    # Environment variables
   volumes: []VolumeMount    # Additional volume mounts
   auth: AuthConfig          # Authentication configuration
   hooks: map[string][]Hook  # Additional hooks to deep-merge
   ```

3. Add `SeedHarnessConfig(harnessConfigName, targetDir string, force bool)` — seeds harness-config directories into `~/.scion/harness-configs/<name>/` from embedded defaults.

4. Implement `scion init --machine` for global setup of `~/.scion/`, including:
   - Seeding all default harness-configs
   - Setting up `settings.yaml` with `default_harness_config`
   - Any other machine-level one-time setup

5. Update `scion init` (project-level) to be lightweight:
   - Creates `.scion/` directory in the project
   - Does NOT populate harness-config defaults or templates
   - Errors with guidance if `~/.scion/` is not set up, prompting user to run `scion init --machine` first

### 7.2 Phase 2: Agnostic Template Support

**Goal:** Support `scion-agent.yaml` without a `harness` field as the new agnostic template format.

1. Update `scion-agent.yaml` schema — remove `harness` field entirely. Add new fields:
   ```yaml
   schema_version: "1"
   name: string              # Required
   description: string
   agent_instructions: string  # relative path to agents.md
   system_prompt: string       # relative path to system prompt file
   default_harness_config: string  # fallback harness-config name
   env: map[string]string
   volumes: []VolumeMount
   resources: ResourceSpec
   max_turns: int
   max_duration: string
   services: []ServiceSpec
   ```

2. Update `Template.LoadConfig()` — detect and validate new format. Templates with a `harness` field present are invalid and produce an error directing the user to update.

3. Add `InjectAgentInstructions()` and `InjectSystemPrompt()` to the `Harness` interface.

4. Implement both methods for each harness:
   - Claude: configure to read `agents.md`, write system prompt to `.claude/CLAUDE.md`
   - Gemini: configure to read `agents.md`, write system prompt to `.gemini/system_prompt.md`
   - Generic: write both to standard locations

5. Update template discovery to handle agnostic templates.

### 7.3 Phase 3: Composition in Provisioning

**Goal:** Wire the composition flow into `ProvisionAgent()`.

1. Harness-config resolution order:
   - CLI `--harness-config` flag
   - Template's `default_harness_config` field
   - Settings `default_harness_config` field
   - Error if none resolved

2. Composition flow:
   ```go
   func composeAgentHome(harnessConfigName, agentHome, templateDir string) error {
       // 1. Resolve harness-config directory
       harnessConfigDir := findHarnessConfig(harnessConfigName, grovePath, globalPath)

       // 2. Copy harness-config base home → agent home
       util.CopyDir(filepath.Join(harnessConfigDir, "home"), agentHome)

       // 3. Apply template harness-config overrides (deep merge for hooks)
       templateOverrides := filepath.Join(templateDir, "harness-configs", harnessConfigName)
       if exists(templateOverrides) {
           applyHarnessConfigOverrides(agentHome, harnessConfigDir, templateOverrides)
       }

       // 4. Copy template home (overlay, template wins on conflict)
       templateHome := filepath.Join(templateDir, "home")
       if exists(templateHome) {
           util.CopyDirMerge(templateHome, agentHome)
       }

       // 5. Copy common files
       copyCommonFiles(agentHome)  // .tmux.conf, .bashrc, .zshrc

       // 6. Inject agent instructions
       h := harness.New(resolvedHarnessName)
       if instructionsPath := template.AgentInstructionsPath(); instructionsPath != "" {
           content, _ := os.ReadFile(instructionsPath)
           h.InjectAgentInstructions(agentHome, content)
       }

       // 7. Inject system prompt
       if promptPath := template.SystemPromptPath(); promptPath != "" {
           content, _ := os.ReadFile(promptPath)
           h.InjectSystemPrompt(agentHome, content)
       }

       return nil
   }
   ```

3. Implement deep merge logic for harness-specific settings files:
   - Hooks arrays are appended/merged, not replaced
   - Scalar values from the override take precedence
   - This enables template-specific and harness-config-specific hook injection

### 7.4 Phase 4: Embed and Init Restructuring

**Goal:** Restructure embeds and update seeding logic.

1. Move harness-specific embed files from `pkg/config/embeds/<harness>/` to `pkg/harness/<harness>/embeds/`.

2. Update `SeedHarnessConfig()` implementations to use the new embed locations.

3. Add default agnostic template(s) to `pkg/config/embeds/templates/`.

4. Add `.zshrc` to `pkg/config/embeds/common/`.

5. Update `scion init --machine` to call `SeedHarnessConfig()` for all known harnesses.

6. Update `scion init` (project-level) to be lightweight — no harness-config seeding.

### 7.5 Phase 5: Default Template and Settings Migration

**Goal:** Ship default agnostic templates and migrate settings.

1. Create a `default` agnostic template with generic agent instructions.

2. Add `default_harness_config` to `settings.yaml` schema.

3. Migrate `harness_configs` entries from `settings.yaml` inline format to on-disk `harness-configs/<name>/config.yaml` format.

4. New templates can be created as agnostic: `scion template create my-template`.

5. Provide a `scion harness-config reset <name>` command to restore defaults from embedded files.

---

## 8. Key Design Decisions

### 8.1 Template config file naming

**Decision:** Keep `scion-agent.yaml`. The `harness` field is removed entirely. A template defines an agent, and the same file format carries forward into provisioned agents as the composed configuration.

**Rationale:** Introducing `scion-template.yaml` would create two parallel config formats. Since `scion-agent.yaml` in a provisioned agent will represent the composed result of template + harness-config + profile, keeping the same filename provides a natural lineage.

### 8.2 Where do harness-config base files live?

**Decision:** On disk at `~/.scion/harness-configs/<name>/` (global) and `.scion/harness-configs/<name>/` (grove-level override). Each directory contains `config.yaml` plus a `home/` subdirectory with base files.

**Rationale:** Co-locating `config.yaml` and `home/` keeps all aspects of a harness-config associated. Named references (`--harness-config gemini`) map directly to directory names. Grove-level overrides allow per-project customization.

### 8.3 Should harness-config runtime params stay in settings.yaml?

**Decision:** No. Move harness-config definitions from inline `settings.yaml` entries to on-disk `harness-configs/<name>/config.yaml`. The `settings.yaml` retains only a `default_harness_config` reference and any other non-harness-config-specific settings.

**Rationale:** Mixing home directory files and runtime parameters in different places is confusing. The on-disk directory approach creates a single source of truth per harness-config.

### 8.4 How does a user customize harness-config files?

**Decision:** Users edit files directly in `~/.scion/harness-configs/<name>/`. For example, to add custom hooks to Claude's settings.json, they edit `~/.scion/harness-configs/claude/home/.claude/settings.json`. To change the default model, they edit `~/.scion/harness-configs/claude/config.yaml`.

A reset mechanism is provided: `scion harness-config reset claude` restores defaults from embedded files.

### 8.5 What happens during initialization?

**Decision:** Split initialization into two tiers:

- **`scion init --machine`** (global, first-time): Sets up `~/.scion/`, seeds all default harness-configs, creates `settings.yaml` with `default_harness_config`, seeds default template(s). This is a prerequisite.
- **`scion init`** (project-level): Creates `.scion/` in the project. Lightweight — does not populate harness-config defaults or global templates. If `~/.scion/` does not exist, errors with guidance to run `scion init --machine` first.

### 8.6 What about legacy templates?

**Decision:** Breaking change. Templates with a `harness` field in `scion-agent.yaml` are invalid under the new format. The system will error with a clear message: "Invalid template: 'harness' field is no longer supported in scion-agent.yaml. Remove it and use --harness-config to specify the harness."

Templates must also have required front-matter fields (e.g., `name`). Invalid or incomplete templates produce an error.

**Rationale:** The project is in alpha. Maintaining backward compatibility with a format that is being fundamentally redesigned adds complexity for limited benefit. A clean break with clear error messages is preferable.

### 8.7 Harness-config resolution for `scion create`

**Decision:** The harness-config is resolved from (in priority order):
1. CLI `--harness-config` flag
2. Template's `default_harness_config` field in `scion-agent.yaml`
3. Settings `default_harness_config` field
4. Error if none resolved

The resolved harness-config name maps to a directory, from which the `harness` type is read from `config.yaml`. There is no `harness` field in templates or in the provisioned agent's `scion-agent.yaml` — the harness is always derived from the harness-config.

---

## 9. Impact on Existing Code

### 9.1 Files That Must Change

| File | Change | Scope |
|------|--------|-------|
| `pkg/api/harness.go` | Add `InjectAgentInstructions()`, `InjectSystemPrompt()` to interface | Small |
| `pkg/harness/*.go` | Implement new interface methods; add `embeds/` directories per harness | Medium |
| `pkg/config/init.go` | Add `SeedHarnessConfig()`, split `InitProject()`/`InitGlobal()`, add `InitMachine()` | Medium |
| `pkg/config/embeds/` | Remove per-harness directories (moved to `pkg/harness/`); add `templates/`, update `common/` with `.zshrc` | Medium |
| `pkg/config/templates.go` | Remove `harness` field handling; validate new `scion-agent.yaml` format; support `default_harness_config` and `agent_instructions` fields | Medium |
| `pkg/agent/provision.go` | Implement composition logic (harness-config base + template overlay + deep merge) | Large |
| `cmd/create.go`, `cmd/start.go` | Add `--harness-config` flag; implement resolution chain; remove legacy template flow | Medium |
| `cmd/init.go` | Add `--machine` flag; implement two-tier initialization | Medium |
| `pkg/config/settings_v1.go` | Add `default_harness_config` field; remove inline `harness_configs` (moved to disk) | Medium |
| `pkg/config/harness_config.go` | New: load harness-config from on-disk directory structure | Medium |

### 9.2 Files That Should NOT Change

| File | Reason |
|------|--------|
| `pkg/agent/run.go` | The `Start()` flow doesn't need to know about template type — by start time, the agent home is already composed |
| `pkg/runtime/*.go` | Runtime is agnostic to template model — it just runs a container |
| `cmd/attach.go`, `cmd/list.go` | Post-creation commands are template-model-agnostic |

---

## 10. Migration Path

Since this is a breaking change (alpha project), the migration is direct rather than phased:

### Step 1: Infrastructure
- Create on-disk `harness-configs/` directory structure
- Move harness embeds to `pkg/harness/<harness>/embeds/`
- Implement `SeedHarnessConfig()` and `InitMachine()`
- Add `InjectAgentInstructions()` and `InjectSystemPrompt()` to harness interface
- Add `.zshrc` to common files

### Step 2: Template Format
- Update `scion-agent.yaml` schema (remove `harness`, add `agent_instructions`, `system_prompt`, `default_harness_config`)
- Error on templates with legacy `harness` field
- Implement template validation for required fields

### Step 3: Composition & Provisioning
- Implement harness-config resolution chain
- Build composition flow (harness-config base + template overlay)
- Implement deep merge for hooks in harness-specific settings files
- Wire into `ProvisionAgent()`

### Step 4: Commands & Settings
- Add `--harness-config` flag to `scion create`
- Add `default_harness_config` to settings
- Implement `scion init --machine`
- Make `scion init` (project-level) lightweight
- Add `scion harness-config reset <name>`

### Step 5: Default Templates
- Create default agnostic template with `agents.md`
- Ship default harness-configs for all supported harnesses
- Remove legacy per-harness templates from `pkg/config/embeds/`

---

## 11. Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Composition order bugs (harness-config base vs template override vs common) | Files in wrong location or overwritten | Comprehensive tests with all harness × template combinations |
| Deep merge of hooks produces invalid config | Agent hooks don't fire correctly | Validate merged settings against harness-specific schema |
| Agent instructions delivery fails for new harnesses | Agent runs without instructions | `InjectAgentInstructions` returns error; provisioning fails early |
| Users customize harness-configs then `scion init --machine --force` overwrites them | Lost customizations | Backup before overwrite, warn user, add `--no-harness-configs` flag |
| Agnostic template + missing harness-config = confusing error | User doesn't understand what to specify | Clear error message: "No harness-config resolved. Specify --harness-config, set default_harness_config in the template, or set default_harness_config in settings." |
| Hub storage format for harness-configs | Complex Hub API | Unified storage format that handles templates and harness-configs |
| Harness-config base files drift from embedded defaults | Stale config | `scion harness-config check` to compare on-disk files with embedded defaults |
| Breaking change invalidates existing user templates | User friction on upgrade | Clear error messages with migration guidance; provide `scion template migrate` helper |

---

## 12. Recommendation

**Implement the Hybrid A+B approach (Harness-Config Base Layers with Optional Template Overrides)** as a clean break from the current coupled model.

This approach:
- Cleanly separates concerns (template = agent purpose, harness-config = execution mechanics)
- Avoids the per-template duplication problem of pure Approach A
- Preserves user customizability through on-disk harness-config directories
- Allows templates to optionally tune harness-specific settings for their role
- Co-locates `config.yaml` and `home/` files under named harness-config directories
- Moves harness embeds to `pkg/harness/` where they conceptually belong
- Properly distinguishes `scion init` (project) from `scion init --machine` (global)
- Distinguishes agent instructions (`agents.md`) from system prompts

The key insight enabling this approach: harness-specific home directory content is **identical across all templates** using the same harness-config. One base layer per harness-config is sufficient. But templates occasionally need to influence harness-specific settings (e.g., model selection for a research template), which the optional `harness-configs/` override directory enables.

### Open Questions

1. **Inline YAML content**: Should `scion-agent.yaml` support inline multi-line text for short agent instructions or system prompts (e.g., `agent_instructions: |` with inline content), in addition to file path references? This would be convenient for simple templates.

2. **Harness-config variants**: How should custom harness-config variants (e.g., `gemini-experimental`) be created? Should there be a `scion harness-config create` command that copies from an existing harness-config, or is manual directory creation sufficient?

3. **Abstract harness capabilities**: The abstract capabilities (resume/continue, message delivery, telemetry) need formal interface definition. Should these be part of the `Harness` interface or a separate `HarnessCapabilities` type?

4. **Settings.yaml migration**: When moving `harness_configs` from inline in `settings.yaml` to on-disk directories, should there be an automatic migration path or just documentation?

5. **Hub harness-config storage**: How should the Hub store and distribute harness-configs? Should they be per-grove, per-user, or global to the Hub instance?
