---
title: Agent Credentials
description: Configuring LLM credentials for Scion agents across harnesses and deployment modes.
---

Scion automatically discovers and injects LLM credentials into agent containers at launch time. Each harness (Claude, Gemini, etc.) defines which credential types it accepts and in what priority order. You provide credentials via environment variables, credential files, or Hub secrets — Scion handles the rest.

## How It Works

When an agent starts, Scion runs a four-stage credential pipeline:

1. **Gather** — Scans your environment variables and well-known file paths for all available credentials.
2. **Overlay** — Applies any harness-specific settings overrides (e.g., Gemini auth mode from `settings.json`).
3. **Resolve** — The harness selects the best auth method from what's available, following its preference order.
4. **Validate** — Checks that the resolved credentials are complete and files exist on disk.

This pipeline works identically for local agents and hub-dispatched agents. For hub-dispatched agents, secrets are injected into the environment before the pipeline runs.

---

## Credentials by Harness

### Claude Code

Claude Code supports two auth methods, tried in this order:

| Priority | Method | What to Provide |
| :--- | :--- | :--- |
| 1 | **API Key** | `ANTHROPIC_API_KEY` environment variable |
| 2 | **Vertex AI** | `GOOGLE_APPLICATION_CREDENTIALS` + `GOOGLE_CLOUD_PROJECT` + `GOOGLE_CLOUD_REGION` |

**API Key** — the simplest option. Set the `ANTHROPIC_API_KEY` environment variable:

```bash
export ANTHROPIC_API_KEY=sk-ant-api01-...
scion start --harness claude my-agent
```

**Vertex AI** — uses Google Cloud's Vertex AI endpoint with Application Default Credentials:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=~/.config/gcloud/application_default_credentials.json
export GOOGLE_CLOUD_PROJECT=my-project
export GOOGLE_CLOUD_REGION=us-east5
scion start --harness claude my-agent
```

If ADC credentials exist at `~/.config/gcloud/application_default_credentials.json`, the file path is detected automatically — you only need to set the project and region.

### Gemini CLI

Gemini supports four auth methods. When no explicit mode is selected, Scion auto-detects in this order:

| Priority | Method | What to Provide |
| :--- | :--- | :--- |
| 1 | **API Key** | `GEMINI_API_KEY` or `GOOGLE_API_KEY` |
| 2 | **ADC** | `GOOGLE_APPLICATION_CREDENTIALS` (or default file path) |
| 3 | **OAuth** | OAuth credentials file at `~/.gemini/oauth_creds.json` |

**API Key**:

```bash
export GEMINI_API_KEY=AIza...
scion start --harness gemini my-agent
```

**Forcing a specific mode** — Gemini also supports explicit mode selection via the agent's `settings.json`. Set `security.auth.selectedType` to one of: `gemini-api-key`, `oauth-personal`, `vertex-ai`, or `compute-default-credentials`.

### Generic

The Generic harness uses a **passthrough** strategy — it injects all available credentials into the container without selecting a specific auth method. This is useful for custom harnesses that handle their own credential logic.

All API keys, project/region variables, and credential files that are present in your environment will be forwarded.

### OpenCode

| Priority | Method | What to Provide |
| :--- | :--- | :--- |
| 1 | **Anthropic API Key** | `ANTHROPIC_API_KEY` |
| 2 | **OpenAI API Key** | `OPENAI_API_KEY` |
| 3 | **Auth File** | Credentials at `~/.local/share/opencode/auth.json` |

### Codex

| Priority | Method | What to Provide |
| :--- | :--- | :--- |
| 1 | **Codex API Key** | `CODEX_API_KEY` |
| 2 | **OpenAI API Key** | `OPENAI_API_KEY` |
| 3 | **Auth File** | Credentials at `~/.codex/auth.json` |

---

## Configuration Methods

### Environment Variables

Set credentials as environment variables before starting agents. Scion scans for these variables automatically:

| Variable | Used By |
| :--- | :--- |
| `ANTHROPIC_API_KEY` | Claude, OpenCode, Generic |
| `GEMINI_API_KEY` | Gemini, Generic |
| `GOOGLE_API_KEY` | Gemini, Generic |
| `OPENAI_API_KEY` | OpenCode, Codex, Generic |
| `CODEX_API_KEY` | Codex, Generic |
| `GOOGLE_APPLICATION_CREDENTIALS` | Claude (Vertex), Gemini (ADC/Vertex), Generic |
| `GOOGLE_CLOUD_PROJECT` | Claude (Vertex), Gemini (Vertex), Generic |
| `GOOGLE_CLOUD_REGION` | Claude (Vertex), Gemini (Vertex), Generic |

Some variables support fallback names:

- `GOOGLE_CLOUD_PROJECT` ← `GCP_PROJECT` ← `ANTHROPIC_VERTEX_PROJECT_ID`
- `GOOGLE_CLOUD_REGION` ← `CLOUD_ML_REGION` ← `GOOGLE_CLOUD_LOCATION`

### Credential Files

Scion probes these well-known file paths and uses them if present:

| File Path | Credential Type | Used By |
| :--- | :--- | :--- |
| `~/.config/gcloud/application_default_credentials.json` | Google ADC | Claude (Vertex), Gemini, Generic |
| `~/.gemini/oauth_creds.json` | Gemini OAuth | Gemini, Generic |
| `~/.codex/auth.json` | Codex auth | Codex, Generic |
| `~/.local/share/opencode/auth.json` | OpenCode auth | OpenCode, Generic |

The ADC file is only used as a fallback when `GOOGLE_APPLICATION_CREDENTIALS` is not set as an environment variable.

### Hub Secrets

When using the Scion Hub, store credentials as secrets so they're automatically injected into agents at launch. See [Secret Management](/guides/secrets) for general secret management.

**API key secrets** (environment type):

```bash
# Anthropic
scion hub secret set ANTHROPIC_API_KEY sk-ant-api01-...

# Gemini
scion hub secret set GEMINI_API_KEY AIza...

# OpenAI / Codex
scion hub secret set OPENAI_API_KEY sk-...
```

**File-based secrets** (file type):

```bash
# Google ADC for Vertex AI
scion hub secret set --type file \
  --target ~/.config/gcloud/application_default_credentials.json \
  GOOGLE_APPLICATION_CREDENTIALS @~/.config/gcloud/application_default_credentials.json

# Gemini OAuth credentials
scion hub secret set --type file \
  --target ~/.gemini/oauth_creds.json \
  OAUTH_CREDS @~/.gemini/oauth_creds.json
```

**Project and region** (environment type):

```bash
scion hub secret set GOOGLE_CLOUD_PROJECT my-gcp-project
scion hub secret set GOOGLE_CLOUD_REGION us-east5
```

#### Hub Secret Reference

| Secret Name | Type | Target Path |
| :--- | :--- | :--- |
| `GEMINI_API_KEY` | environment | — |
| `GOOGLE_API_KEY` | environment | — |
| `ANTHROPIC_API_KEY` | environment | — |
| `OPENAI_API_KEY` | environment | — |
| `CODEX_API_KEY` | environment | — |
| `GOOGLE_CLOUD_PROJECT` | environment | — |
| `GOOGLE_CLOUD_REGION` | environment | — |
| `GOOGLE_APPLICATION_CREDENTIALS` | file | `~/.config/gcloud/application_default_credentials.json` |
| `OAUTH_CREDS` | file | `~/.gemini/oauth_creds.json` |
| `CODEX_AUTH` | file | `~/.codex/auth.json` |
| `OPENCODE_AUTH` | file | `~/.local/share/opencode/auth.json` |

---

## Troubleshooting

### "no valid auth method found"

Each harness produces this error when it can't find any usable credentials. The error message lists exactly what credentials are needed:

- **Claude**: Set `ANTHROPIC_API_KEY`, or provide `GOOGLE_APPLICATION_CREDENTIALS` + `GOOGLE_CLOUD_PROJECT` + `GOOGLE_CLOUD_REGION` for Vertex AI.
- **Gemini**: Set `GEMINI_API_KEY` or `GOOGLE_API_KEY`, provide `GOOGLE_APPLICATION_CREDENTIALS` for ADC, or set up OAuth at `~/.gemini/oauth_creds.json`.
- **OpenCode**: Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`, or provide credentials at `~/.local/share/opencode/auth.json`.
- **Codex**: Set `CODEX_API_KEY` or `OPENAI_API_KEY`, or provide credentials at `~/.codex/auth.json`.

### "auth validation failed: credential file does not exist"

A credential file was detected during gathering but no longer exists when the agent is about to launch. Ensure the file path is correct and the file hasn't been moved or deleted.

### "auth validation failed: env vars have empty values"

A credential environment variable is set but has an empty value. Check that your environment variables are properly exported with non-empty values.

### Vertex AI not activating for Claude

Claude's Vertex AI mode requires **all three** of these to be set:
- `GOOGLE_APPLICATION_CREDENTIALS` (or ADC file at default path)
- `GOOGLE_CLOUD_PROJECT`
- `GOOGLE_CLOUD_REGION`

If any one is missing, Claude falls back to API key mode. If `ANTHROPIC_API_KEY` is also missing, the agent will fail to start.

### Gemini using wrong auth mode

Gemini auto-detects credentials in priority order (API key → ADC → OAuth). To force a specific mode, set `security.auth.selectedType` in the agent's settings. See [Templates & Harnesses](/guides/templates) for how to configure harness settings.
