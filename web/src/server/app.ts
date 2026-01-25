/**
 * Koa application setup
 *
 * Configures the Koa app with middleware stack and routes
 */

import Koa from 'koa';
import Router from '@koa/router';
import cors from '@koa/cors';
import bodyParser from 'koa-bodyparser';
import serve from 'koa-static';
import { resolve } from 'path';
import { fileURLToPath } from 'url';

import type { AppConfig } from './config.js';
import { errorHandler, logger, security, initDevAuth } from './middleware/index.js';
import { healthRoutes, pageRoutes, createApiRouter } from './routes/index.js';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

/**
 * Creates and configures the Koa application
 *
 * @param config - Application configuration
 * @returns Configured Koa application
 */
export function createApp(config: AppConfig): Koa {
  const app = new Koa();
  const router = new Router();

  // Trust proxy headers (for Cloud Run)
  app.proxy = true;

  // Core middleware stack
  // Order matters: error handler should be first to catch all errors
  app.use(errorHandler());
  app.use(logger(config));
  app.use(security(config));
  app.use(
    cors({
      origin: config.cors.origin,
      credentials: config.cors.credentials,
    })
  );

  // Body parsing for JSON requests
  app.use(bodyParser());

  // Dev auth middleware (auto-login for development)
  const devAuth = initDevAuth();
  app.use(devAuth.middleware);

  // Static asset serving from public/ directory
  // Path is relative to compiled location: dist/server/server/app.js
  // So we need to go up 3 levels to reach the project root
  const publicDir = resolve(__dirname, '../../../public');
  app.use(
    serve(publicDir, {
      maxage: config.production ? 86400000 : 0, // 24h in prod, no cache in dev
      gzip: true,
      brotli: true,
    })
  );

  // Mount health check routes
  router.use(healthRoutes.routes());
  router.use(healthRoutes.allowedMethods());

  // Mount API proxy routes
  const apiRouter = createApiRouter(config);
  router.use('/api', apiRouter.routes());
  router.use('/api', apiRouter.allowedMethods());

  // Mount SSR page routes (catch-all, should be last)
  router.use(pageRoutes.routes());
  router.use(pageRoutes.allowedMethods());

  // Apply router middleware
  app.use(router.routes());
  app.use(router.allowedMethods());

  return app;
}
