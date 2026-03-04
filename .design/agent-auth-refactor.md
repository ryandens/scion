# Agent Auth Refactor

## Status: In Progress (Phase 1-4 Complete)

## Problem Statement

The current harness authentication system has grown organically and suffers from several issues:

1. **Fragmented auth discovery**: Each harness implements `DiscoverAuth()` independently, reading from different env vars, files, and settings with inconsistent logic. Some harnesses (Codex, OpenCode) also read env vars directly in `GetEnv()` rather than through `AuthConfig`, bypassing the auth abstraction.

2. **Incomplete `AuthConfig` struct**: The struct is missing fields that harnesses already consume (`CodexAPIKey`, `GoogleCloudRegion`). Some fields are obsolete (`VertexAPIKey` duplicates `GoogleAPIKey`). Claude's Vertex auth support isn't represented at all.

3. **Divergent local vs hub paths**: Local provisioning uses `DiscoverAuth() → GetEnv()/PropagateFiles()`, while hub-dispatched provisioning uses `ResolvedSecrets + env-gather`. These paths have different validation, different env var coverage, and different failure modes. The `AuthConfig` struct is essentially unused in the hub path.

4. **No auth validation before container launch**: The system can launch containers with incomplete auth and only discover the problem when the harness fails at runtime. The `RequiredEnvKeys()` method exists but only covers the env-gather path.

5. **File path normalization is ad-hoc**: `GoogleAppCredentials`, `OAuthCreds`, `OpenCodeAuthFile`, `CodexAuthFile` all store host paths but need to be translated to container paths. Each harness does this translation differently in `PropagateFiles()` and `GetVolumes()`.

## Current Architecture

### AuthConfig Struct (`pkg/api/types.go:299-310`)

```go
type AuthConfig struct {
    GeminiAPIKey         string  // env: GEMINI_API_KEY
    GoogleAPIKey         string  // env: GOOGLE_API_KEY
    VertexAPIKey         string  // env: VERTEX_API_KEY (obsolete, redundant with GoogleAPIKey)
    GoogleAppCredentials string  // env: GOOGLE_APPLICATION_CREDENTIALS, or ~/.config/gcloud/application_default_credentials.json
    GoogleCloudProject   string  // env: GOOGLE_CLOUD_PROJECT, ANTHROPIC_VERTEX_PROJECT_ID
    OAuthCreds           string  // file: ~/.gemini/oauth_creds.json
    AnthropicAPIKey      string  // env: ANTHROPIC_API_KEY
    OpenCodeAuthFile     string  // file: ~/.local/share/opencode/auth.json
    CodexAuthFile        string  // file: ~/.codex/auth.json
    SelectedType         string  // Gemini-specific auth mode selector
}
```

### Auth Flow (Local)

```
cmd/start.go → agent.Start()
    → harness.DiscoverAuth(agentHome) → AuthConfig
    → runtime.Run(RunConfig{Auth: auth, Harness: h, ...})
        → harness.PropagateFiles(homeDir, auth)  // copy files into agent home
        → harness.GetVolumes(auth)                // or mount as volumes
        → harness.GetEnv(auth)                    // set env vars
```

### Auth Flow (Hub-Dispatched)

```
Hub: resolveSecrets() → []ResolvedSecret
    → DispatchAgentCreateWithGather(ResolvedSecrets, RequiredSecrets)
Broker: createAgent()
    → extractRequiredEnvKeys() → check satisfaction
    → if missing: return 202 + requirements
    → CLI gathers missing env → finalizeEnv()
    → agent.Start(opts.ResolvedSecrets, opts.Env)
        → inject env-type secrets into opts.Env
        → runtime projects file/variable secrets
```

### Harness Auth Summary

| Harness  | DiscoverAuth Sources | File Creds | Env Vars in GetEnv | Auth Modes |
|----------|---------------------|------------|-------------------|------------|
| Claude   | `ANTHROPIC_API_KEY` only | None | `ANTHROPIC_API_KEY` | API key only (Vertex via harness-config env) |
| Gemini   | Env + scion-agent.json + agent settings + host settings + files | OAuth, GCP ADC | `GEMINI_API_KEY`, `GOOGLE_API_KEY`, `VERTEX_API_KEY`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_APPLICATION_CREDENTIALS` | gemini-api-key, vertex-ai, oauth-personal, compute-default-credentials |
| Generic  | Env + settings + files | OAuth, GCP ADC | All mapped from AuthConfig | Passthrough |
| OpenCode | `ANTHROPIC_API_KEY` + file | auth.json | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` (direct) | API key + auth file |
| Codex    | File only | auth.json | `OPENAI_API_KEY`, `CODEX_API_KEY` (both direct) | Auth file + API keys |

**Key Issue**: OpenCode and Codex read `OPENAI_API_KEY` and `CODEX_API_KEY` directly from `os.Getenv()` in `GetEnv()`, bypassing AuthConfig entirely.

## Proposed Design

### Phase 1: Update AuthConfig to Be Complete

Add missing fields and remove obsolete ones:

```go
type AuthConfig struct {
    // Google/Gemini auth
    GeminiAPIKey         string  // env: GEMINI_API_KEY
    GoogleAPIKey         string  // env: GOOGLE_API_KEY
    GoogleAppCredentials string  // env/file: GOOGLE_APPLICATION_CREDENTIALS or ~/.config/gcloud/application_default_credentials.json
    GoogleCloudProject   string  // env: GOOGLE_CLOUD_PROJECT, GCP_PROJECT, ANTHROPIC_VERTEX_PROJECT_ID
    GoogleCloudRegion    string  // env: GOOGLE_CLOUD_REGION, CLOUD_ML_REGION, GOOGLE_CLOUD_LOCATION
    OAuthCreds           string  // file: ~/.gemini/oauth_creds.json

    // Anthropic auth
    AnthropicAPIKey      string  // env: ANTHROPIC_API_KEY

    // OpenAI/Codex auth
    OpenAIAPIKey         string  // env: OPENAI_API_KEY
    CodexAPIKey          string  // env: CODEX_API_KEY
    CodexAuthFile        string  // file: ~/.codex/auth.json
    OpenCodeAuthFile     string  // file: ~/.local/share/opencode/auth.json

    // Auth mode selection (primarily Gemini, but available to all)
    SelectedType         string  // e.g. "gemini-api-key", "vertex-ai", "oauth-personal"
}
```

**Changes**:
- Add `GoogleCloudRegion` (sources: `GOOGLE_CLOUD_REGION`, `CLOUD_ML_REGION`, `GOOGLE_CLOUD_LOCATION`)
- Add `OpenAIAPIKey` (source: `OPENAI_API_KEY`)
- Add `CodexAPIKey` (source: `CODEX_API_KEY`)
- Remove `VertexAPIKey` (redundant with `GoogleAPIKey`; all references removed outright, no deprecation path)

### Phase 2: Centralize Auth Gathering

Create a central function that populates AuthConfig from all available sources, replacing per-harness `DiscoverAuth()`:

```go
// pkg/harness/auth.go

// GatherAuth populates an AuthConfig from the environment, filesystem,
// and settings. It is source-agnostic: it checks env vars and well-known
// file paths without knowing which harness will consume the result.
func GatherAuth() AuthConfig {
    home, _ := os.UserHomeDir()

    auth := AuthConfig{
        // Env-var sourced fields
        GeminiAPIKey:       os.Getenv("GEMINI_API_KEY"),
        GoogleAPIKey:       os.Getenv("GOOGLE_API_KEY"),
        AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
        OpenAIAPIKey:       os.Getenv("OPENAI_API_KEY"),
        CodexAPIKey:        os.Getenv("CODEX_API_KEY"),
        GoogleCloudProject: util.FirstNonEmpty(
            os.Getenv("GOOGLE_CLOUD_PROJECT"),
            os.Getenv("GCP_PROJECT"),
            os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID"),
        ),
        GoogleCloudRegion: util.FirstNonEmpty(
            os.Getenv("GOOGLE_CLOUD_REGION"),
            os.Getenv("CLOUD_ML_REGION"),
            os.Getenv("GOOGLE_CLOUD_LOCATION"),
        ),
        GoogleAppCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
    }

    // File-sourced fields: check well-known paths
    if auth.GoogleAppCredentials == "" {
        adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
        if _, err := os.Stat(adcPath); err == nil {
            auth.GoogleAppCredentials = adcPath
        }
    }

    oauthPath := filepath.Join(home, ".gemini", "oauth_creds.json")
    if _, err := os.Stat(oauthPath); err == nil {
        auth.OAuthCreds = oauthPath
    }

    codexPath := filepath.Join(home, ".codex", "auth.json")
    if _, err := os.Stat(codexPath); err == nil {
        auth.CodexAuthFile = codexPath
    }

    opencodePath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
    if _, err := os.Stat(opencodePath); err == nil {
        auth.OpenCodeAuthFile = opencodePath
    }

    return auth
}
```

**Settings overlay**: After `GatherAuth()`, the caller can overlay values from settings (agent settings, host settings, scion-agent.json) for things like `SelectedType`. This overlay logic currently lives in Gemini's `DiscoverAuth()` and should be extracted. The overlay should be limited to `SelectedType` and similar mode-selection fields; API keys in settings files should not silently supersede environment-sourced credentials.

> **Out of scope**: The Gemini `selectedAuth` mechanism in `~/.gemini/settings.json` has its own complexity and should be refactored in a separate effort. This refactor provides the overlay hook but does not redesign the Gemini settings chain.

### Phase 3: Harness Auth Resolution (Per-Harness Preference Order)

Each harness defines its auth preference order and resolves which auth method to use given a populated AuthConfig. This replaces the current ad-hoc logic spread across `GetEnv()`.

```go
// pkg/api/harness.go - add to Harness interface

// ResolveAuth examines a fully-populated AuthConfig and returns the
// resolved auth method plus the env vars and files needed to establish it.
// Returns an error if no valid auth method could be determined.
type ResolvedAuth struct {
    Method   string            // e.g. "anthropic-api-key", "vertex-ai", "oauth-personal"
    EnvVars  map[string]string // env vars to inject into container
    Files    []FileMapping     // files to copy/mount into container
}

type FileMapping struct {
    SourcePath    string // host path (or AuthConfig field value)
    ContainerPath string // target path inside container (uses ~ for home)
}
```

#### Claude Auth Resolution

This replaces the current approach where Vertex auth is configured entirely through the harness-config `settings.json` `env:{}` block (hardcoded `CLAUDE_CODE_USE_VERTEX=1`, `CLOUD_ML_REGION`, `ANTHROPIC_VERTEX_PROJECT_ID`). That static env block will be removed and replaced by dynamic resolution here.

```
Preference order:
1. AnthropicAPIKey → set ANTHROPIC_API_KEY
2. GoogleAppCredentials + GoogleCloudProject + GoogleCloudRegion → Vertex mode
   → set CLAUDE_CODE_USE_VERTEX=1, CLOUD_ML_REGION, ANTHROPIC_VERTEX_PROJECT_ID
   → propagate ADC file
3. Fail: no valid auth → clear error message listing required credentials
```

**Error reporting**: On failure, produce an actionable message such as: *"Claude requires either ANTHROPIC_API_KEY or Vertex credentials (GOOGLE_APPLICATION_CREDENTIALS + GOOGLE_CLOUD_PROJECT + GOOGLE_CLOUD_REGION). Set the appropriate environment variables or configure hub secrets."*

#### Gemini Auth Resolution

```
Preference order (respects SelectedType if set):
1. If SelectedType == "oauth-personal" && OAuthCreds exists → oauth mode
2. If SelectedType == "vertex-ai" → vertex mode (needs GoogleCloudProject)
3. If SelectedType == "gemini-api-key" → api key mode (needs GeminiAPIKey or GoogleAPIKey)
4. If SelectedType == "compute-default-credentials" → ADC mode
5. Auto-detect: GeminiAPIKey/GoogleAPIKey → api-key, GoogleAppCredentials → ADC, OAuthCreds → oauth
6. Fail: no valid auth
```

#### Codex Auth Resolution

```
Preference order:
1. CodexAPIKey → set CODEX_API_KEY
2. OpenAIAPIKey → set OPENAI_API_KEY
3. CodexAuthFile → propagate to ~/.codex/auth.json
4. Fail: no valid auth
```

#### OpenCode Auth Resolution

```
Preference order:
1. AnthropicAPIKey → set ANTHROPIC_API_KEY
2. OpenAIAPIKey → set OPENAI_API_KEY
3. OpenCodeAuthFile → propagate to ~/.local/share/opencode/auth.json
4. Fail: no valid auth
```

### Phase 4: Unified Provisioning Flow

Replace the current split between `DiscoverAuth()`/`GetEnv()`/`PropagateFiles()`/`GetVolumes()` with a single flow:

```
1. GatherAuth()                    → AuthConfig (env + filesystem scan)
2. overlaySettings(auth, settings) → AuthConfig (settings/config overlays)
3. harness.ResolveAuth(auth)       → ResolvedAuth (method + env + files)
4. validateAuth(resolved)          → error if insufficient
5. applyAuth(resolved, container)  → inject env vars, copy/mount files
```

This flow works identically for both local and hub-dispatched modes:

- **Local**: `GatherAuth()` reads from host env/filesystem. `overlaySettings()` applies local settings.
- **Hub-dispatched**: `GatherAuth()` reads from broker env + injected `ResolvedSecrets` (environment-type secrets become env vars, file-type secrets are already written to target paths). The same `ResolveAuth()` logic applies.

**Auth inputs to `agent.Start()`**: The `agent.Start()` function should derive auth exclusively from the environment (env vars, filesystem), resolved secrets, and harness/harness-config configuration. The existing `AuthProvider` interface is removed (see Decisions below). There is no separate injection point — `GatherAuth()` reads what's available in the process environment, and `ResolveAuth()` determines the auth method.

### Phase 5: Simplify Harness Interface

The following Harness interface methods can be consolidated:

**Remove** (replaced by `ResolveAuth`):
- `DiscoverAuth(agentHome string) AuthConfig` — replaced by `GatherAuth()` + `ResolveAuth()`
- `GetVolumes(unixUsername string, auth AuthConfig) []VolumeMount` — volumes are derived from `ResolvedAuth.Files`

**Simplify** (derive from `ResolvedAuth`):
- `GetEnv()` — non-auth env vars remain (system prompt, telemetry), but auth env moves to `ResolvedAuth.EnvVars`
- `PropagateFiles()` — auth file propagation moves to `ResolvedAuth.Files`; non-auth file ops (settings.json updates) remain in `Provision()`

**New**:
- `ResolveAuth(auth AuthConfig) (*ResolvedAuth, error)` — per-harness auth resolution with preference order

### Phase 6: Align Hub Secrets with AuthConfig Fields

Document the mapping between hub secret names and AuthConfig fields so users know what to store:

| Hub Secret Name | Type | Target | AuthConfig Field |
|----------------|------|--------|-----------------|
| `GEMINI_API_KEY` | environment | `GEMINI_API_KEY` | GeminiAPIKey |
| `GOOGLE_API_KEY` | environment | `GOOGLE_API_KEY` | GoogleAPIKey |
| `ANTHROPIC_API_KEY` | environment | `ANTHROPIC_API_KEY` | AnthropicAPIKey |
| `OPENAI_API_KEY` | environment | `OPENAI_API_KEY` | OpenAIAPIKey |
| `CODEX_API_KEY` | environment | `CODEX_API_KEY` | CodexAPIKey |
| `GOOGLE_CLOUD_PROJECT` | environment | `GOOGLE_CLOUD_PROJECT` | GoogleCloudProject |
| `GOOGLE_CLOUD_REGION` | environment | `GOOGLE_CLOUD_REGION` | GoogleCloudRegion |
| `GOOGLE_APPLICATION_CREDENTIALS` | file | `~/.config/gcloud/application_default_credentials.json` | GoogleAppCredentials |
| `OAUTH_CREDS` | file | `~/.gemini/oauth_creds.json` | OAuthCreds |
| `CODEX_AUTH` | file | `~/.codex/auth.json` | CodexAuthFile |
| `OPENCODE_AUTH` | file | `~/.local/share/opencode/auth.json` | OpenCodeAuthFile |

For file-type secrets, the Hub stores base64-encoded content and the runtime projects them to the target path. The `GatherAuth()` function on the broker side would detect these projected files the same way it detects host files.

## Implementation Plan

### Step 1: Expand AuthConfig ✅
- Add `GoogleCloudRegion`, `OpenAIAPIKey`, `CodexAPIKey`
- Remove `VertexAPIKey` outright (search for all references, delete them; no migration/deprecation path)
- Remove `AuthProvider` interface from `pkg/api/types.go`
- Update all test fixtures
- Moved Codex/OpenCode `GetEnv()` from direct `os.Getenv()` to `AuthConfig` fields

### Step 2: Create GatherAuth ✅
- Implement `GatherAuth()` in `pkg/harness/auth.go`
- Write tests (env vars, project/region fallbacks, file discovery, precedence)

### Step 3: Implement ResolveAuth Per Harness ✅
- Add `ResolveAuth(AuthConfig) (*ResolvedAuth, error)` to Harness interface
- Implement for each harness with documented preference order
- Add `ResolvedAuth`, `FileMapping` types to `pkg/api/types.go`
- Write tests for each harness's resolution logic (valid combos, missing fields, preference order)

### Step 4: Wire Into Provisioning
- Update `agent.Start()` to use new flow: `GatherAuth → overlay → ResolveAuth → validate → apply`
- Remove `AuthProvider` usage from `agent.Start()` — auth comes from env/secrets + harness `ResolveAuth()`
- Update `runtime/common.go` to apply `ResolvedAuth` (env vars + files)
- Ensure hub-dispatched path produces same results
- Update broker `extractRequiredEnvKeys()` to consult `ResolveAuth`

### Step 4a: Remove Claude Harness-Config Static Auth
- Remove the `env:{}` block from Claude's `settings.json` embed (`pkg/harness/claude/embeds/settings.json`) that hardcodes `CLAUDE_CODE_USE_VERTEX=1`, `CLOUD_ML_REGION`, `ANTHROPIC_VERTEX_PROJECT_ID`
- Claude's `ResolveAuth()` dynamically produces these env vars when Vertex credentials are available
- Ensure Claude's `ResolveAuth()` produces clear error messages when no valid auth method is found

### Step 5: Clean Up Legacy Methods
- Remove `DiscoverAuth` from interface (after all callers migrated)
- Simplify `GetEnv` to only return non-auth env vars
- Simplify `PropagateFiles` to only handle non-auth file ops
- Remove `GetVolumes` if fully subsumed

### Step 6: Validation and Error Reporting
- Add `ValidateAuth()` that checks `ResolvedAuth` completeness before container launch
- Each harness's `ResolveAuth()` should produce clear, actionable error messages when auth is insufficient (e.g., "Claude requires ANTHROPIC_API_KEY or Vertex credentials (GOOGLE_APPLICATION_CREDENTIALS + GOOGLE_CLOUD_PROJECT + GOOGLE_CLOUD_REGION)")
- Integrate with env-gather to only request what's actually needed

### Step 7: Documentation
- Document the auth resolution flow and per-harness preference orders
- Document the hub secret name → AuthConfig field mapping
- Document how to configure each auth method (API key, Vertex, OAuth, ADC) for each harness

## Decisions

The following design questions have been resolved based on review feedback.

### D1. `ResolveAuth` returns a single best method

`ResolveAuth` returns the single best auth method, not a ranked list. The `SelectedType` field allows users to force a specific method when auto-detection isn't desired.

### D2. Gemini settings.json uses overlay pattern

`GatherAuth()` is env+filesystem only. Settings values (like `SelectedType`) are applied in `overlaySettings()`, called after `GatherAuth()` and before `ResolveAuth()`. The overlay should be limited to mode-selection fields; API keys in settings files should not silently supersede environment-sourced credentials. The broader Gemini `selectedAuth` mechanism refactor is out of scope for this effort.

### D3. File paths use `~` as portable home placeholder

`ResolvedAuth.Files[].ContainerPath` uses `~` as the home directory placeholder. The runtime layer expands `~` to the actual container home at apply time. This normalizes the abstraction and avoids harness-level path manipulation.

### D4. `GatherAuth` is harness-agnostic

`GatherAuth()` scans for all possible credentials regardless of which harness will consume them. The cost of a few extra `os.Stat` calls is negligible, and it keeps the architecture simple. `ResolveAuth()` is where harness-specificity lives.

### D5. Remove `AuthProvider` interface

The `AuthProvider` interface (`GetAuthConfig(context.Context) (AuthConfig, error)`) is removed. Currently `agent.Start()` uses it as an alternative to `DiscoverAuth()`:

```go
// Current code in agent.Start() (pkg/agent/run.go):
if opts.Auth != nil {
    auth, err = opts.Auth.GetAuthConfig(ctx)
} else {
    auth = h.DiscoverAuth(agentHome)
}
```

With the new flow, `agent.Start()` should rely exclusively on:
- **Environment**: env vars available in the process (set by the host, or injected from `ResolvedSecrets` for hub-dispatched agents)
- **Filesystem**: well-known credential file paths (detected by `GatherAuth()`)
- **Harness/Harness-config**: the harness's `ResolveAuth()` logic, informed by harness-config settings

There is no need for a separate `AuthProvider` abstraction. The inputs are fully determined by the execution environment. For testing, `GatherAuth()` can be called in a test environment with controlled env vars and filesystem, or `ResolveAuth()` can be called directly with a synthetic `AuthConfig`.

### D6. Claude Vertex auth moves from harness-config to dynamic resolution

The `env:{}` block in Claude's `settings.json` embed (`pkg/harness/claude/embeds/settings.json`) that hardcodes `CLAUDE_CODE_USE_VERTEX=1`, `CLOUD_ML_REGION=global`, `ANTHROPIC_VERTEX_PROJECT_ID=duet01` will be **removed**. Claude's `ResolveAuth()` will dynamically detect when `GoogleAppCredentials + GoogleCloudProject + GoogleCloudRegion` are available and produce the appropriate Vertex env vars. This unifies local and hub auth for Claude.

Clear error messages are critical: when no valid auth method is found, `ResolveAuth()` must explain exactly what credentials are needed and how to provide them.

### D7. Remove `VertexAPIKey` outright

`VertexAPIKey` is removed from `AuthConfig` with no deprecation path or migration shim. All references in `AuthConfig`, Gemini's `GetEnv()`, and the K8s runtime backward-compat code are deleted. Any existing configurations that reference `VERTEX_API_KEY` will fail with errors, which is acceptable for an alpha project.

### D8. All env var reads go through AuthConfig

All harness `GetEnv()` methods that read env vars via direct `os.Getenv()` calls (notably `OPENAI_API_KEY` in Codex/OpenCode, `CODEX_API_KEY` in Codex) must move into `AuthConfig` and be populated by `GatherAuth()`. This ensures all auth credentials are visible to validation and follow the single auth flow.

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Breaking existing local setups that use `VertexAPIKey` or `VERTEX_API_KEY` | Alpha project; break cleanly with clear errors. Users switch to `GoogleAPIKey` / `GOOGLE_API_KEY` |
| Claude Vertex auth stops working when harness-config env:{} block is removed | `ResolveAuth()` must be wired before the env block is removed; test both API-key and Vertex paths |
| Hub secrets not yet stored for all field types | Document the mapping; existing env-type secrets already work |
| Gemini's complex settings.json chain | Extract to overlay function with thorough test coverage; full selectedAuth redesign is out of scope |
| K8s runtime backward-compat code references old fields | Update K8s M1 fallback to use new field names |
| Harness interface changes break external consumers | Alpha project; interface changes are acceptable per CLAUDE.md |
| Insufficient auth produces confusing errors | Each `ResolveAuth()` must return detailed, actionable error messages listing required credentials |

## Files Affected

### Core Changes
- `pkg/api/types.go` — AuthConfig struct (expand + remove VertexAPIKey), remove AuthProvider interface, new ResolvedAuth type
- `pkg/api/harness.go` — Harness interface (add ResolveAuth, eventually remove DiscoverAuth/GetVolumes)
- `pkg/harness/auth.go` — **New**: GatherAuth, overlaySettings
- `pkg/harness/claude_code.go` — Add ResolveAuth, simplify GetEnv/PropagateFiles
- `pkg/harness/claude/embeds/settings.json` — Remove static Vertex auth `env:{}` block
- `pkg/harness/gemini_cli.go` — Add ResolveAuth, extract settings overlay, simplify GetEnv/PropagateFiles
- `pkg/harness/generic.go` — Add ResolveAuth, simplify GetEnv/PropagateFiles
- `pkg/harness/opencode.go` — Add ResolveAuth, simplify GetEnv/PropagateFiles
- `pkg/harness/codex.go` — Add ResolveAuth, simplify GetEnv/PropagateFiles

### Provisioning Flow
- `pkg/agent/run.go` — Use new GatherAuth → ResolveAuth flow; remove AuthProvider usage
- `pkg/runtime/common.go` — Apply ResolvedAuth (env + files)
- `pkg/runtime/interface.go` — RunConfig may carry ResolvedAuth instead of AuthConfig

### Hub/Broker
- `pkg/runtimebroker/handlers.go` — Update extractRequiredEnvKeys to use ResolveAuth
- `pkg/runtime/k8s_runtime.go` — Update M1 backward-compat fallback

### Tests
- `pkg/harness/*_test.go` — New tests for ResolveAuth, update existing
- `pkg/runtime/common_test.go` — Update for new auth flow
- `pkg/config/mock_harness_test.go` — Add ResolveAuth to mock
