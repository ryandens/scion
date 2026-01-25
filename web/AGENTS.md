# Scion Web Frontend - Agent Instructions

This document provides instructions for AI agents working on the Scion Web Frontend.

## Design Documents

Before making changes, review the relevant design documentation:

- **[Web Frontend Design](../.design/hosted/web-frontend-design.md)** - Architecture, technology stack, component patterns
- **[Frontend Milestones](../.design/hosted/frontend-milestones.md)** - Implementation phases and test criteria

## Development Workflow

### Starting the Development Server

```bash
cd web
npm install    # First time only, or after package.json changes

# Option 1: Full build and run (recommended)
npm run build && npm start

# Option 2: Development with client assets built
npm run dev:full

# Option 3: Server only (client assets will be placeholders)
npm run build:server && npm start

# Option 4: Development mode with tsx (may have ESM compatibility issues)
npm run dev
```

**Note:** Options 1 and 2 include building the client-side JavaScript. Without building the client, navigation and interactive features will not work properly.

### Common Commands

| Command | Purpose |
|---------|---------|
| `npm run dev` | Start development server with tsx (hot reload) |
| `npm run dev:full` | Build client assets, then start dev server |
| `npm run build` | Build both server and client for production |
| `npm run build:server` | Build server-side TypeScript |
| `npm run build:client` | Build client-side with Vite |
| `npm start` | Run the production build |
| `npm run lint` | Check for linting errors |
| `npm run lint:fix` | Auto-fix linting errors |
| `npm run format` | Format code with Prettier |
| `npm run typecheck` | Run TypeScript type checking |

### Verifying Changes

After making changes, verify:

1. **Type checking passes:** `npm run typecheck`
2. **Linting passes:** `npm run lint`
3. **Server builds:** `npm run build:server`
4. **Server starts:** `npm start`
5. **Health endpoint works:** `curl localhost:8080/healthz`
6. **SSR works:** `curl localhost:8080/` (should return full HTML with Lit components)

## Project Structure

```
web/
├── src/
│   ├── server/           # Koa server code
│   │   ├── index.ts      # Entry point
│   │   ├── app.ts        # Koa app configuration
│   │   ├── config.ts     # Environment config
│   │   ├── middleware/   # Koa middleware
│   │   ├── routes/       # Route handlers
│   │   │   ├── health.ts # Health check endpoints
│   │   │   └── pages.ts  # SSR page routes
│   │   └── ssr/          # Server-side rendering
│   │       ├── renderer.ts  # Lit SSR renderer
│   │       └── templates.ts # HTML shell templates
│   ├── client/           # Browser-side code
│   │   └── main.ts       # Client entry point (hydration)
│   ├── components/       # Lit web components
│   │   ├── index.ts      # Component exports
│   │   ├── app-shell.ts  # Main application shell
│   │   ├── shared/       # Reusable UI components
│   │   │   ├── index.ts      # Shared component exports
│   │   │   ├── nav.ts        # Sidebar navigation
│   │   │   ├── header.ts     # Top header bar
│   │   │   ├── breadcrumb.ts # Breadcrumb navigation
│   │   │   └── status-badge.ts # Status indicator badges
│   │   └── pages/        # Page components
│   │       ├── home.ts   # Dashboard page
│   │       └── not-found.ts # 404 page
│   ├── styles/           # CSS theme and utilities
│   │   ├── theme.css     # CSS custom properties, light/dark mode
│   │   └── utilities.css # Utility classes
│   └── shared/           # Shared types between server/client
│       └── types.ts      # Type definitions
├── public/               # Static assets
│   └── assets/           # CSS, JS, images
├── dist/                 # Build output (gitignored)
│   └── server/           # Compiled server code
└── package.json
```

## Technology Stack

- **Server:** Koa 2.x with TypeScript
- **Components:** Lit 3.x with decorators
- **UI Library:** Web Awesome / Shoelace (Milestone 3)
- **Build:** Vite for client, tsc for server
- **SSR:** @lit-labs/ssr with declarative shadow DOM
- **Routing:** @vaadin/router (client-side)

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `HOST` | `0.0.0.0` | Server hostname |
| `NODE_ENV` | `development` | Environment mode |
| `HUB_API_URL` | `http://localhost:9810` | Hub API endpoint |
| `SCION_API_DEBUG` | `false` | Enable debug logging (verbose request/response logs) |
| `SCION_DEV_TOKEN` | - | Explicit dev token (overrides file) |
| `SCION_DEV_TOKEN_FILE` | `~/.scion/dev-token` | Path to dev token file |
| `SCION_DEV_AUTH_ENABLED` | `true` in dev | Enable/disable dev auth |

## Development Authentication

The web frontend supports automatic development authentication when running locally. This provides a seamless developer experience without needing to set up OAuth.

### How It Works

1. When the server starts in development mode (non-production), it looks for a dev token:
   - First from `SCION_DEV_TOKEN` environment variable
   - Then from `~/.scion/dev-token` file

2. If a token is found:
   - The user is automatically logged in as "Development User" (`dev@localhost`)
   - API proxy requests include the dev token in the `Authorization` header
   - The UI shows the logged-in user with a dropdown menu

3. The Hub API generates the dev token on startup. The token is saved to `~/.scion/dev-token`.

### Testing Dev Auth

```bash
# If Hub is running and created the token:
cat ~/.scion/dev-token

# Or create a test token manually:
echo "scion_dev_testtoken12345678901234567890abcd" > ~/.scion/dev-token
chmod 600 ~/.scion/dev-token

# Start the web frontend
npm run build && npm start
```

The server will show a warning banner confirming dev auth is enabled with the auto-login user.

See [dev-auth design](../.design/hosted/dev-auth.md) for the complete specification.

## Milestone Progress

Track implementation progress in the [frontend milestones](../.design/hosted/frontend-milestones.md) document.

Current status:
- ✅ **M1: Koa Server Foundation** - Complete
- ✅ **M2: Lit SSR Integration** - Complete
- ✅ **M3: Web Awesome Component Library** - Complete
- ⬜ M4: Authentication Flow
- ⬜ M5: Hub API Proxy
- ⬜ M6: Grove & Agent Pages
- ⬜ M7: SSE + NATS Real-Time Updates
- ⬜ M8: Terminal Component
- ⬜ M9: Agent Creation Workflow
- ⬜ M10: Production Hardening
- ⬜ M11: Cloud Run Deployment

## Key Patterns

### Adding a New Page

1. Create component in `src/components/pages/`
2. Register in `src/server/ssr/renderer.ts` (import and add to `getPageTemplate`)
3. Import in `src/client/main.ts` for client-side hydration
4. Add route pattern to `isKnownRoute()` in `src/server/routes/pages.ts`

### Adding a New Route

1. Create route file in `src/server/routes/`
2. Export from `src/server/routes/index.ts`
3. Mount in `src/server/app.ts`

### Adding Middleware

1. Create middleware in `src/server/middleware/`
2. Export from `src/server/middleware/index.ts`
3. Add to middleware stack in `src/server/app.ts` (order matters!)

### Creating Lit Components

Components use standard Lit patterns with TypeScript decorators:

```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('my-component')
export class MyComponent extends LitElement {
  @property({ type: String })
  myProp = 'default';

  static override styles = css`
    :host { display: block; }
  `;

  override render() {
    return html`<div>${this.myProp}</div>`;
  }
}
```

### Error Handling

Use `HttpError` from `middleware/error-handler.ts` for known errors:

```typescript
import { HttpError } from '../middleware/error-handler.js';

throw new HttpError(404, 'Resource not found', 'NOT_FOUND');
```

## SSR Considerations

- All components must be imported on the server for SSR to work
- Use declarative shadow DOM (`<template shadowroot="open">`)
- Initial data is serialized in `<script id="__SCION_DATA__">` tag
- Client hydrates components and sets up routing on load

## Shoelace Component Library

The application uses [Shoelace](https://shoelace.style/) for UI components. Shoelace is loaded from CDN in the HTML template.

### Using Shoelace Components

```typescript
// Use Shoelace components directly in Lit templates
render() {
  return html`
    <sl-button variant="primary" @click=${() => this.handleClick()}>
      <sl-icon slot="prefix" name="plus-lg"></sl-icon>
      Create Agent
    </sl-button>

    <sl-badge variant="success">Running</sl-badge>

    <sl-dropdown>
      <sl-button slot="trigger">Menu</sl-button>
      <sl-menu>
        <sl-menu-item>Option 1</sl-menu-item>
        <sl-menu-item>Option 2</sl-menu-item>
      </sl-menu>
    </sl-dropdown>
  `;
}
```

### Using Shared Scion Components

```typescript
import '../shared/status-badge.js';

render() {
  return html`
    <scion-status-badge status="running" size="small"></scion-status-badge>
    <scion-status-badge status="error" label="Failed"></scion-status-badge>
  `;
}
```

### Theme Variables

Use CSS custom properties with the `--scion-` prefix for consistent theming:

```css
:host {
  background: var(--scion-surface);
  color: var(--scion-text);
  border: 1px solid var(--scion-border);
  border-radius: var(--scion-radius);
}

.primary-action {
  background: var(--scion-primary);
  color: white;
}

.primary-action:hover {
  background: var(--scion-primary-hover);
}
```

### Dark Mode

Dark mode is handled automatically via CSS custom properties. The theme toggle in the navigation saves the preference to localStorage. Components should use the semantic color variables (e.g., `--scion-surface`, `--scion-text`) which automatically adjust for dark mode.

### Adding a Shared Component

1. Create component in `src/components/shared/`
2. Export from `src/components/shared/index.ts`
3. Import in `src/server/ssr/renderer.ts` for SSR
4. Import in `src/client/main.ts` for hydration
5. Add to `customElements.whenDefined()` list in client main.ts
