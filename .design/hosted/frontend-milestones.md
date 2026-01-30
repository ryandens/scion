# Scion Web Frontend Implementation Milestones

## Overview

This document outlines the implementation milestones for the Scion Web Frontend. Each milestone is designed to be independently testable and builds upon previous work. The milestones follow a bottom-up approach: infrastructure first, then core functionality, then enhanced features.

For architectural details and component specifications, see **`web-frontend-design.md`**.

---

## Progress Summary

| Milestone | Status | Description |
|-----------|--------|-------------|
| M1 | Complete | Koa Server Foundation |
| M2 | Complete | Lit SSR Integration |
| M3 | Complete | Web Awesome Component Library |
| M4 | Complete | Authentication Flow |
| M5 | In Progress | Hub API Proxy |
| M6 | Complete | Grove & Agent Pages |
| M7 | Not Started | SSE + NATS Real-Time Updates |
| M8 | Not Started | Terminal Component |
| M9 | Not Started | Agent Creation Workflow |
| M10 | Not Started | Template Management UI |
| M11 | Not Started | User & Group Management UI |
| M12 | Not Started | Permissions & Policy Management UI |
| M13 | Not Started | Environment Variables & Secrets UI |
| M14 | Not Started | API Key Management UI |
| M15 | Not Started | Production Hardening |
| M16 | Not Started | Cloud Run Deployment |

**Status Legend:** Not Started | In Progress | Complete

---

## Milestone 1: Koa Server Foundation

**Goal:** Establish the basic Koa server infrastructure with static asset serving, health endpoints, and development tooling.

### Deliverables

- [x] **Project scaffolding**
   - TypeScript configuration
   - ESLint/Prettier setup
   - Vite build configuration
   - Package.json with dependencies

- [x] **Koa application core**
   - Application entry point (`src/server/index.ts`)
   - Middleware stack (logger, error handler, security headers)
   - Static asset serving from `public/`

- [x] **Health endpoints**
   - `GET /healthz` - liveness probe
   - `GET /readyz` - readiness probe (initially same as liveness)

- [x] **Development workflow**
   - Hot reload for server changes
   - Vite dev server for client assets
   - npm scripts: `dev`, `build`, `start`

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Server starts | `npm run dev` | Server listens on port 8080 |
| Health check | `curl localhost:8080/healthz` | `{"status":"healthy"}` with 200 |
| Static file | `curl localhost:8080/assets/test.txt` | File contents returned |
| 404 handling | `curl localhost:8080/nonexistent` | 404 with JSON error |
| Security headers | `curl -I localhost:8080/healthz` | CSP, X-Frame-Options present |

### Directory Structure After M1

```
web/
├── src/
│   └── server/
│       ├── index.ts
│       ├── app.ts
│       ├── config.ts
│       └── middleware/
│           ├── error-handler.ts
│           ├── logger.ts
│           └── security.ts
├── public/
│   └── assets/
├── package.json
├── tsconfig.json
└── vite.config.ts
```

---

## Milestone 2: Lit SSR Integration

**Goal:** Integrate @lit-labs/ssr for server-side rendering of Lit components with client-side hydration.

### Deliverables

- [x] **SSR renderer**
   - HTML shell template with hydration script injection
   - Lit component rendering via `@lit-labs/ssr`
   - Initial data serialization (`__SCION_DATA__` script tag)

- [x] **Basic Lit components (server + client)**
   - `<scion-app>` - application shell
   - `<scion-page-home>` - simple home page
   - `<scion-page-404>` - not found page

- [x] **Client hydration**
   - Client entry point (`src/client/main.ts`)
   - Hydration of SSR content
   - Client-side router setup (@vaadin/router)

- [x] **Page routes**
   - `GET /` - home page (SSR)
   - `GET /*` - catch-all for SPA routing

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| SSR home page | `curl localhost:8080/` | HTML with `<scion-app>` content |
| View source | Browser "View Source" | Complete HTML (not empty shell) |
| Hydration | Browser console | No hydration errors |
| Client navigation | Click internal link | Client-side route change (no reload) |
| Initial data | `document.getElementById('__SCION_DATA__')` | JSON with page data |
| 404 page | `curl localhost:8080/nonexistent-page` | 404 page SSR rendered |

### Key Technical Decisions

- Use declarative shadow DOM for SSR (`<template shadowroot="open">`)
- Serialize initial data as JSON in script tag (not inline in HTML)
- Use @vaadin/router for client-side routing (Lit-compatible)

---

## Milestone 3: Web Awesome Component Library

**Goal:** Integrate Web Awesome component library and establish the UI foundation with theming.

### Deliverables

- [x] **Web Awesome integration**
   - CDN script/style loading (using Shoelace)
   - Theme CSS custom properties
   - Component registration verification

- [x] **Core UI components**
   - `<scion-nav>` - sidebar navigation
   - `<scion-header>` - top header bar
   - `<scion-breadcrumb>` - breadcrumb navigation
   - `<scion-status-badge>` - status indicator

- [x] **Layout system**
   - Responsive sidebar layout
   - Content area with padding/scrolling
   - Mobile breakpoint handling

- [x] **Theme configuration**
   - Light/dark mode support
   - CSS custom property overrides
   - Consistent color palette

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Web Awesome loads | Browser console | No 404 for WA assets |
| Components render | Visual inspection | `<wa-button>`, `<wa-card>` display correctly |
| Theme variables | DevTools | CSS custom properties applied |
| Dark mode | Toggle theme | Colors switch appropriately |
| Responsive | Resize to mobile | Sidebar collapses/hides |
| Navigation | Click nav items | Routes change, active state updates |

### Notes

- Initially load Web Awesome from CDN for simplicity
- Future optimization: bundle locally for offline/faster loads
- Ensure SSR output includes Web Awesome component tags (hydrated client-side)

---

## Milestone 4: Authentication Flow


**Goal:** Implement OAuth authentication with session management.

### Deliverables

- [x] **Session middleware**
   - koa-session configuration
   - Secure cookie settings
   - Session store (in-memory for dev, Redis for prod)

- [x] **OAuth routes**
   - `GET /auth/login/:provider` - initiate OAuth
   - `GET /auth/callback/:provider` - OAuth callback
   - `POST /auth/logout` - clear session
   - `GET /auth/me` - current user info

- [x] **OAuth providers**
   - Google OAuth 2.0 integration
   - GitHub OAuth integration
   - Provider abstraction for future additions

- [x] **Auth middleware**
   - `auth()` middleware for protected routes
   - Redirect to login for unauthenticated requests
   - User context injection into SSR

- [x] **Login UI**
   - `<scion-login-page>` component
   - Provider selection buttons
   - Error handling/display

### Basic authorization
While the Google oauth provides authentication, we will have a simple settings based authorization that for now will simply check the domain of the email address of the logged in user against a list in the settings of AuthorizedDomains

Note: the implementation of this auth flow should not interfer with the use of dev-auth mode.

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Login redirect | Visit protected route | Redirect to `/auth/login` |
| Google OAuth | Click "Login with Google" | Redirect to Google, then callback |
| Session created | After OAuth callback | Session cookie set |
| User in context | Visit protected route | User info available in page |
| Logout | POST /auth/logout | Session cleared, redirect to login |
| Auth/me API | `curl /auth/me` with session | User JSON returned |
| Invalid session | Expired/tampered cookie | Redirect to login |

### Configuration Required

```bash
# Environment variables for testing
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
SESSION_SECRET=dev-secret-min-32-chars-long
BASE_URL=http://localhost:8080
```

---

## Milestone 5: Hub API Proxy

**Goal:** Proxy requests to the Hub API with authentication header injection.

### Deliverables

- [x] **API proxy middleware**
   - Route `/api/*` to Hub API
   - Forward authentication headers
   - Request/response logging
   - Error transformation

- [x] **Hub client service**
   - Typed API client for server-side calls
   - Request timeout handling
   - Retry logic with backoff

- [ ] **SSR data fetching**
   - Fetch data during SSR for initial render
   - Pass data to Lit components
   - Error boundary for failed fetches

- [ ] **Mock Hub API (for testing)**
   - Standalone mock server
   - Fixtures for common responses
   - Configurable via environment

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Proxy request | `curl /api/groves` (authenticated) | Hub API response |
| Auth forwarding | Check Hub logs | Authorization header present |
| Timeout | Slow Hub response | 504 after timeout |
| Hub down | Stop Hub | 502 Bad Gateway |
| Rate limit headers | Response headers | X-RateLimit-* forwarded |
| SSR with data | Visit `/groves` | Page renders with grove list |
| Mock mode | `HUB_MOCK=true npm run dev` | Mock data returned |

### Mock Server

Create a simple mock for development without a real Hub:

```typescript
// tools/mock-hub/index.ts
// Serves static JSON responses for Hub API endpoints
```

### Implementation Notes

- **API Proxy** (`src/server/routes/api.ts`): Full implementation with auth header forwarding, debug logging, error transformation, and query string passthrough
- **Dev Token Injection**: When `DEV_AUTH=true`, the proxy injects a dev token for local Hub authentication
- **SSR Data Fetching**: Deferred to client-side in current implementation; pages fetch data on mount rather than during SSR

---

## Milestone 6: Grove & Agent Pages

**Goal:** Implement the core pages for viewing and managing groves and agents.

### Deliverables

- [x] **Grove pages**
   - [x] `<scion-grove-list>` - list all groves with filtering
   - [x] `<scion-grove-detail>` - single grove view with agent list
   - [x] Grove card component with status summary

- [x] **Agent pages**
   - [x] `<scion-agent-list>` - agents within a grove
   - [x] `<scion-agent-detail>` - single agent view
   - [x] Agent card component with status, actions

- [x] **Action handlers**
   - [x] Start/stop agent buttons (wired to API)
   - [x] Delete agent with confirmation
   - [ ] Create agent dialog (basic) - deferred to M9

- [ ] **State management (client)**
   - [x] Basic client-side state in components
   - [ ] State manager class
   - [ ] Hydration from SSR data
   - [ ] Optimistic updates

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Grove list loads | Visit `/groves` | Groves displayed from Hub API |
| Grove detail | Click grove | Navigate to grove detail page |
| Agent list | Visit grove detail | Agents listed for that grove |
| Agent status | View agent card | Correct status badge color |
| Stop agent | Click "Stop" button | API call, status updates |
| Start agent | Click "Start" button | API call, status updates |
| Delete agent | Click "Delete", confirm | Agent removed from list |
| Empty state | Grove with no agents | "No agents" message |
| Loading state | Slow API | Loading spinner shown |
| Error state | API error | Error message displayed |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/groves` | Grove list | All groves |
| `/groves/:groveId` | Grove detail | Grove + agents |
| `/agents/:agentId` | Agent detail | Agent |

### Implementation Notes

- **Grove List** (`src/components/pages/groves.ts`): Fetches from `/api/groves` on mount, displays cards with status badges
- **Grove Detail** (`src/components/pages/grove-detail.ts`): Fetches grove and agents in parallel, displays grove info, stats, and agent cards with actions
- **Agent List** (`src/components/pages/agents.ts`): Fetches from `/api/agents` on mount, shows agent cards with wired action buttons (Start/Stop/Delete)
- **Agent Detail** (`src/components/pages/agent-detail.ts`): Fetches agent and grove info, displays detailed agent information with quick actions
- **Status Badges**: Uses Shoelace badge variants mapped from API status strings
- **Action Handlers**: Start/Stop/Delete wired to Hub API via POST/DELETE requests with loading states
- **Routing**: SSR renderer handles `/groves/:groveId` and `/agents/:agentId` routes
- **Remaining Work**: State manager class, SSR data hydration, optimistic updates, agent creation dialog (M9)

---

## Milestone 7: SSE + NATS Real-Time Updates

**Goal:** Implement the Snapshot + Delta pattern with SSE and NATS for real-time updates.

### Deliverables

- [ ] **NATS client**
   - Connection management with reconnection
   - Subject subscription/unsubscription
   - Message deserialization

- [ ] **SSE endpoint**
   - `GET /events` - SSE stream
   - `POST /events/subscribe` - subscribe to subjects
   - Connection tracking per user
   - Heartbeat messages

- [ ] **SSE manager**
   - Connection lifecycle management
   - NATS-to-SSE message bridging
   - Subject-based routing
   - Permission filtering

- [ ] **Client SSE handler**
   - EventSource connection
   - Reconnection with backoff
   - Message parsing and dispatch

- [ ] **Reactive component updates**
   - State manager integration with SSE
   - Delta merging into local state
   - Lit reactive property updates

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| SSE connects | Browser Network tab | EventSource connection open |
| Heartbeat | Wait 30s | Heartbeat event received |
| Subscribe | Call subscribe API | Subscription confirmed |
| Agent status update | Change agent status in Hub | UI updates without refresh |
| Agent created | Create agent via CLI | New agent appears in list |
| Agent deleted | Delete agent via CLI | Agent removed from list |
| Reconnection | Kill NATS, restart | SSE reconnects automatically |
| Multiple tabs | Open same page in 2 tabs | Both receive updates |
| Permission check | Subscribe to unauthorized grove | Subscription rejected |

### NATS Testing

```bash
# Start NATS for local development
docker run -p 4222:4222 nats:latest

# Publish test message
nats pub agent.test123.status '{"status":"running"}'
```

---

## Milestone 8: Terminal Component

**Goal:** Implement the xterm.js-based terminal for PTY access to agents.

### Deliverables

- [ ] **Terminal component**
   - `<scion-terminal>` Lit component
   - xterm.js integration with addons (fit, web-links)
   - Theme matching Web Awesome colors

- [ ] **WebSocket connection**
   - Ticket-based authentication
   - PTY WebSocket proxy through Koa
   - Binary data handling (base64)

- [ ] **Terminal features**
   - Auto-resize on container change
   - Connection status indicator
   - Reconnection handling
   - Copy/paste support

- [ ] **Terminal page**
   - Full-screen terminal view
   - Toolbar with agent info and actions
   - Back navigation

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Terminal loads | Visit `/agents/:id/terminal` | Terminal container renders |
| WebSocket connects | Network tab | WS connection established |
| PTY output | Run command in agent | Output displays in terminal |
| Keyboard input | Type in terminal | Input sent to agent |
| Resize | Resize browser window | Terminal adjusts, PTY resizes |
| Connection lost | Kill agent container | "Disconnected" shown |
| Reconnect | Click reconnect button | Terminal reconnects |
| Theme | Toggle dark/light mode | Terminal colors update |
| Copy text | Select and Ctrl+C | Text copied to clipboard |

### WebSocket Proxy

The Koa server proxies WebSocket connections to the Hub API:

```
Browser WS → Koa WS Proxy → Hub API WS → Runtime Host
```

---

## Milestone 9: Agent Creation Workflow

**Goal:** Implement the full agent creation flow with template selection and configuration.

### Deliverables

- [ ] **Create agent dialog**
   - `<scion-create-agent-dialog>` component
   - Template selector
   - Configuration form (name, task, branch)
   - Advanced options (image, env vars)

- [ ] **Template browser**
   - List available templates
   - Template detail view
   - Template preview

- [ ] **Creation flow**
   - Form validation
   - API submission
   - Progress tracking
   - Error handling

- [ ] **Post-creation navigation**
   - Redirect to agent detail
   - Option to open terminal
   - Notification of creation

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Open dialog | Click "New Agent" | Dialog opens |
| Template list | View templates | Templates from Hub API |
| Select template | Click template | Template selected, form updates |
| Validation | Submit empty name | Validation error shown |
| Create agent | Fill form, submit | Agent created, redirect to detail |
| Creation error | Hub returns error | Error message displayed |
| Cancel | Click cancel | Dialog closes, no changes |
| Advanced options | Expand advanced | Additional fields shown |

---

## Milestone 10: Template Management UI

**Goal:** Implement the template browser, viewer, and upload functionality for managing agent templates.

### Deliverables

- [ ] **Template list page**
   - `<scion-template-list>` - filterable template browser
   - Scope filtering (global/grove/user)
   - Harness type filtering
   - Search functionality

- [ ] **Template detail/viewer**
   - `<scion-template-detail>` - template configuration viewer
   - File manifest display
   - Version history (future)

- [ ] **Template card component**
   - `<scion-template-card>` - template summary card
   - Scope badge display
   - Action buttons (view, clone, delete)

- [ ] **Template upload wizard**
   - `<scion-template-upload>` - multi-step upload form
   - Metadata entry (name, description, harness)
   - File selection and upload
   - Signed URL upload integration
   - Finalization step

- [ ] **Template scope selector**
   - `<scion-scope-selector>` - reusable scope picker
   - Support for user/grove scopes

- [ ] **Template clone dialog**
   - Clone to different scope
   - Rename on clone

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Template list loads | Visit `/templates` | Templates displayed |
| Scope filter | Select "Global" | Only global templates shown |
| Search | Type in search box | Templates filtered by name |
| View template | Click "View" | Navigate to detail page |
| Clone template | Click "Clone" | Clone dialog opens |
| Upload template | Complete upload wizard | Template created, files uploaded |
| Delete template | Click "Delete" (non-global) | Template removed |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/templates` | Template list | Templates (scoped) |
| `/templates/:templateId` | Template detail | Template + files |
| `/templates/new` | Template upload | None (form) |

---

## Milestone 11: User & Group Management UI

**Goal:** Implement user listing, group management, and membership functionality.

### Deliverables

- [ ] **User list page**
   - `<scion-user-list>` - user directory
   - Role/status badges
   - Search and filtering
   - Role modification (admin only)

- [ ] **User detail page**
   - `<scion-user-detail>` - user profile view
   - Group memberships
   - Recent activity

- [ ] **User avatar component**
   - `<scion-user-avatar>` - avatar with fallback
   - Status indicator

- [ ] **Group list page**
   - `<scion-group-list>` - group directory
   - Member count display
   - Create group button

- [ ] **Group detail page**
   - `<scion-group-detail>` - group management
   - Member list with add/remove
   - Group metadata editing
   - Nested group support (display)

- [ ] **Member list component**
   - `<scion-member-list>` - reusable member table
   - User and group members
   - Role column
   - Remove action

- [ ] **Group badge component**
   - `<scion-group-badge>` - group indicator

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| User list | Visit `/users` | Users displayed |
| User search | Type in search | Users filtered |
| View user | Click user row | Navigate to user detail |
| Group list | Visit `/groups` | Groups displayed |
| Create group | Click "New Group" | Group created |
| Add member | Click "Add Member" | Member added |
| Remove member | Click "Remove" | Member removed |
| Edit group | Modify group name | Group updated |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/users` | User list | Users (paginated) |
| `/users/:userId` | User detail | User + memberships |
| `/groups` | Group list | Groups |
| `/groups/:groupId` | Group detail | Group + members |

---

## Milestone 12: Permissions & Policy Management UI

**Goal:** Implement policy creation, editing, and access evaluation debugging tools.

### Deliverables

- [ ] **Policy list page**
   - `<scion-policy-list>` - policy directory
   - Scope/effect filtering
   - Principal count display

- [ ] **Policy detail/editor**
   - `<scion-policy-editor>` - policy form
   - Scope type selection
   - Resource type selection
   - Action checkboxes
   - Effect radio (allow/deny)
   - Priority input

- [ ] **Principal selector**
   - `<scion-principal-selector>` - user/group picker
   - Multi-select support
   - Search within selector

- [ ] **Permission badge component**
   - `<scion-permission-badge>` - permission indicator
   - Allow/deny styling

- [ ] **Access evaluation tool**
   - `<scion-access-evaluator>` - debug interface
   - Principal selection
   - Resource selection
   - Action selection
   - Evaluate button
   - Result display with explanation
   - Matched policy display

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Policy list | Visit `/policies` | Policies displayed |
| Create policy | Click "New Policy" | Policy form opens |
| Add principal | Use principal selector | Principal added |
| Save policy | Submit form | Policy created |
| Evaluate access | Use evaluator | Result shown |
| Denied result | Evaluate denied access | Red denied badge, explanation |
| Matched policy | Evaluate access | Matching policy displayed |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/policies` | Policy list | Policies |
| `/policies/:policyId` | Policy editor | Policy + bindings |
| `/policies/new` | Policy creator | None (form) |

---

## Milestone 13: Environment Variables & Secrets UI

**Goal:** Implement scoped environment variable and secret management.

### Deliverables

- [ ] **Environment settings page**
   - `<scion-settings-env>` - env/secrets management
   - Tab: Environment Variables
   - Tab: Secrets
   - Scope selector (user/grove/host)

- [ ] **Env var table**
   - Key/value display
   - Sensitive value masking
   - Edit/delete actions

- [ ] **Secret table**
   - Key/metadata display (no values)
   - Version tracking
   - Update/delete actions

- [ ] **Scope selector component**
   - Scope type dropdown
   - Grove/host selector when applicable

- [ ] **Env var editor dialog**
   - `<scion-env-var-editor>` - create/edit form
   - Key validation (UPPER_SNAKE_CASE)
   - Sensitive toggle
   - Description field

- [ ] **Secret editor dialog**
   - `<scion-secret-editor>` - create/update form
   - Write-only value field
   - Description field
   - "Keep current value" option on edit

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| List env vars | Visit settings page | Env vars displayed |
| Switch scope | Select "Grove" | Grove env vars shown |
| Add env var | Click "Add Variable" | Dialog opens |
| Edit env var | Click "Edit" | Dialog with current value |
| Delete env var | Click "Delete" | Variable removed |
| List secrets | Switch to Secrets tab | Secrets displayed (no values) |
| Add secret | Click "Add Secret" | Dialog opens |
| Update secret | Click "Update" | New value saved |
| Secret not shown | View secret row | Only metadata, no value |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/settings/env` | Env settings (user) | User env vars + secrets |
| `/groves/:groveId/settings/env` | Env settings (grove) | Grove env vars + secrets |

---

## Milestone 14: API Key Management UI

**Goal:** Implement API key creation, listing, and revocation.

### Deliverables

- [ ] **API keys page**
   - `<scion-api-keys>` - key management
   - Key list table
   - Key prefix display
   - Last used timestamp
   - Expiry display

- [ ] **Create key dialog**
   - Key name input
   - Expiry option
   - Scope selection (future)

- [ ] **Key display alert**
   - One-time key display
   - Copy button
   - Warning about single display

- [ ] **Revoke confirmation**
   - Confirmation dialog
   - Key name display

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| List keys | Visit `/settings/api-keys` | Keys displayed |
| Create key | Click "Create API Key" | Dialog opens |
| Key shown once | After creation | Full key displayed |
| Copy key | Click copy button | Key copied to clipboard |
| Key hidden | View list after creation | Only prefix shown |
| Revoke key | Click "Revoke" | Key removed |
| Revoked key fails | Use revoked key | 401 Unauthorized |

### Routes

| Route | Page | SSR Data |
|-------|------|----------|
| `/settings/api-keys` | API keys | Key metadata (no values) |

---

## Milestone 15: Production Hardening

**Goal:** Prepare for production deployment with security, performance, and observability improvements.

### Deliverables

- [ ] **Security hardening**
   - CSRF protection
   - Rate limiting
   - Input sanitization
   - Audit logging

- [ ] **Performance optimization**
   - Asset bundling and minification
   - Gzip/Brotli compression
   - Cache headers configuration
   - Critical CSS inlining

- [ ] **Error handling**
   - Global error boundary
   - User-friendly error pages
   - Error reporting integration (optional)

- [ ] **Logging and monitoring**
   - Structured JSON logging
   - Request ID tracing
   - Metrics endpoint (optional)

- [ ] **Configuration management**
   - Environment-based config
   - Secret handling
   - Feature flags

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| CSRF protection | POST without token | 403 Forbidden |
| Rate limiting | Exceed limit | 429 Too Many Requests |
| Asset compression | Check Content-Encoding | gzip or br |
| Cache headers | Check static assets | Cache-Control set |
| Error page | Trigger 500 error | Friendly error page |
| Structured logs | Check stdout | JSON log entries |
| Request tracing | Check logs | X-Request-ID present |

---

## Milestone 16: Cloud Run Deployment

**Goal:** Deploy the web frontend to Cloud Run with full CI/CD pipeline.

### Deliverables

- [ ] **Container image**
   - Multi-stage Dockerfile
   - Minimal production image
   - Non-root user

- [ ] **Cloud Run configuration**
   - Service definition (cloudrun.yaml)
   - Environment variables
   - Secret references
   - Resource limits

- [ ] **CI/CD pipeline**
   - Build on push to main
   - Run tests
   - Build and push image
   - Deploy to Cloud Run

- [ ] **Infrastructure**
   - Secret Manager setup
   - IAM configuration
   - VPC connector (for Hub access)
   - Custom domain (optional)

- [ ] **Monitoring**
   - Cloud Run metrics
   - Error reporting
   - Uptime checks

### Test Criteria

| Test | Method | Expected Result |
|------|--------|-----------------|
| Container builds | `docker build .` | Image builds successfully |
| Container runs | `docker run ...` | Server starts, health check passes |
| Deploy to staging | Push to staging branch | Deploys to staging environment |
| Health check | Cloud Run console | Instance healthy |
| Cold start | Scale to 0, then request | Response within 5s |
| Secrets loaded | Check app behavior | OAuth works, session works |
| Hub connectivity | Create agent | Agent created successfully |
| Custom domain | Visit domain | SSL works, site loads |

### Deployment Commands

```bash
# Build and push
gcloud builds submit --tag gcr.io/PROJECT/scion-web

# Deploy
gcloud run deploy scion-web \
  --image gcr.io/PROJECT/scion-web \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated
```

---

## Milestone Dependencies

```
M1 ──► M2 ──► M3 ──┬──► M4 ──► M5 ──► M6 ──┬──► M7 ──► M8
                   │                        │
                   │                        ├──► M9
                   │                        │
                   │                        ├──► M10 (Template Mgmt)
                   │                        │
                   │                        ├──► M11 (User/Group) ──► M12 (Permissions)
                   │                        │
                   │                        ├──► M13 (Env/Secrets)
                   │                        │
                   │                        └──► M14 (API Keys)
                   │
                   └──────────────────────────────────► M15 ──► M16
```

| Milestone | Depends On | Can Parallelize With |
|-----------|------------|----------------------|
| M1: Koa Foundation | - | - |
| M2: Lit SSR | M1 | - |
| M3: Web Awesome | M2 | - |
| M4: Authentication | M3 | - |
| M5: Hub API Proxy | M4 | - |
| M6: Grove & Agent Pages | M5 | - |
| M7: SSE + NATS | M6 | M9, M10-M14 |
| M8: Terminal | M7 | - |
| M9: Agent Creation | M6 | M7, M8, M10-M14 |
| M10: Template Management | M6 | M7-M9, M11-M14 |
| M11: User & Group Mgmt | M6 | M7-M10, M13-M14 |
| M12: Permissions & Policy | M11 | M7-M10, M13-M14 |
| M13: Env & Secrets | M6 | M7-M12, M14 |
| M14: API Key Mgmt | M4 | M7-M13 |
| M15: Production Hardening | M3+ | M7-M14 |
| M16: Cloud Run Deployment | M15 | - |

---

## Estimated Complexity

| Milestone | Complexity | Key Risks |
|-----------|------------|-----------|
| M1: Koa Foundation | Low | None |
| M2: Lit SSR | Medium | @lit-labs/ssr edge cases |
| M3: Web Awesome | Low | Version compatibility |
| M4: Authentication | Medium | OAuth provider config |
| M5: Hub API Proxy | Low | None |
| M6: Grove & Agent Pages | Medium | UI/UX decisions |
| M7: SSE + NATS | High | Connection management, race conditions |
| M8: Terminal | Medium | xterm.js SSR compatibility |
| M9: Agent Creation | Medium | Form complexity |
| M10: Template Management | Medium | File upload UX, signed URL handling |
| M11: User & Group Mgmt | Medium | Member list UX, nested groups |
| M12: Permissions & Policy | High | Policy model complexity, evaluation logic |
| M13: Env & Secrets | Low | Scope switching UX |
| M14: API Key Mgmt | Low | Key display security |
| M15: Production Hardening | Medium | Security review |
| M16: Cloud Run Deployment | Medium | Infrastructure setup |

---

## Testing Strategy

### Unit Tests
- Component rendering tests (Lit)
- Middleware tests (Koa)
- Service tests (Hub client, NATS client)

### Integration Tests
- API proxy end-to-end
- SSE subscription flow
- OAuth flow with mock provider

### E2E Tests
- Full user flows (login → create agent → terminal)
- Playwright or Cypress
- Run against staging environment

### Manual Testing
- Cross-browser compatibility
- Mobile responsiveness
- Accessibility audit (WCAG 2.1 AA)

---

## References

- **Web Frontend Design:** `web-frontend-design.md`
- **Hub API:** `hub-api.md`
- **Server Implementation:** `server-implementation-design.md`
- **Hosted Architecture:** `hosted-architecture.md`
- **Authentication Design:** `authentication-design.md`
- **Permissions Design:** `permissions-design.md`
- **Hosted Templates:** `hosted-templates.md`
