---
title: Authentication & Identity
description: Configuring authentication flows for Scion.
---

Scion supports multiple authentication methods for different use cases:

- **OAuth (Google/GitHub)**: For production web and CLI authentication
- **Development Auth**: For local development and testing
- **API Keys**: For programmatic access and CI/CD pipelines

## OAuth Authentication

Scion supports OAuth authentication via Google and GitHub. OAuth credentials are configured separately for web and CLI clients due to different redirect URI requirements.

### Web OAuth Setup

Configure web OAuth with these environment variables:

```bash
export SCION_SERVER_OAUTH_WEB_GOOGLE_CLIENTID="your-client-id"
export SCION_SERVER_OAUTH_WEB_GOOGLE_CLIENTSECRET="your-client-secret"
export SCION_SERVER_OAUTH_WEB_GITHUB_CLIENTID="your-client-id"
export SCION_SERVER_OAUTH_WEB_GITHUB_CLIENTSECRET="your-client-secret"
```

### CLI OAuth Setup

Configure CLI OAuth with these environment variables:

```bash
export SCION_SERVER_OAUTH_CLI_GOOGLE_CLIENTID="your-client-id"
export SCION_SERVER_OAUTH_CLI_GOOGLE_CLIENTSECRET="your-client-secret"
export SCION_SERVER_OAUTH_CLI_GITHUB_CLIENTID="your-client-id"
export SCION_SERVER_OAUTH_CLI_GITHUB_CLIENTSECRET="your-client-secret"
```

## Domain Authorization

You can restrict authentication to specific email domains using the `SCION_AUTHORIZED_DOMAINS` setting. This provides an additional layer of access control beyond OAuth authentication.

### Configuration

Set the environment variable with a comma-separated list of allowed domains:

```bash
# Allow only users from these domains
export SCION_AUTHORIZED_DOMAINS="example.com,mycompany.org"
```

Or configure in `server.yaml`:

```yaml
auth:
  authorizedDomains:
    - example.com
    - mycompany.org
```

### Behavior

- **Empty list (default)**: All email domains are allowed
- **Non-empty list**: Only emails from listed domains can authenticate
- **Case insensitive**: `Example.COM` matches `example.com`
- **Exact match**: Subdomains must be listed explicitly (`sub.example.com` does not match `example.com`)

### Enforcement

Domain authorization is enforced at multiple layers:

1. **Web Frontend**: Checked during OAuth callback before creating a session
2. **Hub API**: Checked at login/token endpoints before issuing tokens

If a user's email domain is not authorized, they receive an error message.

## Development Authentication

For local development and testing, Scion provides a zero-configuration authentication mode.

### Enabling Dev Auth

Start the server with the `--dev-auth` flag:

```bash
scion server start --enable-hub --dev-auth
```

The server will generate a token and display it:

```
WARNING: Development authentication enabled - not for production use
Dev token: scion_dev_a1b2c3d4e5f6789012345678901234567890abcd

To authenticate CLI commands, run:
  export SCION_DEV_TOKEN=scion_dev_a1b2c3d4e5f6789012345678901234567890abcd
```

### Using the Dev Token

```bash
# Set the token in your environment
export SCION_DEV_TOKEN=scion_dev_...

# CLI commands will automatically use it
scion hub status
```

The token is also saved to `~/.scion/dev-token` for automatic resolution.

## API Keys

For programmatic access, users can create API keys through the web dashboard or CLI.

### Creating an API Key

```bash
scion auth api-key create --name "CI Pipeline"
```

### Using an API Key

```bash
# Via Authorization header
curl -H "Authorization: Bearer sk_live_..." https://hub.example.com/api/v1/agents

# Via X-API-Key header
curl -H "X-API-Key: sk_live_..." https://hub.example.com/api/v1/agents
```

## Security Best Practices

1. **Use OAuth in production** - Dev auth is for development only
2. **Configure authorized domains** - Restrict access to your organization's email domains
3. **Use HTTPS** - All authentication should occur over encrypted connections
4. **Rotate API keys** - Periodically rotate API keys for long-running integrations
5. **Limit API key scopes** - Grant only necessary permissions to API keys
