---
title: Release Notes
---

## Feb 24, 2026

This release focuses on synchronizing JSON schemas with Go configuration structs and enhancing settings for GKE, soft-delete, and harness-agnostic templates.

### 🚀 Features
* **GKE Support:** Added a `gke` boolean to runtime configurations to enable GKE-specific features like Workload Identity.
* **Soft-Delete Lifecycle:** Introduced `soft_delete_retention` and `soft_delete_retain_files` to the Hub server configuration, allowing for temporary retention and recovery of deleted agents.
* **Harness-Agnostic Templates:** Added `agent_instructions`, `system_prompt`, and `default_harness_config` fields to the agent configuration schema to support role-based templates that work across different LLM harnesses.
* **Per-Agent Hub Overrides:** Agents can now specify a custom Hub endpoint in their `scion-agent.yaml`, overriding grove and server-level defaults.
* **Sidecar Environment Variables:** Added support for environment variables in auxiliary sidecar services.
* **Ready Check Enhancements:** Added a `delay` ready check type for services that simply need a fixed wait period.

### 🐛 Fixes
* **Schema Alignment:** Synchronized numerous field names between Go tags and JSON schemas, including camelCase fixes for `runtimeClassName`, `serviceAccountName`, and `auth_selectedType`.
* **Validation Suite:** Added comprehensive full-schema validation tests for both settings and agent configurations.

## Feb 22, 2026

This period introduced significant data management features, including agent soft-delete and centralized harness configuration storage, while advancing the secrets management and execution limits infrastructure.

### 🚀 Features
* **Agent Soft-Delete & Restore:** Implemented a complete soft-delete lifecycle for agents. This includes Hub-side archiving, a new `scion restore` command, list filtering for deleted agents, and an automated background purge loop for expired records.
* **Secrets-Gather & Interactive Input:** Enhanced the environment gathering pipeline to support "secrets-gather." Templates can now define required secrets, and the CLI provides interactive prompts to collect missing values, which are then securely backed by the configured secret provider.
* **K8s Native Secret Mounting:** Completed Phase 4 of the secrets strategy, enabling native secret mounting for agents running in Kubernetes. This includes support for GKE CSI drivers and robust fallback paths.
* **Harness Config Hub Storage:** Added Hub-resident storage for harness configurations. This enables centralized management (CRUD), CLI synchronization, and ensures configurations are consistently propagated to brokers during agent creation.
* **Agent Execution Limits:** Introduced Phase 1 of the agent limits infrastructure, including support for `max_turns` and `max_duration` constraints and a new `LIMITS_EXCEEDED` agent state.
* **CLI UX Improvements:** Added a `--all` flag to `scion stop` for bulk agent termination, introduced Hub auth verification with version reporting, and enhanced `scion look` with better visual padding and borders.
* **Web UI & Real-time Updates:** Launched a new "Create Agent" UI, optimized frontend performance by moving to explicit component imports, and enabled real-time grove list updates via Server-Sent Events (SSE).

### 🐛 Fixes
* **Provisioning Robustness:** Improved cleanup of provisioning agents during failed or cancelled environment gathering sessions to prevent stale container accumulation.
* **Sync & State Consistency:** Fixed a race condition where Hub synchronization could remove freshly created agents and ensured harness types are correctly propagated during agent sync.
* **Deployment Pipeline:** Corrected the build sequence in GCE deployment scripts to ensure web assets are fully compiled before the Go binary is built.
* **Config Resolution:** Fixed several configuration issues, including profile runtime application, grove flag resolution in subdirectories, and Hub environment variable suppression when the Hub is disabled.

## Feb 21, 2026

This period heavily focused on implementing the end-to-end "env-gather" flow to manage environment variables safely, alongside several CLI improvements and runtime fixes.

### 🚀 Features
* **Env-Gather Flow Pipeline:** Implemented a comprehensive environment variable gathering system across the CLI, Hub, and Broker. This includes harness-aware env key extraction, Hub 202 handling with submission endpoints, and broker-side evaluation to finalize the environment prior to agent creation.
* **Agent Context Threading:** Threaded the CLI hub endpoint directly to agent containers and added support for environment variable overrides.
* **Agent Dashboard Enhancements:** The agent details page now displays the `lastSeen` heartbeat as a relative time format.
* **Template Pathing:** Added support for `SCION_EXTRA_PATH` to optionally include template bin directories in the system `PATH`.
* **Build System Upgrades:** Overhauled the Makefile with new standard targets for build, install, test, lint, and web compilation.

### 🐛 Fixes
* **Env-Gather Safety & UX:** Added strict rejection of env-gather in non-interactive modes to prevent unsanctioned variable forwarding. Improved confirmation messaging and added dispatch support for grove-scoped agent creation.
* **CLI Output Formatting:** Redirected informational CLI output to `stderr` to ensure `stdout` can be piped cleanly as JSON.
* **Podman Performance:** Fixed slow container provisioning on Podman by directly editing `/etc/passwd` instead of using `usermod`.
* **Profile Parameter Routing:** Corrected the threading of the profile parameter from the CLI through the Hub to the runtime broker.
* **Hub API Accuracy:** The Hub API now correctly surfaces the `harness` type in responses for agent listings.
* **Docker Build Context:** Fixed an issue where the `scion-base` Docker image build was missing the web package context.

## Feb 20, 2026

This period focused heavily on unifying the Hub API and Web Server architectures, refactoring the agent status model, and enhancing the web frontend experience with new routing and pages.

### ⚠️ BREAKING CHANGES
* **Status Model:** Consolidated the `SessionStatus` field into the primary `Status` field across the codebase (API, Database, UI). The `WAITING_FOR_INPUT` and `COMPLETED` states are now treated as "sticky" statuses.
* **Server Architecture:** Combined the Hub API and Web server to serve on a single port (`8080`) when both are enabled. API traffic is now routed to `/api/v1/`, resolving CORS issues and simplifying local deployment.

### 🚀 Features
* **Web Frontend Enhancements:** Added a new Brokers list page, implemented full client-side routing for the Vite dev server, and unified OAuth provider detection via a new `/auth/providers` endpoint.
* **Agent Environment:** Added support for injecting harness-specific telemetry and hub environment variables directly into agent containers based on grove settings.
* **Git Operations:** Added cloning status indicators and improved git clone config parity during grove-scoped agent creation.

### 🐛 Fixes
* **Real-time UI Updates:** Fixed the Server-Sent Events (SSE) format to ensure real-time UI updates correctly broadcast agent state changes.
* **Routing & Port Prioritization:** Fixed port prioritization to use the web port for broker hub endpoints in combined mode, and ensured unhandled `/api/` routes return proper JSON 404 responses.
* **OAuth & Login:** Fixed conditional rendering for the `/login` route and correctly populated OAuth provider attributes during client-side navigation.
* **Container Configuration:** Fixed container image resolution from on-disk harness configurations and normalized YAML key parsing.
* **Status Reporting:** Ensured Hub status reporting correctly respects and preserves the newly unified, sticky statuses.

## Feb 19, 2026

This period represented a major architectural shift, consolidating the web server into a single Go binary, removing dependencies like NATS and Koa, and introducing hub-first remote workspaces via Git.

### ⚠️ BREAKING CHANGES
* **Secrets Management:** The system now strictly requires a configured production secret backend (e.g., `gcpsm`) for any secret Set operations across user, grove, and runtime broker scopes. Plaintext fallbacks have been removed. Read, list, and delete operations remain functional locally to support data migration.
* **Server Architecture:** The Node.js Koa server and NATS message broker dependencies have been completely retired. The Scion Hub now natively handles web frontend serving, SPA routing, and Server-Sent Events (SSE) via a consolidated Go binary.

### 🚀 Features
* **Hub-First Git Workspaces:** Implemented end-to-end support for creating remote workspaces directly from Git URLs. This integration enables git clone mode across `sciontool init` and the runtime broker pipeline.
* **Web Server & Auth Integration:** Introduced native session management and OAuth routing within the Go web server, alongside a new EventPublisher for real-time SSE streaming.
* **Telemetry & Settings:** Added telemetry injection to the `v1` settings schema. Telemetry configuration now supports hierarchical merging and is automatically bridged into the agent container's environment variables.
* **CLI Additions:** Introduced the `scion look` command for non-interactive terminal viewing. Project initialization now automatically sets up template directories and requires a global grove.

### 🐛 Fixes
* **Lifecycle Hooks:** Relocated the cleanup handler to container lifecycle hooks to guarantee reliable execution upon container termination.
* **Settings Overrides:** Fixed configuration parsing to ensure environment variable overrides are correctly applied when loaded from `settings.yaml`.
* **CLI Defaults:** Ensured the `update-default` command consistently targets the global grove, and introduced a new `--force` flag.
* **Frontend Assets:** Resolved static asset serving issues by removing an erroneous `StripPrefix` in the router, and fixed client entry point imports.
