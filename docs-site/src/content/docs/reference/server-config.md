---
title: Server Configuration (Hub & Runtime Host)
---

This document describes the configuration for the Scion Hub (State Server) and the Scion Runtime Host services.

## Purpose
Server configuration controls the operational behavior of the Scion backend components in a "Hosted" or distributed architecture. This includes network settings, database connections, and security configurations.

## Locations
- **Config File**: `~/.scion/server.yaml` or `./server.yaml`.
- **Environment Variables**: Overridden using the `SCION_SERVER_` prefix.

## Key Sections
- **Hub**: Configuration for the central Hub API server (Port, Host, Timeout, CORS).
- **RuntimeHost**: Configuration for the execution host service (Enabled status, Hub endpoint, Host ID).
- **Database**: Connection settings for the persistence layer (SQLite or PostgreSQL).
- **Auth**: Settings for development authentication, tokens, and domain authorization.
- **Logging**: Log level and format (Text or JSON) for system observability.

## Environment Variables
Server settings use a nested naming convention for environment variables. For example, `SCION_SERVER_HUB_PORT` maps to the `hub.port` setting, and `SCION_SERVER_DATABASE_DRIVER` maps to `database.driver`.

### Authentication Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SCION_AUTHORIZED_DOMAINS` | Comma-separated list of email domains allowed to authenticate. Empty allows all. | (empty) |
| `SCION_SERVER_AUTH_DEVMODE` | Enable development authentication mode. | `false` |
| `SCION_SERVER_AUTH_DEVTOKEN` | Explicit development token value. | (auto-generated) |

### Domain Authorization

To restrict authentication to specific email domains:

```bash
export SCION_AUTHORIZED_DOMAINS="example.com,mycompany.org"
```

Or in `server.yaml`:

```yaml
auth:
  authorizedDomains:
    - example.com
    - mycompany.org
```

This setting is enforced at both the web frontend and Hub API layers.
