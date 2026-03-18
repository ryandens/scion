# Notification Subscriptions: Scoped Subscribe & Deduplication

## Status
**Design** | March 2026

## Problem

The current notification system only allows subscriptions to be created at agent-start time via the `--notify` flag. This creates several limitations:

1. **No post-creation subscription**: A user or agent cannot subscribe to notifications for an agent they did not create, or after the agent is already running.
2. **No grove-wide subscription**: There is no way to say "notify me about all agents in this grove" — subscriptions are strictly per-agent.
3. **No independent subscription management**: Subscriptions are a side-effect of `scion start --notify`, not a first-class resource with its own CRUD lifecycle.
4. **No deduplication across overlapping scopes**: If grove-wide subscriptions are added, a subscriber watching both a specific agent and its parent grove would receive duplicate notifications.

### Motivating Scenarios

**Scenario 1: Human monitoring a grove**
A user wants to be notified whenever any agent in their project grove completes or needs input, without having to subscribe individually to each agent.

**Scenario 2: Lead agent subscribing to a peer**
Agent A is a coordinator. Agent B was started by a different entity. Agent A wants to subscribe to Agent B's status to react when it completes.

**Scenario 3: Overlapping subscriptions**
Agent A subscribes to all events for Agent B, Agent C, and Grove Foo. Agent C is in Grove Foo. When Agent C completes, Agent A should receive exactly one notification — not two (one from the agent subscription and one from the grove subscription).

---

## Current Architecture

### Subscription Model (as-is)

```go
// pkg/store/models.go
type NotificationSubscription struct {
    ID                string    // UUID
    AgentID           string    // Agent being watched (always required today)
    SubscriberType    string    // "agent" | "user"
    SubscriberID      string    // Slug or ID of the subscriber
    GroveID           string    // Grove scope (used for authorization)
    TriggerActivities []string  // e.g. ["COMPLETED", "WAITING_FOR_INPUT"]
    CreatedAt         time.Time
    CreatedBy         string
}
```

### Current Subscription Creation Flow

1. `scion start --notify` sets `CreateAgentRequest.Notify = true`
2. Hub handler resolves subscriber identity from JWT
3. Creates a `NotificationSubscription` with `AgentID` = newly created agent
4. `NotificationDispatcher.handleEvent()` queries `GetNotificationSubscriptions(agentID)` to find matching subscriptions

### Key Constraint

The `notification_subscriptions` table has `agent_id TEXT NOT NULL` with a foreign key to `agents(id)`. Every subscription is scoped to a single agent. The dispatcher only queries by `agent_id`.

---

## Design

### Subscription Scope Model

Introduce an explicit scope to subscriptions. A subscription targets either a single agent or an entire grove:

```go
const (
    SubscriptionScopeAgent = "agent"  // Watch a specific agent
    SubscriptionScopeGrove = "grove"  // Watch all agents in a grove
)

type NotificationSubscription struct {
    ID                string
    Scope             string    // "agent" or "grove" (NEW)
    AgentID           string    // Required when Scope="agent", empty when Scope="grove"
    SubscriberType    string    // "agent" | "user"
    SubscriberID      string
    GroveID           string    // Always required (grove context)
    TriggerActivities []string
    CreatedAt         time.Time
    CreatedBy         string
}
```

### Schema Changes

```sql
-- Migration: alter notification_subscriptions
-- 1. Make agent_id nullable (grove-scoped subscriptions have no agent_id)
-- 2. Add scope column
-- 3. Add unique constraint to prevent duplicate subscriptions

ALTER TABLE notification_subscriptions ADD COLUMN scope TEXT NOT NULL DEFAULT 'agent';

-- Drop the existing NOT NULL on agent_id by recreating the table
-- (SQLite doesn't support ALTER COLUMN)
CREATE TABLE notification_subscriptions_new (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL DEFAULT 'agent',       -- 'agent' | 'grove'
    agent_id TEXT,                              -- NULL for grove-scoped
    subscriber_type TEXT NOT NULL,
    subscriber_id TEXT NOT NULL,
    grove_id TEXT NOT NULL,
    trigger_activities TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by TEXT NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

-- Unique constraint: one subscription per (scope, target, subscriber, activities-hash)
-- prevents exact duplicates but allows different activity sets
CREATE UNIQUE INDEX idx_notification_subs_unique
    ON notification_subscriptions(scope, COALESCE(agent_id, ''), subscriber_type, subscriber_id, grove_id);

CREATE INDEX idx_notification_subs_agent ON notification_subscriptions(agent_id);
CREATE INDEX idx_notification_subs_grove ON notification_subscriptions(grove_id);
CREATE INDEX idx_notification_subs_subscriber ON notification_subscriptions(subscriber_type, subscriber_id);
```

### Dispatcher Changes: Grove-Scoped Matching + Deduplication

The `handleEvent()` method currently only queries agent-level subscriptions. It must also query grove-level subscriptions and deduplicate:

```go
func (nd *NotificationDispatcher) handleEvent(evt Event) {
    // ... unmarshal event ...

    // Collect subscriptions from both scopes
    agentSubs, _ := nd.store.GetNotificationSubscriptions(ctx, statusEvt.AgentID)
    groveSubs, _ := nd.store.GetNotificationSubscriptionsByGroveScope(ctx, statusEvt.GroveID)
    allSubs := append(agentSubs, groveSubs...)

    // Deduplicate: one notification per (subscriber_type, subscriber_id)
    seen := make(map[string]bool)
    for _, sub := range allSubs {
        dedupeKey := sub.SubscriberType + ":" + sub.SubscriberID
        if seen[dedupeKey] {
            continue
        }
        if !sub.MatchesActivity(matchStatus) {
            continue
        }
        // ... dedup against last notification status ...
        seen[dedupeKey] = true
        nd.storeAndDispatch(ctx, &sub, statusEvt)
    }
}
```

**Deduplication strategy**: When an event matches multiple subscriptions for the same subscriber (e.g., agent-scoped + grove-scoped), only the first matching subscription generates a notification. Agent-scoped subscriptions take priority (queried first) since they are more specific.

### New Store Methods

```go
// GetNotificationSubscriptionsByGroveScope returns grove-scoped subscriptions
// (scope='grove') for a given grove, as opposed to GetNotificationSubscriptionsByGrove
// which returns ALL subscriptions within a grove regardless of scope.
GetNotificationSubscriptionsByGroveScope(ctx context.Context, groveID string) ([]NotificationSubscription, error)

// GetSubscriptionsForSubscriber returns all subscriptions owned by a subscriber.
GetSubscriptionsForSubscriber(ctx context.Context, subscriberType, subscriberID string) ([]NotificationSubscription, error)
```

---

## API Endpoints

### New Endpoints

```
POST   /api/v1/notifications/subscriptions          — Create a subscription
GET    /api/v1/notifications/subscriptions          — List subscriptions for the caller
DELETE /api/v1/notifications/subscriptions/{id}     — Delete a subscription
```

### Create Subscription

```
POST /api/v1/notifications/subscriptions
```

**Request body:**
```json
{
    "scope": "agent",
    "agentId": "uuid-of-agent",
    "groveId": "uuid-of-grove",
    "triggerActivities": ["COMPLETED", "WAITING_FOR_INPUT", "LIMITS_EXCEEDED"]
}
```

Or for grove-wide:
```json
{
    "scope": "grove",
    "groveId": "uuid-of-grove",
    "triggerActivities": ["COMPLETED", "WAITING_FOR_INPUT"]
}
```

**Validation:**
- `scope` must be `"agent"` or `"grove"`
- `agentId` required when scope is `"agent"`, must be empty when scope is `"grove"`
- `groveId` always required
- `triggerActivities` must be non-empty, values must be from allowed set
- Subscriber identity resolved from JWT (same pattern as `--notify`)
- Duplicate detection: if an identical subscription already exists, return the existing one (idempotent)

**Response:** `201 Created` with the subscription object.

### List Subscriptions

```
GET /api/v1/notifications/subscriptions
    ?groveId=...          (optional, filter by grove)
    ?agentId=...          (optional, filter by watched agent)
    ?scope=agent|grove    (optional, filter by scope)
```

Returns subscriptions owned by the authenticated caller.

### Delete Subscription

```
DELETE /api/v1/notifications/subscriptions/{id}
```

Only the subscription owner (or hub admin) can delete. Returns `204 No Content`.

---

## CLI Design

All subscription management lives under a new `scion notifications` command group. Usage requires hub mode — non-hub invocations return an error.

### Command Structure

```
scion notifications                           — List your notifications (alias for current `scion hub notifications`)
scion notifications ack [id]                  — Acknowledge notification(s) (--all flag for bulk)
scion notifications subscribe                 — Create a subscription
scion notifications unsubscribe [id]          — Remove a subscription
scion notifications subscriptions             — List your subscriptions
```

### Subscribe Command

```bash
# Subscribe to a specific agent
scion notifications subscribe --agent <agent-name-or-id> --grove <grove>

# Subscribe to all agents in a grove
scion notifications subscribe --grove <grove>

# Subscribe with specific triggers (default: COMPLETED, WAITING_FOR_INPUT, LIMITS_EXCEEDED)
scion notifications subscribe --agent <agent> --grove <grove> --triggers COMPLETED,WAITING_FOR_INPUT

# Subscribe from within an agent context (uses JWT identity)
scion notifications subscribe --agent <peer-agent-slug>
```

**Behavior:**
- `--grove` is always required (can be inferred from current context like other commands)
- If only `--grove` is provided (no `--agent`), creates a grove-scoped subscription
- If `--agent` is provided, creates an agent-scoped subscription
- `--triggers` defaults to `COMPLETED,WAITING_FOR_INPUT,LIMITS_EXCEEDED`
- Idempotent — re-subscribing with same parameters returns existing subscription

### Unsubscribe Command

```bash
# Unsubscribe by subscription ID
scion notifications unsubscribe <subscription-id>

# Unsubscribe from everything in a grove
scion notifications unsubscribe --grove <grove> --all
```

### List Subscriptions

```bash
# List all your subscriptions
scion notifications subscriptions

# Filter by grove
scion notifications subscriptions --grove <grove>

# JSON output
scion notifications subscriptions --json
```

**Output format:**
```
ID          SCOPE    TARGET           GROVE       TRIGGERS                                    CREATED
a1b2c3d4    agent    my-agent         my-project  COMPLETED,WAITING_FOR_INPUT,LIMITS_EXCEEDED  2026-03-18
e5f6g7h8    grove    (all agents)     my-project  COMPLETED,WAITING_FOR_INPUT                  2026-03-17
```

### Hub Requirement

```go
func runNotificationsSubscribe(cmd *cobra.Command, args []string) error {
    settings, err := config.LoadSettings(resolvedPath)
    if err != nil {
        return fmt.Errorf("failed to load settings: %w", err)
    }
    if !settings.IsHubEnabled() {
        return fmt.Errorf("notifications require Hub mode. Enable with 'scion hub enable <endpoint>'")
    }
    // ...
}
```

### Migration from `scion hub notifications`

The existing `scion hub notifications` and `scion hub notifications ack` commands move to the new `scion notifications` group. No aliases or deprecation notices — the old `hub notifications` path is simply removed.
---

## Web UX

### Subscription Management Panel

A new "Subscriptions" section appears in two locations:

#### 1. Agent Detail Page — "Subscribe" Action Button

On the agent detail page, add a "Subscribe" button to the header actions area (alongside existing actions like Stop, Delete, etc.):

- **Bell icon button**: Toggles subscription for the current user to this agent
- **State**: Filled bell = subscribed, outline bell = not subscribed
- **Click behavior**:
  - If not subscribed → creates agent-scoped subscription with default triggers
  - If subscribed → shows popover with current subscription details and "Unsubscribe" option
  - Allows editing trigger activities via checkboxes

#### 2. Grove Detail Page — Subscriptions Section

On the grove detail page, add a "Notification Subscriptions" card/section:

- **"Subscribe to Grove" button**: Creates a grove-scoped subscription
- **Subscription list table**: Shows all subscriptions the current user has within this grove

Table columns:
```
SCOPE       TARGET          TRIGGERS                         ACTIONS
agent       worker-agent    COMPLETED, WAITING_FOR_INPUT     [Edit] [Delete]
grove       (all agents)    COMPLETED                        [Edit] [Delete]
```

- **Edit**: Opens an inline popover (anchored to the row) with checkboxes for trigger activities, allowing the user to update which events fire notifications for that subscription. Saves via PATCH on dismiss.
- **Delete**: Remove subscription with confirmation

#### 3. Notification Tray Enhancement

The existing notification tray in the header already works well. Minimal changes needed:

- Notification items can include a small scope indicator (agent icon vs grove icon) to show where the subscription came from
- "Manage subscriptions" link at the bottom of the tray popover, navigating to a subscriptions list page

#### Component: `subscription-manager.ts`

A reusable Lit component following the `env-var-list.ts` CRUD pattern:

```typescript
@customElement('scion-subscription-manager')
export class ScionSubscriptionManager extends LitElement {
    @property() groveId: string;
    @property() agentId?: string;  // If provided, shows only agent-scoped
    @state() private subscriptions: Subscription[] = [];
    @state() private dialogOpen = false;
    // ... CRUD operations against /api/v1/notifications/subscriptions
}
```

**API calls:**
```
GET    /api/v1/notifications/subscriptions?groveId=...
POST   /api/v1/notifications/subscriptions
DELETE /api/v1/notifications/subscriptions/{id}
```

---

## Alternatives Considered

### Alternative A: Expand `--notify` to Accept Targets

Instead of a separate subscription API, extend `--notify` to accept a target:

```bash
scion start --notify=agent:coordinator fooagent "Do the thing"
```

**Rejected because:**
- Only works at agent creation time — doesn't solve post-creation subscription
- Doesn't address grove-wide subscriptions
- Conflates agent creation with subscription management
- The `--notify` flag should remain simple (boolean, "notify me")

### Alternative B: Topic-Based Pub/Sub (NATS-Style)

Model subscriptions as topic subscriptions on the event bus:

```
# Subscribe to: grove.{id}.agent.status
scion subscribe grove.mygrove.agent.status
```

**Rejected because:**
- Exposes internal event bus topology to users
- Doesn't provide deduplication semantics
- Doesn't fit the "notification" mental model (users think in terms of agents and groves, not event subjects)
- Would require a separate persistence layer anyway for durable subscriptions

### Alternative C: Grove-Level Setting (Auto-Notify All)

Add a grove setting `auto_notify: true` that automatically subscribes the grove owner to all agent events:

```bash
scion grove settings set auto-notify true
```

**Rejected because:**
- Too coarse — no per-user or per-agent granularity
- Doesn't support agent-to-agent subscriptions
- Settings are a different abstraction than subscriptions
- Could be built later on top of the subscription model as a convenience feature

### Alternative D: Wildcard AgentID in Current Schema

Use a sentinel value (e.g., `"*"`) in the existing `agent_id` column to mean "all agents in grove":

```sql
INSERT INTO notification_subscriptions (agent_id, grove_id, ...)
VALUES ('*', 'grove-uuid', ...);
```

**Rejected because:**
- Breaks the foreign key constraint (`agent_id REFERENCES agents(id)`)
- Sentinel values are fragile and error-prone
- Query patterns become awkward (need `WHERE agent_id = ? OR agent_id = '*'`)
- A proper `scope` column is cleaner and more extensible

---

## Design Decisions

### 1. Subscription Limits
Should there be a maximum number of subscriptions per subscriber? A user subscribing to 1000 individual agents would create query overhead. **Decision**: No limits initially. Grove-scoped subscriptions naturally reduce the need for many individual subscriptions. A configurable cap can be added later if needed.

### 2. Trigger Activity Editing
Should updating trigger activities on an existing subscription be supported, or should users delete and recreate? **Decision**: Support a PATCH endpoint for modifying `triggerActivities` on an existing subscription. This is cleaner UX than delete+recreate.

### 3. Subscription Persistence After Agent Deletion
Agent-scoped subscriptions cascade-delete when the agent is deleted (via FK constraint). Should there be a notification that the watched agent was deleted? **Decision**: Yes — treat agent deletion as a notification-worthy event (new trigger activity: `DELETED`). The notification is generated before the cascade delete removes the subscription. This aligns naturally with the existing soft-delete model where deletion is a state change.

### 4. Authorization Model
Who can subscribe to what?
- **Users**: Any authenticated user can subscribe to any agent/grove they have read access to
- **Agents**: Can subscribe to any agent in their grove (same grove only, enforced by JWT scope)
- **Cross-grove subscriptions**: Not supported initially

**Decision**: Follow existing authorization patterns — use grove membership or read access as the gate.

### 5. `--notify` Flag Behavior
Should `--notify` continue to create subscriptions as a side-effect, or should it be refactored to call the subscription API internally? **Decision**: Refactor `--notify` to use the new subscription API internally. This unifies the code path, reduces tech debt, and ensures `--notify` subscriptions appear in `scion notifications subscriptions` listings.

### 6. Notification Channels for Subscriptions
Should grove-scoped subscriptions also dispatch to external channels (Slack, webhooks)? **Decision**: Yes — channel dispatch is subscriber-type-dependent (user subscribers get channels), not scope-dependent. Both agent-scoped and grove-scoped subscriptions for user subscribers should dispatch to channels. Notification channels should support brokered channel providers via the broker plugin system.

### 7. CLI Command Placement
The design places commands under `scion notifications` (top-level). An alternative is `scion hub notifications subscribe`. **Decision**: Top-level `scion notifications` — it's shorter, notifications are a platform feature (not a "hub admin" action), and the common case is subscribing within the same grove.

---

## Implementation Phases

### Phase 1: Core Subscription API + Dispatcher Changes ✅ COMPLETE
**Goal**: Subscriptions are a first-class resource with CRUD API and grove-scoped matching with deduplication.

**Tasks:**
1. ✅ Add `scope` column to `NotificationSubscription` model and DB schema migration
2. ✅ Make `agent_id` nullable in the schema (for grove-scoped subscriptions)
3. ✅ Add `GetNotificationSubscriptionsByGroveScope()` and `GetSubscriptionsForSubscriber()` store methods
4. ✅ Implement SQLite store methods
5. ✅ Add unique constraint for duplicate prevention
6. ✅ Update `NotificationDispatcher.handleEvent()` with grove-scope query and deduplication logic
7. ✅ Add HTTP handlers: `POST/GET/DELETE /api/v1/notifications/subscriptions`
8. ✅ Add hubclient `SubscriptionService` interface and implementation
9. ✅ Write unit and integration tests
10. ✅ Refactor `--notify` flag in `createAgent` handler to use the new subscription creation path

### Phase 2: CLI Commands ✅ COMPLETE
**Goal**: Users and agents can manage subscriptions from the command line.

**Tasks:**
1. ✅ Create `scion notifications` command group with hub-required check
2. ✅ Implement `scion notifications subscribe` with `--agent`, `--grove`, `--triggers` flags
3. ✅ Implement `scion notifications unsubscribe` command
4. ✅ Implement `scion notifications subscriptions` listing command
5. ✅ Move `scion hub notifications` / `scion hub notifications ack` to the new `scion notifications` group (old commands deprecated with redirects)
6. ✅ Write CLI integration tests

### Phase 3: Web UX ✅ COMPLETE
**Goal**: Users can manage subscriptions and see subscription context in the browser.

**Tasks:**
1. ✅ Add `Subscription` type to `web/src/shared/types.ts`
2. ✅ Create `subscription-manager.ts` shared component (CRUD table + dialog)
3. ✅ Add "Subscribe" bell button to agent detail page header
4. ✅ Add "Notification Subscriptions" section to grove detail page
5. ✅ Add "Manage subscriptions" link to notification tray popover
6. Wire up SSE events for real-time subscription state updates (deferred — optional)
7. ✅ Add scope indicator styles to notification tray (scope data in notification payloads deferred to Phase 4)

### Phase 4: Polish & Extensions (Future)
**Goal**: Refinements based on usage feedback.

**Tasks:**
1. Subscription editing (PATCH trigger activities)
2. `DELETED` trigger activity for agent deletion events
3. Subscription limits / rate limiting
4. Bulk subscribe/unsubscribe operations
5. Subscription templates (pre-configured activity sets for common patterns)
6. Email notification channel integration

---

## Summary

This design extends the notification system from a start-time-only, single-agent model to a flexible subscription system with two scopes (agent, grove), subscriber-level deduplication, full CRUD lifecycle, and management through both CLI and web UI. The core changes are modest — a new column and query path in the dispatcher, new API endpoints, and new CLI commands — while preserving full backward compatibility with the existing `--notify` flag.
