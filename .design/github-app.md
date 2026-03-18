# GitHub App Integration for Scion Agents

**Created:** 2026-03-18
**Status:** Draft / Proposal (Rev 2)
**Related:** `hosted/git-groves.md`, `hosted/secrets-gather.md`, `agent-credentials.md`, `hosted/auth/oauth-setup.md`

---

## 1. Overview

Today, Scion agents authenticate to GitHub using **Personal Access Tokens (PATs)** stored as secrets (`GITHUB_TOKEN`). This works but has significant limitations:

- **PATs are user-scoped**: Tied to a single person's identity. If that person leaves or rotates credentials, all groves using their token break.
- **No automatic rotation**: PATs have fixed expiration. When they expire, agents fail until someone manually updates the secret.
- **Coarse permission model**: Fine-grained PATs can be scoped to repos, but the permissions are static — there's no way to issue narrower tokens per-agent or per-operation.
- **Attribution**: All commits and API calls appear as the PAT owner, not as the agent or the system.
- **Organization governance**: Org admins have limited visibility into which PATs access their repos and no central revocation mechanism.

**GitHub Apps** address all of these issues. This document proposes a design for integrating GitHub App authentication into Scion as a first-class alternative to PATs.

### Goals

1. Support GitHub App installation tokens as a credential source for agent git operations (clone, push) and GitHub API access (PRs, issues).
2. Automatic short-lived token generation — no manual rotation required.
3. Clear ownership model: who registers the app, who installs it, how installations map to groves.
4. Coexist with the existing PAT flow — GitHub App is an alternative, not a replacement.

### Non-Goals

- GitHub App as a Scion Hub user authentication provider (the existing GitHub OAuth flow handles Hub login separately).
- Multi-provider abstraction (GitLab, Bitbucket app equivalents). This design targets GitHub only.
- GitHub App Manifest flow for automated app creation.

---

## 2. GitHub App Primer

### 2.1 What Is a GitHub App?

A GitHub App is a first-class integration registered on GitHub. Unlike OAuth Apps or PATs, a GitHub App:

- Has its **own identity** separate from any user.
- Is **installed** on organizations or user accounts, granting it access to specific repositories.
- Authenticates using a **private key** (RSA) to generate short-lived JWTs, which are exchanged for **installation access tokens**.
- Has **fine-grained permissions** declared at registration time (e.g., Contents: read/write, Pull Requests: read/write, Issues: read/write).
- Can further **restrict tokens to specific repositories** at token creation time.

### 2.2 Authentication Flow

```
                GitHub App (registered)
                     |
                     | Private Key (PEM)
                     v
            ┌─────────────────┐
            │  Generate JWT   │  (signed with private key, 10-min expiry)
            │  (app identity) │
            └────────┬────────┘
                     |
                     v
            ┌─────────────────┐
            │  POST /app/     │  (JWT as Bearer token)
            │  installations/ │
            │  {id}/access_   │
            │  tokens         │
            └────────┬────────┘
                     |
                     v
            ┌─────────────────┐
            │ Installation    │  (scoped to repos, 1-hour expiry)
            │ Access Token    │
            └─────────────────┘
```

1. **JWT Generation**: The app signs a JWT using its private key. The JWT identifies the app (by App ID) and expires in 10 minutes.
2. **Token Request**: The JWT is used to call `POST /app/installations/{installation_id}/access_tokens`, optionally scoping to specific repositories and permissions.
3. **Installation Token**: GitHub returns a token (format `ghs_xxx`) valid for 1 hour. This token is used for git operations and API calls.

### 2.3 Installation Model

A GitHub App can be installed on:

- **An organization account**: Grants access to repos owned by that org. An org admin approves the installation.
- **A user account**: Grants access to repos owned by that user.

Each installation has a unique `installation_id`. A single GitHub App can have many installations across different orgs and users.

The installer chooses which repositories the app can access:
- **All repositories** in the org/account.
- **Selected repositories** — a specific subset.

### 2.4 Key Properties for Scion

| Property | PAT | GitHub App |
|----------|-----|------------|
| **Identity** | Personal user | App (machine identity) |
| **Token lifetime** | User-configured (max 1 year) | 1 hour (auto-generated) |
| **Rotation** | Manual | Automatic |
| **Repo scoping** | At PAT creation time (static) | Per-token request (dynamic) |
| **Permission scoping** | At PAT creation time (static) | Per-token request (dynamic, up to app max) |
| **Org visibility** | Limited (admin audit log) | Full (installed apps page, permissions visible) |
| **Rate limits** | User-level (5000/hr shared) | App-level (5000/hr per installation, separate from user) |
| **Revocation** | Per-token | Per-installation or per-app |
| **Commit attribution** | PAT owner | App identity (configurable) |

---

## 3. Ownership Model: Who Owns What?

This is the central design question. There are three levels at which a GitHub App could be attached to Scion:

### 3.1 Option A: Hub-Level App (Recommended)

**One GitHub App per Scion Hub deployment.**

```
Scion Hub
  └── GitHub App (registered by Hub admin)
        ├── Installation: org-acme (installation_id: 12345)
        │     ├── Grove: acme-widgets → repo: acme/widgets
        │     └── Grove: acme-api → repo: acme/api
        ├── Installation: org-beta (installation_id: 67890)
        │     └── Grove: beta-platform → repo: beta/platform
        └── Installation: user-alice (installation_id: 11111)
              └── Grove: alice-dotfiles → repo: alice/dotfiles
```

**Who does what:**

| Actor | Action |
|-------|--------|
| **Hub Admin** | Registers the GitHub App on GitHub. Configures App ID + private key on the Hub server. |
| **Org Admin / Repo Owner** | Installs the GitHub App on their org or user account (via GitHub UI). Selects which repos the app can access. |
| **Grove Creator** | Links a grove to a GitHub App installation (by providing the installation ID or via auto-discovery). |

**Pros:**
- Single app to manage. Org admins see one "Scion" app in their installed apps.
- Hub admin controls the app's maximum permissions.
- Natural fit for the Hub's role as central state authority.
- The private key never leaves the Hub — brokers receive only short-lived installation tokens.

**Cons:**
- Requires the Hub admin to register a GitHub App (operational burden for self-hosted deployments).
- The Hub must be reachable to mint tokens (already true for hosted mode).
- All organizations using the Hub must trust the same app identity.

### 3.2 Option B: User-Brought App (BYOA)

**Each user (or organization) registers their own GitHub App and provides credentials to the Hub.**

```
Scion Hub
  ├── User: alice
  │     └── GitHub App: alice-scion-app (App ID: 111, private key stored as secret)
  │           └── Installation: org-acme (installation_id: 12345)
  │                 └── Grove: acme-widgets
  └── User: bob
        └── GitHub App: bob-scion-app (App ID: 222, private key stored as secret)
              └── Installation: org-beta (installation_id: 67890)
                    └── Grove: beta-platform
```

**Who does what:**

| Actor | Action |
|-------|--------|
| **User / Org Admin** | Registers their own GitHub App. Provides App ID + private key to Scion (stored as a secret). |
| **User** | Installs the app on their org/account and associates installations with groves. |

**Pros:**
- No Hub admin involvement for GitHub setup.
- Users maintain full control over their app's permissions and installations.
- Different orgs can have fully independent app configurations.

**Cons:**
- More complex UX — every user must understand GitHub App registration.
- Private keys are uploaded as secrets to the Hub (acceptable with existing encrypted secret storage, but expands the trust surface).
- Multiple apps installed on the same org creates visual clutter in GitHub's UI.

### 3.3 Option C: Grove-Level App

**Each grove can have its own GitHub App configuration.**

This is essentially a finer-grained variant of Option B. Rather than one app per user, each grove can reference a different app. This adds flexibility but multiplies complexity. Not recommended as a primary model but should be supported as an escape hatch.

### 3.4 Recommendation

**Primary: Option A (Hub-Level App)** with **Option B (BYOA) as an advanced override.**

The Hub-level app covers the majority case: a team or organization deploys Scion Hub and configures a single GitHub App. Users install it on their orgs. This is the simplest UX for grove creators — they don't need to know about GitHub App internals.

For advanced users or multi-tenant deployments where organizations don't want to share an app identity, BYOA allows storing a user-specific or grove-specific GitHub App configuration. This uses the existing secret storage system.

The resolution hierarchy for GitHub App credentials follows the existing scope pattern:

```
Grove GitHub App config  →  (most specific, if set)
  ↓ fallback
User GitHub App config   →  (BYOA, if user registered their own app)
  ↓ fallback
Hub GitHub App config    →  (default, managed by Hub admin)
  ↓ fallback
GITHUB_TOKEN secret      →  (legacy PAT flow)
```

**Solo/Local Mode:** GitHub App is **Hub-only**. Solo mode continues to use PATs exclusively. GitHub App requires infrastructure (key management, token minting) that naturally lives on a server.

---

## 4. Data Model

### 4.1 GitHub App Configuration (Hub-Level)

The Hub server gains a new configuration section for the GitHub App:

```yaml
# Hub server config (e.g., hub.yaml or server flags)
github_app:
  app_id: 123456
  private_key_path: /etc/scion/github-app-key.pem
  # OR inline:
  # private_key: |
  #   -----BEGIN RSA PRIVATE KEY-----
  #   ...
  api_base_url: https://api.github.com  # default; override for GHES
```

In Go:

```go
type GitHubAppConfig struct {
    AppID          int64  `json:"app_id" yaml:"app_id" koanf:"app_id"`
    PrivateKeyPath string `json:"private_key_path,omitempty" yaml:"private_key_path,omitempty" koanf:"private_key_path"`
    PrivateKey     string `json:"private_key,omitempty" yaml:"private_key,omitempty" koanf:"private_key"`
    APIBaseURL     string `json:"api_base_url,omitempty" yaml:"api_base_url,omitempty" koanf:"api_base_url"`
}
```

**Settings Schema Note:** The `api_base_url` field must be tracked in the Hub settings schema for validation and UI rendering (see §11.5 resolution).

### 4.2 Installation Registration

Each GitHub App installation is registered as a Hub resource, linked to an organization or user:

```go
type GitHubInstallation struct {
    InstallationID int64     `json:"installation_id"`
    AccountLogin   string    `json:"account_login"`   // GitHub org or user login
    AccountType    string    `json:"account_type"`     // "Organization" or "User"
    AppID          int64     `json:"app_id"`           // Which app this installation belongs to
    CreatedAt      time.Time `json:"created_at"`
    CreatedBy      string    `json:"created_by"`       // Scion user who registered it
    Status         string    `json:"status"`           // "active", "suspended", "deleted"
}
```

### 4.3 Grove-to-Installation Mapping

A grove references a GitHub App installation for its credential source:

```go
// Existing Grove model, extended:
type Grove struct {
    // ... existing fields ...

    // GitHubInstallationID links this grove to a GitHub App installation.
    // When set, agents use installation tokens instead of PATs.
    GitHubInstallationID *int64 `json:"github_installation_id,omitempty"`

    // GitHubPermissions specifies the permissions to request when minting
    // installation tokens for this grove. If nil, the default set is used.
    GitHubPermissions *GitHubTokenPermissions `json:"github_permissions,omitempty"`
}

type GitHubTokenPermissions struct {
    Contents     string `json:"contents,omitempty"`      // "read" or "write"
    PullRequests string `json:"pull_requests,omitempty"` // "read" or "write"
    Issues       string `json:"issues,omitempty"`        // "read" or "write"
    Metadata     string `json:"metadata,omitempty"`      // "read"
    Checks       string `json:"checks,omitempty"`        // "read" or "write"
    Actions      string `json:"actions,omitempty"`       // "read"
}
```

Since groves are 1:1 with a repository, the installation token is always scoped to exactly one repo. The Hub automatically restricts the token to the grove's target repository regardless of whether the installation grants broader access.

### 4.4 BYOA: User-Level App Credentials

For Option B, the user stores their GitHub App credentials as secrets:

```bash
# Store App ID as a user-scoped secret
scion hub secret set GITHUB_APP_ID --type variable 123456

# Store private key as a user-scoped file secret
scion hub secret set GITHUB_APP_PRIVATE_KEY --type file @./my-app-key.pem
```

Or at grove scope for grove-level override:

```bash
scion hub secret set GITHUB_APP_ID --grove acme-widgets --type variable 789
scion hub secret set GITHUB_APP_PRIVATE_KEY --grove acme-widgets --type file @./grove-key.pem
```

---

## 5. Token Lifecycle

### 5.1 Token Minting

The Hub is the sole authority for minting installation tokens. This ensures private keys never leave the Hub.

```
Agent Start                   Hub                          GitHub API
    |                          |                              |
    |-- CreateAgent ---------->|                              |
    |                          |-- Resolve grove ------------>|
    |                          |   (has installation_id?)     |
    |                          |                              |
    |                          |-- Generate JWT (app key) --->|
    |                          |                              |
    |                          |-- POST /installations/       |
    |                          |   {id}/access_tokens ------->|
    |                          |   { repositories: [repo],    |
    |                          |     permissions: {            |
    |                          |       contents: write,        |
    |                          |       pull_requests: write    |
    |                          |     }                         |
    |                          |   }                          |
    |                          |                              |
    |                          |<-- token: ghs_xxx (1hr) -----|
    |                          |                              |
    |<-- GITHUB_TOKEN=ghs_xxx-|                              |
    |    (in resolved env)     |                              |
```

The minted token is injected as `GITHUB_TOKEN` in the agent's environment — **the agent doesn't know or care whether the token came from a PAT or a GitHub App**. This is key: the credential source is transparent to the agent and harness.

### 5.2 Token Refresh — Blended Approach

Installation tokens expire after 1 hour. Agents that run longer than 1 hour need token refresh. The design uses a **blended approach** that combines a credential helper (for git) with a background refresh loop (for `gh` CLI and other API consumers).

#### Component 1: Credential Helper (Git Operations)

The `sciontool` credential helper intercepts git credential requests and returns fresh tokens on demand:

```bash
# Git credential helper (configured during clone):
git config credential.helper '!sciontool credential-helper'

# sciontool credential-helper:
#   1. Check cached token age
#   2. If fresh (< 50 min): return cached token
#   3. If stale: call Hub refresh endpoint, cache new token, return
```

This provides the most native git integration — git operations transparently receive fresh tokens without any polling or background processes.

#### Component 2: Background Refresh Loop (API/CLI Operations)

`sciontool` runs a background goroutine that proactively refreshes the token before expiry, ensuring the `GITHUB_TOKEN` environment variable and on-disk token file stay current for non-git consumers like the `gh` CLI:

```
sciontool init
  └── tokenRefreshLoop():
        every 50 minutes:
          1. POST to Hub: /api/v1/agents/{id}/refresh-token
          2. Hub mints new installation token
          3. Hub returns token
          4. sciontool updates:
             - writes to /tmp/.github-token (for running processes to read)
             - updates git credential helper cache
```

The `gh` CLI and other tools that read `GITHUB_TOKEN` can be configured to read from `/tmp/.github-token` via a wrapper, or the token file path can be set via `GH_TOKEN_PATH` (custom env var read by sciontool's gh wrapper).

#### Why Both?

| Consumer | Mechanism | Rationale |
|----------|-----------|-----------|
| `git clone/push` | Credential helper | Native git integration; lazy refresh only when needed |
| `gh` CLI | Background loop | `gh` reads `GITHUB_TOKEN` at invocation time; needs proactive refresh |
| Custom scripts | Background loop | Any process reading the token file gets a fresh value |

### 5.3 Environment Variables

The following environment variables control GitHub App token behavior inside the agent container:

| Variable | Purpose |
|----------|---------|
| `GITHUB_TOKEN` | Initial token (set at agent start) |
| `SCION_GITHUB_APP_ENABLED` | `true` when credential source is GitHub App (enables refresh) |
| `SCION_GITHUB_TOKEN_EXPIRY` | ISO 8601 timestamp of initial token expiry |
| `SCION_GITHUB_TOKEN_PATH` | Path to refreshable token file (`/tmp/.github-token`) |

---

## 6. Installation Discovery and Association

### 6.1 Manual Association

The simplest flow: the user provides the installation ID when creating or configuring a grove.

```bash
# During grove creation
scion hub grove create https://github.com/acme/widgets.git --github-installation 12345

# Or after creation
scion hub grove set acme-widgets --github-installation 12345
```

The user finds the installation ID from the GitHub App's installation page or from `GET /app/installations` (which the Hub can proxy).

### 6.2 Auto-Discovery

When a grove is created from a GitHub URL and the Hub has a GitHub App configured, the Hub can automatically discover matching installations:

```
1. Hub generates JWT (app identity)
2. Hub calls GET /app/installations (lists all installations)
3. For each installation, calls GET /installation/repositories
4. Finds installation(s) that include the grove's target repo
5. If exactly one match: auto-associate
6. If multiple matches: prompt user to select (or pick the org-level one)
7. If no match: fall back to PAT, suggest installing the app
```

This auto-discovery runs during `scion hub grove create` or `scion start` (if the grove doesn't yet have an installation associated).

**Automated Install Scope:** Since groves are 1:1 with a repository, the Hub can programmatically scope the installation token to exactly the grove's target repo at minting time. Even if the installation grants "all repositories" access, the minted token is restricted to the single target repo. The Hub should log a recommendation when it detects an installation with overly broad "all repositories" access.

### 6.3 Installation Registration Flow

For Hub-level apps, a streamlined flow:

```bash
# Hub admin: configure the app (one-time)
scion server --github-app-id 123456 --github-app-key /path/to/key.pem

# User: install the app on their org (happens on GitHub.com)
# GitHub redirects to Hub callback URL after installation

# User: create a grove (auto-discovers installation)
scion hub grove create https://github.com/acme/widgets.git
# Output:
#   Grove created: acme-widgets
#   GitHub App: Found installation for org 'acme' (id: 12345)
#   Credential source: GitHub App (auto-refresh enabled)
```

### 6.4 Installation Webhooks

GitHub sends webhooks when the app is installed, uninstalled, or suspended. The Hub registers a webhook endpoint to automatically track installation lifecycle:

```
POST /api/v1/webhooks/github

Payload: { action: "created", installation: { id: 12345, account: { login: "acme" } } }
→ Hub creates GitHubInstallation record (status: "active")

Payload: { action: "deleted", installation: { id: 12345 } }
→ Hub marks installation as "deleted", alerts affected groves

Payload: { action: "suspend", installation: { id: 12345 } }
→ Hub marks installation as "suspended", affected groves fall back to PAT if available
```

**Public-Facing Requirement:** Webhook support is only available when the Hub endpoint is publicly reachable (GitHub must be able to POST to it). The Hub should:

1. **Detect accessibility:** During GitHub App configuration, attempt to verify that the webhook URL is reachable (e.g., by checking if the configured `hub.external_url` resolves to a public address, or by registering and testing a webhook ping).
2. **Graceful degradation:** If the Hub is not public-facing (e.g., behind a firewall, local network), webhooks are disabled and the Hub falls back to polling-based or manual discovery. A warning is surfaced in the admin UI.
3. **Webhook secret:** The webhook endpoint validates payloads using a shared secret configured alongside the GitHub App, preventing spoofed events.

**Revocation Handling:** When an installation is revoked (webhook `action: "deleted"` or detected via 403 during token minting):

1. The Hub marks the installation as `deleted`.
2. Running agents with valid tokens continue until their token expires (up to 1 hour).
3. Token refresh attempts fail; `sciontool` logs a clear error: "GitHub App installation revoked for org 'acme'."
4. Affected groves fall back to PAT if one is configured, or surface an error status.
5. The Hub sends a notification (via existing notification system) to the grove owner.

---

## 7. Hub API Changes

### 7.1 New Endpoints

```
# GitHub App configuration (admin only)
GET    /api/v1/github-app              → Returns app config (app ID, status, not the key)
PUT    /api/v1/github-app              → Update app config

# Installations
GET    /api/v1/github-app/installations           → List known installations
POST   /api/v1/github-app/installations/discover   → Trigger discovery from GitHub API
GET    /api/v1/github-app/installations/{id}       → Get installation details

# Grove association
PUT    /api/v1/groves/{id}/github-installation     → Set installation for grove
DELETE /api/v1/groves/{id}/github-installation     → Remove (fall back to PAT)

# Grove GitHub permissions
PUT    /api/v1/groves/{id}/github-permissions       → Set per-grove token permissions
GET    /api/v1/groves/{id}/github-permissions       → Get current permission config
DELETE /api/v1/groves/{id}/github-permissions       → Reset to defaults

# Token refresh (called by sciontool inside agent container)
POST   /api/v1/agents/{id}/refresh-token           → Mint fresh installation token

# Webhooks
POST   /api/v1/webhooks/github                     → Receive GitHub webhook events
```

### 7.2 Modified Endpoints

The existing agent creation flow (`POST /api/v1/groves/{id}/agents` and the Hub→Broker dispatch) is modified to:

1. Check if the grove has a `github_installation_id`.
2. If yes: mint an installation token (with grove-specific permissions if configured, otherwise defaults) and include it as `GITHUB_TOKEN` in resolved environment.
3. If no: fall through to existing PAT secret resolution.

This is transparent to the Broker and agent — they always receive a `GITHUB_TOKEN` env var regardless of source.

---

## 8. Permission Model

### 8.1 App-Level Permissions (Set at Registration)

The GitHub App should be registered with the **maximum permissions** any agent might need:

| Permission | Access | Purpose |
|------------|--------|---------|
| Contents | Read and write | Clone, commit, push |
| Metadata | Read | Repository info |
| Pull requests | Read and write | Create/update PRs |
| Issues | Read and write | Create/comment on issues |
| Checks | Read and write | Report CI status (future) |
| Actions | Read | Read workflow status (future) |

### 8.2 Per-Token Permission Restriction

When minting an installation token, the Hub can request a **subset** of the app's registered permissions. This enables least-privilege per grove:

```go
// Token request body
{
    "repositories": ["widgets"],           // Scope to specific repo (always single repo, grove is 1:1)
    "permissions": {
        "contents": "write",
        "pull_requests": "write",
        "metadata": "read"
    }
}
```

### 8.3 Grove-Level Permission Settings

Each grove can declare the permissions its agents need. This is configured in grove settings and stored as part of the grove model (see `GitHubTokenPermissions` in §4.3).

**CLI configuration:**

```bash
# Set grove-specific permissions
scion hub grove set acme-widgets --github-permissions contents:write,pull_requests:write,metadata:read

# View current permissions
scion hub grove get acme-widgets --show-github-permissions
```

**Template-driven defaults:**

```yaml
# In scion-agent.yaml template
github_permissions:
  contents: write
  pull_requests: write
  metadata: read
```

If a grove does not have explicit permissions configured, the **default permission set** is used: `Contents: write, Pull Requests: write, Metadata: read`.

**Validation:** The Hub validates that requested grove-level permissions do not exceed the app's registered permissions. If a grove requests `checks: write` but the app was not registered with Checks permission, the grove configuration is rejected with a clear error.

**Web UI:** Grove-level permission settings are managed in the **Grove Settings tab** alongside other grove configuration.

---

## 9. Integration with Existing Systems

### 9.1 Secret Resolution Pipeline

The GitHub App integration slots into the existing secret resolution pipeline as a new resolution source. The priority order:

```
1. Grove-scoped GITHUB_TOKEN secret (explicit PAT override)
2. GitHub App installation token (if grove has installation_id)
3. User-scoped GITHUB_TOKEN secret (user's PAT)
4. Hub-level GITHUB_TOKEN secret (shared PAT, if any)
```

If a grove has both a `GITHUB_TOKEN` secret and a `github_installation_id`, the explicit secret wins. This allows per-grove override (e.g., a grove that needs a token with org-admin permissions that the app doesn't have).

### 9.2 Agent Transparency

The agent and harness code requires **zero changes**. The credential arrives as `GITHUB_TOKEN` regardless of source. The git credential helper configured by `sciontool` works identically with both PATs and installation tokens. The `gh` CLI also uses `GITHUB_TOKEN` natively.

### 9.3 sciontool Changes

`sciontool` gains:

1. **Token refresh credential helper**: When `SCION_GITHUB_APP_ENABLED=true` is set in the environment, the credential helper calls the Hub to refresh tokens instead of returning a static value.
2. **Background token refresh loop**: Proactively refreshes the token every 50 minutes, writing the fresh token to `SCION_GITHUB_TOKEN_PATH` for non-git consumers.
3. **Token metadata awareness**: `sciontool` receives `SCION_GITHUB_TOKEN_EXPIRY` to know when the initial token expires, enabling proactive refresh scheduling.

### 9.4 Web UI

The web frontend gains:

**Hub Admin Page:**
- GitHub App configuration (App ID, status, API base URL, webhook status).
- Installation list with status indicators (active/suspended/deleted).
- Discovery trigger button.
- Webhook connectivity indicator (public-facing detection result).

**Grove Settings Tab** (grove-level items live here):
- Credential source indicator (PAT vs GitHub App) with health status.
- Installation association (select or auto-discover).
- GitHub token permission configuration.
- Token refresh status for active agents.

**Grove Creation Flow:**
- Option to select a GitHub App installation or enter PAT.
- Auto-discovery results when creating from a GitHub URL.

---

## 10. Commit Attribution

Agent commits can be attributed in three configurable ways:

### 10.1 Option A: App Bot Identity (Default)

Commits from `scion-app[bot]@users.noreply.github.com`. Clear automated provenance.

### 10.2 Option B: Custom Identity

Groves or templates specify `git user.name` and `git user.email`. The installation token authenticates the push, but the commit author is the configured identity. This is already supported — custom templates use standard Scion environment variable injection for `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, etc.

### 10.3 Option C: Co-authored-by Trailers

Use the bot identity but add `Co-authored-by: Alice <alice@example.com>` trailers linking to the Scion user who started the agent.

### 10.4 Configuration

Attribution mode is configurable at the grove level (in grove settings) and template level:

```yaml
# In grove settings or template
git_identity:
  mode: bot          # "bot" (default), "custom", "co-authored"
  name: "My Agent"   # Used when mode is "custom"
  email: "agent@example.com"
```

The default is **bot identity** (Option A). Templates that already set git user identity via Scion env vars continue to work — the custom identity takes precedence when explicitly configured.

---

## 11. Rate Limiting

GitHub App installation tokens have their own rate limit (5000 req/hr per installation). With many agents on the same grove (same installation), rate limits could potentially be exhausted by API-heavy agents.

**Strategy:**
1. **Monitor:** The Hub logs rate limit headers (`X-RateLimit-Remaining`, `X-RateLimit-Reset`) from GitHub API responses during token minting.
2. **Surface:** Rate limit status is included in agent health checks and visible in the Web UI.
3. **Warn:** When remaining rate limit drops below a threshold (e.g., 20%), the Hub surfaces a warning on affected groves.
4. **Future:** Consider per-agent rate limit budgeting if this becomes a practical issue.

---

## 12. GitHub Enterprise Server

The design supports GitHub Enterprise Server (GHES) from the start via the `api_base_url` configuration field:

```yaml
github_app:
  app_id: 123
  private_key_path: /path/to/key.pem
  api_base_url: https://github.mycompany.com/api/v3  # default: https://api.github.com
```

**Settings schema tracking:** The `api_base_url` field is registered in the Hub settings schema so that:
- The Web UI can render an appropriate configuration field.
- Validation ensures the URL is well-formed and reachable.
- The webhook public-facing detection (§6.4) accounts for GHES instances that may be on the same network as the Hub.

---

## 13. Private Key Rotation

The GitHub App private key can be rotated using GitHub's multi-key support combined with Scion's existing secret management:

**Procedure:**
1. Generate a new private key on GitHub (GitHub App settings → Generate a private key).
2. Update the key in Scion: `scion hub secret update GITHUB_APP_PRIVATE_KEY --type file @./new-key.pem` (for BYOA) or update the Hub server config file / secret manager.
3. Restart the Hub server or trigger a config reload.
4. Verify token minting works with the new key.
5. Delete the old key on GitHub.

During steps 2-3, both keys are valid on GitHub's side, so there is no downtime window.

**Documentation:** A runbook for key rotation should be included in the operations guide.

---

## 14. Alternatives Considered

### 14.1 GitHub OAuth User Tokens for Git Operations

Instead of a GitHub App, use the existing GitHub OAuth flow (already used for Hub login) to obtain user tokens with repo access scopes.

**Why rejected:**
- OAuth user tokens inherit the user's full access — no way to restrict to specific repos.
- Token refresh requires user interaction (re-auth).
- Commits attributed to the user, not the system.
- Conflates Hub authentication (who is this person?) with agent authorization (what can this agent do?).

### 14.2 GitHub App as Sole Auth Method (Replace PATs Entirely)

Force all users to use GitHub App, deprecate PAT support.

**Why rejected:**
- PATs are simpler for solo/local mode where there's no Hub.
- Not all users have org admin access to install apps.
- GitHub Enterprise Server may have restrictions on GitHub Apps.
- Backward compatibility — existing deployments rely on PATs.

### 14.3 Per-Agent GitHub App (One App per Agent)

Register a separate GitHub App for each agent.

**Why rejected:**
- GitHub has limits on app creation per account.
- Massive operational overhead.
- No benefit over installation-scoped tokens from a single app.

### 14.4 GitHub App Owned by Grove Creator (Not Hub Admin)

Instead of the Hub admin registering the app, require each grove creator to register one.

**Why rejected as primary:**
- Unreasonable UX burden for most users.
- However, this is preserved as the BYOA escape hatch (Option B in §3.2).

### 14.5 Proxy All Git Operations Through Hub

Instead of giving agents tokens, route all git clone/push through a Hub-side proxy that handles auth.

**Why rejected:**
- Massive bandwidth and latency implications.
- Breaks standard git tooling inside the agent.
- Over-engineered for the problem.

---

## 15. Security Considerations

### 15.1 Private Key Protection

The GitHub App private key is the most sensitive credential in this system. It can mint tokens for any installation of the app.

- **At rest**: Stored on the Hub server's filesystem or in a cloud secret manager (GCP SM, AWS SM). For BYOA, stored via Scion's encrypted secret storage (`scion hub secret set`).
- **In transit**: Never leaves the Hub. Brokers and agents receive only installation tokens.
- **Access**: Only the Hub server process reads the key. Filesystem permissions: `0600`, owned by the Hub service user.
- **Rotation**: Supported via GitHub's multi-key feature. Use `scion hub secret update` for managed rotation (see §13).

### 15.2 Installation Token Scope

Installation tokens are always scoped to the **minimum necessary**:
- **Repositories**: Scoped to the grove's target repository (single repo, since groves are 1:1 with repos).
- **Permissions**: Grove-level if configured (§8.3), otherwise default set (Contents: write, Pull Requests: write, Metadata: read).

Even if an installation grants access to "all repositories" in an org, the minted token only gets access to the specific repo the grove targets.

### 15.3 Token Exposure

Installation tokens are treated identically to PATs in the security model:
- Injected as environment variables (same as today).
- Never logged or written to disk by `sciontool` in plain text (existing sanitization applies). The token file at `SCION_GITHUB_TOKEN_PATH` has permissions `0600`.
- 1-hour expiry limits blast radius of token theft.

### 15.4 Webhook Security

The webhook endpoint (`/api/v1/webhooks/github`) validates all incoming payloads:
- **Signature verification**: Using the webhook secret configured alongside the GitHub App (`X-Hub-Signature-256` header).
- **Event filtering**: Only processes `installation` and `installation_repositories` events; ignores all others.
- **Rate limiting**: The webhook endpoint has its own rate limit to prevent abuse.

### 15.5 Trust Boundary

The Hub is the trust anchor. Organizations installing the GitHub App are trusting:
1. The Hub operator (who holds the private key).
2. The Scion platform (to mint correctly scoped tokens).
3. Their own installation scope (which repos the app can access).

This is comparable to installing any third-party GitHub App (CI systems, code review tools, etc.).

---

## 16. Implementation Phases

### Phase 1: Hub-Level App Configuration and Token Minting

1. Add `GitHubAppConfig` to Hub server configuration (including `api_base_url` for GHES).
2. Register `api_base_url` in Hub settings schema.
3. Implement JWT generation from private key (`pkg/hub/githubapp/`).
4. Implement installation token minting via GitHub API.
5. Add Hub API: `GET /api/v1/github-app`, `PUT /api/v1/github-app`.
6. Add `GitHubInstallation` model and store operations (with `status` field).
7. Add Hub API: `GET/POST /api/v1/github-app/installations`.
8. Unit tests for JWT generation and token exchange.

### Phase 2: Grove Association, Permissions, and Secret Resolution

1. Add `github_installation_id` and `github_permissions` to Grove model.
2. Modify `scion hub grove create` to accept `--github-installation` and `--github-permissions` flags.
3. Implement auto-discovery of installations for a given repo.
4. Add Hub API: grove GitHub permissions endpoints.
5. Integrate into secret resolution: when grove has installation, mint token with grove-specific permissions (or defaults).
6. Transparent injection as `GITHUB_TOKEN` in agent environment.
7. Integration tests: grove create → agent start → git clone with app token.

### Phase 3: Token Refresh (Blended)

1. Add Hub API: `POST /api/v1/agents/{id}/refresh-token`.
2. Extend `sciontool` credential helper to call Hub for fresh tokens (git operations).
3. Add `sciontool` background token refresh loop (gh CLI / API operations).
4. Add `SCION_GITHUB_APP_ENABLED`, `SCION_GITHUB_TOKEN_EXPIRY`, and `SCION_GITHUB_TOKEN_PATH` env vars.
5. Test long-running agents with token refresh cycle across both git and gh CLI usage.

### Phase 4: Webhooks and Installation Lifecycle

1. Add webhook endpoint: `POST /api/v1/webhooks/github`.
2. Implement webhook signature verification.
3. Handle installation created/deleted/suspended events.
4. Public-facing detection: verify Hub external URL is reachable for webhook delivery.
5. Graceful degradation when Hub is not public-facing (disable webhooks, warn in UI).
6. Proactive notification to grove owners on installation revocation.

### Phase 5: BYOA, Web UI, and Advanced Features

1. Support user-scoped and grove-scoped GitHub App secrets (`GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY`).
2. Resolution hierarchy: grove app → user app → hub app → PAT.
3. Commit attribution configuration (bot/custom/co-authored).
4. Web UI: Hub admin page, grove settings tab (credential source, permissions, installation).
5. Web UI: Grove creation flow with auto-discovery.
6. Rate limit monitoring and warning system.

---

## 17. Open Questions

### 17.1 Token File Security in Shared Containers

**Question:** The background refresh loop writes fresh tokens to `/tmp/.github-token`. In environments where the container filesystem might be inspected (e.g., debugging, shared volumes), is this an acceptable trade-off?

**Consideration:** The file has `0600` permissions and the token expires in 1 hour. This matches the security posture of `GITHUB_TOKEN` being available as an environment variable (which is also readable by any process in the container). However, environment variables are ephemeral while files persist until deleted.

**Action needed:** Decide whether `sciontool` should clean up the token file on agent exit, and whether an in-memory alternative (e.g., Unix domain socket) is worth the complexity.

### 17.2 Webhook Endpoint and Network Topology

**Question:** How does the Hub reliably determine whether its webhook endpoint is publicly reachable?

**Consideration:** Checking `hub.external_url` against DNS may not account for NAT, proxies, or firewall rules. Options:
- (a) Rely on the admin to declare `webhooks.enabled: true` in config (manual assertion).
- (b) Register a test webhook with GitHub and check for the ping event.
- (c) Use an external service to probe the endpoint.

**Leaning:** (a) as the simplest, with (b) as a validation step during GitHub App setup in the Web UI.

### 17.3 gh CLI Token Refresh Mechanism

**Question:** The `gh` CLI reads `GITHUB_TOKEN` from the environment at process start. If an agent runs `gh pr create` after the initial token has expired, it will use the stale env var. How do we ensure `gh` picks up the refreshed token?

**Consideration:** Options:
- (a) Wrapper script (`/usr/local/bin/gh`) that reads from the token file before delegating to the real `gh`.
- (b) Configure `gh` to use `GITHUB_TOKEN` from a file via `gh auth login --with-token < /tmp/.github-token` at refresh time.
- (c) Set `GH_TOKEN` to a shell command substitution (not supported by `gh` natively).

**Leaning:** (a) — a lightweight wrapper is the most reliable approach and already fits the `sciontool` pattern of injecting tooling into the agent container.

### 17.4 Multiple Installations for Same Org

**Question:** If an organization has multiple GitHub App installations (e.g., from BYOA where two users registered separate apps on the same org), how does auto-discovery resolve conflicts?

**Consideration:** Auto-discovery lists all installations that can access the target repo. If multiple installations match, the system needs a disambiguation strategy:
- Prefer the Hub-level app installation over BYOA installations.
- If multiple BYOA installations match, require the user to specify explicitly.
- The resolution hierarchy (§3.4) already handles this at the credential level, but auto-discovery needs to be aware of it.

### 17.5 Token Permissions Drift

**Question:** What happens when the GitHub App's registered permissions are reduced (e.g., admin removes "Issues: write" from the app), but groves still request that permission in their grove-level settings?

**Consideration:** GitHub will reject the token request if it includes permissions the app doesn't have. The Hub should:
- Detect this failure and surface a clear error.
- Periodically sync the app's current permission set from GitHub (`GET /app`) and validate grove configurations against it.
- The Web UI's permission selector should only offer permissions the app currently has.

### 17.6 Webhook Event Ordering and Idempotency

**Question:** GitHub does not guarantee webhook delivery order or exactly-once delivery. How should the Hub handle duplicate or out-of-order events?

**Consideration:** The `GitHubInstallation` model has a `Status` field. State transitions should be idempotent:
- A duplicate "created" event for an already-active installation is a no-op.
- A "deleted" event for an already-deleted installation is a no-op.
- Events should be processed with the GitHub delivery ID to detect duplicates.

---

## 18. References

- **GitHub Docs**: [About GitHub Apps](https://docs.github.com/en/apps/overview)
- **GitHub Docs**: [Authenticating as a GitHub App](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/about-authentication-with-a-github-app)
- **GitHub Docs**: [Creating an installation access token](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
- **Scion Design**: `.design/hosted/git-groves.md` — Current PAT-based git authentication
- **Scion Design**: `.design/hosted/secrets-gather.md` — Secret provisioning and resolution
- **Scion Design**: `.design/agent-credentials.md` — Agent credential management
- **Scion Design**: `.design/hosted/auth/oauth-setup.md` — Hub OAuth configuration (user auth, separate from this)
