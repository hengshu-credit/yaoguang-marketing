# Compact Compose development and source hot reload design

## Goal

The default local workflow must run Notifuse from bind-mounted source without rebuilding the production image after every edit. It must collapse the seven Go runtime roles into one backend process, keep the two React applications in one frontend container, use one browser entry point, and support a hot-reload switch that defaults to enabled.

The production image and independently scalable high-availability topology remain immutable and source-free.

## Topologies

### Default development topology

`docker compose up -d` starts eight long-running services:

1. `backend`: the Go application with `NOTIFUSE_ROLE=all`;
2. `frontend`: the Console and Notification Center development gateway;
3. PostgreSQL;
4. PgBouncer;
5. RabbitMQ;
6. Redis;
7. ClickHouse;
8. MinIO.

RabbitMQ and MinIO initialization jobs remain short-lived setup tasks and are not operational services.

The repository is bind-mounted at `/workspace` in both application containers. Go build caches, npm caches, `node_modules`, and generated frontend output use named volumes so Linux dependencies do not leak into the Windows checkout.

### High-availability topology

The current source-free topology moves to `compose.ha.yaml`. It continues to use the production `Dockerfile` and separate `api`, `outbox-relay`, `rule-worker`, `journey-worker`, `delivery-worker`, `analytics-worker`, and `scheduler` containers.

It starts with:

```powershell
docker compose -f compose.ha.yaml up -d --build
```

No development entrypoint, source mount, npm volume, or file watcher is present in this topology.

## Development images

`Dockerfile.dev` contains two named targets:

- `backend-dev`: Go 1.25 on Alpine, the pinned Go reloader, Git and PostgreSQL client tools;
- `frontend-dev`: Node 22 on Alpine plus nginx.

The images contain tools and dependency manifests only. Application source always comes from the bind mount. Dependency caches are populated on first start and reused across restarts.

## Reload modes

Both application containers read the same variable:

```env
DEV_HOT_RELOAD=true
```

The Compose default is `true`.

### Hot reload enabled

The backend entrypoint runs a pinned Go file watcher in polling mode. A `.go`, `go.mod`, `go.sum`, template, or embedded SDK change triggers a bounded rebuild and graceful replacement of the Go process. Temporary binaries are written to a named build volume, never the source tree.

The frontend entrypoint starts both Vite development servers with polling enabled:

- Console on an internal port;
- Notification Center on a second internal port.

nginx listens on the container's public port and routes `/console/*` and `/notification-center/*` to the matching Vite server, preserving HMR WebSocket upgrades. Every other path is proxied to `backend:8080`, including `/api`, `/config.js`, public pages and webhooks.

The browser always uses `http://localhost:8081`; direct backend access is not required. SMTP remains available at `127.0.0.1:1587` from the backend container.

### Hot reload disabled

The backend entrypoint runs `go run ./cmd/api` once. It does not watch source files. A source edit therefore becomes active only after the backend container restarts and recompiles.

The frontend entrypoint builds both React applications once, then nginx serves their generated output and proxies all non-frontend paths to the backend. It does not run Vite watchers. React edits become active only after restart and rebuild.

After disabling reload once with a container recreation:

```powershell
$env:DEV_HOT_RELOAD='false'
docker compose up -d --force-recreate backend frontend
```

subsequent source changes are applied with:

```powershell
docker compose restart backend frontend
```

Changing the switch itself requires `docker compose up -d --force-recreate`; `restart` does not reload Compose environment variables.

## Frontend gateway

The frontend container owns the only HTTP host mapping, `127.0.0.1:8081:8080`. nginx provides a stable gateway in both reload modes:

- `/console/` serves or proxies the Console SPA;
- `/notification-center/` serves or proxies the Notification Center SPA;
- HMR WebSocket upgrades are forwarded only in reload mode;
- all other paths proxy to `backend:8080`;
- `/` therefore retains the backend's existing redirects and public routing;
- forwarded host, scheme and client IP headers are preserved.

Both Vite configurations accept environment-driven host, port, base path, HTTPS and polling settings. Existing certificate-based standalone development remains available when those environment values are explicitly supplied, but Docker development uses HTTP inside the private Compose network.

## Entrypoints and lifecycle

Development entrypoints use strict shell error handling and validate `DEV_HOT_RELOAD` as `true` or `false`. Invalid values fail startup with an actionable message.

The frontend entrypoint owns both Vite children in reload mode. If either child exits unexpectedly, it terminates the sibling and exits so Compose can restart the container. SIGTERM and SIGINT are forwarded and all children are reaped. nginx remains the foreground readiness boundary.

Dependency installation uses a lockfile digest marker. `npm ci` or `go mod download` reruns only when the matching lockfile changes or its dependency volume is empty.

During a Go hot restart nginx may briefly return `502` for backend routes. It must not cache that response. Provider workers stop with the old process and resume from PostgreSQL/RabbitMQ durable state in the replacement process.

## Health and startup order

- Backend health checks `http://localhost:8080/healthz`.
- Frontend health checks `http://localhost:8080/console/` through nginx.
- Frontend waits for backend health before starting.
- Backend waits for database, RabbitMQ, Redis, ClickHouse and object storage initialization.
- Database migration continues to run once in the consolidated backend before its health check succeeds.

Named data volumes for PostgreSQL, RabbitMQ, Redis, ClickHouse and MinIO are unchanged. Switching between compact development and HA topology must not recreate or delete them.

## Files changed

- replace the default `compose.yaml` with the compact development topology;
- preserve the existing full topology as `compose.ha.yaml`;
- add `Dockerfile.dev`;
- add backend and frontend development entrypoints;
- add nginx development gateway configuration;
- add the Go watcher configuration;
- make both Vite configurations environment-driven and Docker-aware;
- update `.dockerignore`, README startup documentation and Compose tests.

## Verification

The implementation is complete only when all of these pass:

1. `docker compose config` and `docker compose -f compose.ha.yaml config` validate;
2. development entrypoint shell syntax checks pass;
3. a cold default start reaches eight healthy long-running services and database/code version 45;
4. a Go source change replaces the backend child process without rebuilding the image;
5. Console and Notification Center source changes are observed by their Vite watchers;
6. with `DEV_HOT_RELOAD=false`, source changes do not restart either runtime before `docker compose restart backend frontend`;
7. the same restart recompiles Go, rebuilds both React applications and serves the updated output;
8. `/`, `/console/`, `/notification-center/`, `/config.js`, `/api/*`, `/webhooks/*` and HMR WebSockets route to the correct process;
9. backend tests, affected frontend tests, lint, production builds and the production Dockerfile build continue to pass;
10. the HA Compose file still starts the independent runtime roles without source mounts.

## Non-goals

- Removing RabbitMQ, Redis, ClickHouse, MinIO or PgBouncer from the high-concurrency runtime;
- using bind-mounted source in production;
- changing delivery semantics, database schemas or public APIs;
- automatically switching a running container when `DEV_HOT_RELOAD` changes without recreation.
