# Compact Compose Development Hot Reload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the default Compose workflow run bind-mounted Go and React source in two application containers, with hot reload enabled by default and restart-only rebuilds when disabled.

**Architecture:** A consolidated `backend` runs `NOTIFUSE_ROLE=all`; a `frontend` container runs nginx plus the Console and Notification Center Vite processes. The current immutable seven-role topology is preserved in `compose.ha.yaml`, while named dependency/build volumes isolate Linux artifacts from the Windows checkout.

**Tech Stack:** Docker Compose v5, Docker multi-target builds, Go 1.25, Air, Node 22, Vite 7, nginx Alpine, PowerShell validation.

**Spec:** `docs/superpowers/specs/2026-08-29-compose-development-hot-reload-design.md`

## Global Constraints

- `DEV_HOT_RELOAD` accepts only `true` or `false` and defaults to `true`.
- Default development uses exactly two long-running application containers: `backend` and `frontend`.
- `backend` always runs `NOTIFUSE_ROLE=all` in the default topology.
- The browser entry point remains `http://localhost:8081`; SMTP remains `127.0.0.1:1587`.
- Source is bind-mounted at `/workspace`; Go caches, npm caches, `node_modules`, generated dist, and temporary binaries use named volumes.
- The production `Dockerfile` and HA runtime use no source bind mounts or development entrypoints.
- Existing PostgreSQL, PgBouncer, RabbitMQ, Redis, ClickHouse and MinIO data volumes retain their names.
- File watching must work on Windows Docker Desktop through polling.

---

### Task 1: Preserve and validate the HA topology

**Files:**
- Create: `compose.ha.yaml`
- Modify: `scripts/validate-realtime-compose.ps1`

**Interfaces:**
- Consumes: the current `compose.yaml` seven-role runtime contract.
- Produces: a standalone `compose.ha.yaml` and a validator that accepts a compose-file parameter.

- [ ] **Step 1: Change the validator contract first**

Add a parameter and derive expectations from `-Topology`:

```powershell
param(
    [ValidateSet('development', 'ha')]
    [string]$Topology = 'development'
)

$composeName = if ($Topology -eq 'ha') { 'compose.ha.yaml' } else { 'compose.yaml' }
$composeFile = Join-Path $repoRoot $composeName
```

For HA require the existing seven roles and assert every application service has no bind mount whose source is the repository root. For development require `backend` and `frontend`, assert `backend.environment.NOTIFUSE_ROLE -eq 'all'`, and assert both application services bind the repository to `/workspace`.

- [ ] **Step 2: Run the HA validation and observe the missing-file failure**

Run:

```powershell
./scripts/validate-realtime-compose.ps1 -Topology ha
```

Expected: failure because `compose.ha.yaml` does not exist.

- [ ] **Step 3: Create the standalone HA file**

Create `compose.ha.yaml` from the current `compose.yaml` without changing service definitions, image build, role environment, health checks, ports, volumes, init jobs, or named-volume identifiers.

- [ ] **Step 4: Validate the HA contract**

Run:

```powershell
docker compose -f compose.ha.yaml config --quiet
./scripts/validate-realtime-compose.ps1 -Topology ha
```

Expected: both commands succeed and print `Realtime Compose ha structure is valid.`

- [ ] **Step 5: Commit**

```powershell
git add compose.ha.yaml scripts/validate-realtime-compose.ps1
git commit -m "build: preserve high availability compose topology"
```

### Task 2: Add deterministic development runtimes

**Files:**
- Create: `Dockerfile.dev`
- Create: `.air.toml`
- Create: `deploy/dev/backend-entrypoint.sh`
- Create: `deploy/dev/frontend-entrypoint.sh`
- Create: `deploy/dev/nginx.conf`

**Interfaces:**
- Consumes: `/workspace`, `DEV_HOT_RELOAD`, `SERVER_PORT=8080`, named cache directories under `/cache`.
- Produces: Docker targets `backend-dev` and `frontend-dev`; both entrypoints expose a long-running process with signal-safe shutdown.

- [ ] **Step 1: Add entrypoint contract checks to the Compose validator**

Require development build targets and commands:

```powershell
if ($rendered.services.backend.build.target -ne 'backend-dev') { throw 'backend-dev target is required' }
if ($rendered.services.frontend.build.target -ne 'frontend-dev') { throw 'frontend-dev target is required' }
if ($rendered.services.backend.environment.DEV_HOT_RELOAD -ne 'true') { throw 'hot reload must default to true' }
```

- [ ] **Step 2: Create `Dockerfile.dev`**

Use this target boundary:

```dockerfile
FROM golang:1.25-alpine AS backend-dev
RUN apk add --no-cache git postgresql-client ca-certificates tzdata
RUN go install github.com/air-verse/air@v1.63.0
WORKDIR /workspace
COPY deploy/dev/backend-entrypoint.sh /usr/local/bin/notifuse-backend-dev
RUN chmod +x /usr/local/bin/notifuse-backend-dev
ENTRYPOINT ["/usr/local/bin/notifuse-backend-dev"]

FROM node:22-alpine AS frontend-dev
RUN apk add --no-cache nginx
WORKDIR /workspace
COPY deploy/dev/frontend-entrypoint.sh /usr/local/bin/notifuse-frontend-dev
COPY deploy/dev/nginx.conf /etc/nginx/http.d/default.conf
RUN chmod +x /usr/local/bin/notifuse-frontend-dev
ENTRYPOINT ["/usr/local/bin/notifuse-frontend-dev"]
```

If the pinned Air version is unavailable, select the newest version verified by `go install` and record the exact pin in the Dockerfile; never use `@latest`.

- [ ] **Step 3: Implement the backend entrypoint**

The script must validate the switch, refresh modules when `go.sum` changes, and `exec` exactly one foreground runtime:

```sh
#!/bin/sh
set -eu
case "${DEV_HOT_RELOAD:-true}" in true|false) ;; *) echo "DEV_HOT_RELOAD must be true or false" >&2; exit 64;; esac
export GOCACHE="${GOCACHE:-/cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-/cache/go-mod}"
mkdir -p "$GOCACHE" "$GOMODCACHE" /cache/notifuse-bin
digest="$(sha256sum go.sum | awk '{print $1}')"
marker=/cache/go-mod/.notifuse-go-sum
if [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$digest" ]; then
  go mod download
  printf '%s' "$digest" > "$marker"
fi
if [ "$DEV_HOT_RELOAD" = true ]; then exec air -c .air.toml; fi
exec go run ./cmd/api
```

- [ ] **Step 4: Configure polling Air**

`.air.toml` builds `/cache/notifuse-bin/server`, watches Go, HTML, template and embedded SDK inputs, excludes `.git`, all `node_modules`, all `dist`, data and cache directories, uses polling, and sends interrupt before a bounded kill timeout.

- [ ] **Step 5: Implement the frontend entrypoint**

Implement reusable lockfile installation and child cleanup:

```sh
install_app() {
  app="$1"; marker="/workspace/$app/node_modules/.notifuse-lock"
  digest="$(sha256sum "/workspace/$app/package-lock.json" | awk '{print $1}')"
  if [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$digest" ]; then
    (cd "/workspace/$app" && npm ci --cache "/cache/npm")
    printf '%s' "$digest" > "$marker"
  fi
}
```

Validate the switch, install both apps, and then:

- hot mode: start both `npm run dev` processes on fixed internal ports, start nginx, trap signals, and exit if either Vite child dies;
- restart-only mode: run both production builds, then `exec nginx -g 'daemon off;'`.

- [ ] **Step 6: Add nginx routing**

Configure nginx port 8080 with these exact ownership rules:

```nginx
location /console/ { proxy_pass http://127.0.0.1:5173; }
location /notification-center/ { proxy_pass http://127.0.0.1:5174; }
location / { proxy_pass http://backend:8080; }
```

Hot-mode proxy locations must forward `Upgrade` and `Connection` headers. Restart-only mode selects static `try_files` locations through an environment-generated include while keeping the catch-all backend proxy.

- [ ] **Step 7: Verify shell and image syntax**

Run:

```powershell
docker run --rm -v "${PWD}:/workspace" alpine:3.19 sh -n /workspace/deploy/dev/backend-entrypoint.sh
docker run --rm -v "${PWD}:/workspace" alpine:3.19 sh -n /workspace/deploy/dev/frontend-entrypoint.sh
docker build --target backend-dev -f Dockerfile.dev -t notifuse-backend-dev:test .
docker build --target frontend-dev -f Dockerfile.dev -t notifuse-frontend-dev:test .
```

Expected: all commands exit 0.

- [ ] **Step 8: Commit**

```powershell
git add Dockerfile.dev .air.toml deploy/dev
git commit -m "build: add source-mounted development runtimes"
```

### Task 3: Make both Vite applications gateway-aware

**Files:**
- Modify: `console/vite.config.ts`
- Modify: `notification_center/vite.config.ts`
- Test: `console` and `notification_center` production builds.

**Interfaces:**
- Consumes: `VITE_DEV_HOST`, `VITE_DEV_PORT`, `VITE_DEV_BASE`, `VITE_USE_POLLING`, `VITE_DEV_HTTPS`.
- Produces: Console at `/console/` on 5173 and Notification Center at `/notification-center/` on 5174, both with HMR through nginx.

- [ ] **Step 1: Replace hard-coded Console development settings**

Load environment through Vite's `loadEnv`. Defaults must be:

```typescript
const host = env.VITE_DEV_HOST || '0.0.0.0'
const port = Number(env.VITE_DEV_PORT || 5173)
const usePolling = env.VITE_USE_POLLING !== 'false'
const useHTTPS = env.VITE_DEV_HTTPS === 'true'
```

Read certificate files only when `useHTTPS` is true. Set `server.watch.usePolling`, `server.hmr.clientPort`, and the existing `/console/` base without changing tests or production output.

- [ ] **Step 2: Configure Notification Center base and polling**

Use the same environment contract with defaults `0.0.0.0`, `5174`, `/notification-center/`, and polling enabled. Set the Vite `base` so generated asset paths and HMR requests remain under `/notification-center/`.

- [ ] **Step 3: Verify both applications**

Run:

```powershell
Set-Location console
npm test -- --run
npm run build
Set-Location ../notification_center
npm run test:run
npm run build
Set-Location ..
```

Expected: test suites and both production builds pass. Existing unrelated lint warnings may be reported separately but modified files must have zero lint errors.

- [ ] **Step 4: Commit**

```powershell
git add console/vite.config.ts notification_center/vite.config.ts
git commit -m "build: make vite development container aware"
```

### Task 4: Replace the default Compose file with compact development

**Files:**
- Modify: `compose.yaml`
- Modify: `.dockerignore`
- Modify: `scripts/validate-realtime-compose.ps1`

**Interfaces:**
- Consumes: `backend-dev`, `frontend-dev`, source root, existing infrastructure and named data volumes.
- Produces: `docker compose up -d` with `backend`, `frontend` and six infrastructure services.

- [ ] **Step 1: Run development validation before replacement**

Run:

```powershell
./scripts/validate-realtime-compose.ps1 -Topology development
```

Expected: failure because the current default file contains the split roles rather than `backend` and `frontend`.

- [ ] **Step 2: Define the backend service**

The rendered Compose contract must include:

```yaml
backend:
  build:
    context: .
    dockerfile: Dockerfile.dev
    target: backend-dev
  environment:
    NOTIFUSE_ROLE: all
    DEV_HOT_RELOAD: ${DEV_HOT_RELOAD:-true}
  volumes:
    - .:/workspace
    - go-build-cache:/cache/go-build
    - go-mod-cache:/cache/go-mod
    - go-runtime-cache:/cache/notifuse-bin
  ports:
    - '127.0.0.1:${SMTP_HOST_PORT:-1587}:587'
```

Reuse all current realtime/database/storage environment values and dependency health gates. Do not publish backend HTTP directly.

- [ ] **Step 3: Define the frontend service**

The rendered contract must include:

```yaml
frontend:
  build:
    context: .
    dockerfile: Dockerfile.dev
    target: frontend-dev
  environment:
    DEV_HOT_RELOAD: ${DEV_HOT_RELOAD:-true}
    VITE_USE_POLLING: 'true'
  volumes:
    - .:/workspace
    - console-node-modules:/workspace/console/node_modules
    - console-dist:/workspace/console/dist
    - notification-center-node-modules:/workspace/notification_center/node_modules
    - notification-center-dist:/workspace/notification_center/dist
    - npm-cache:/cache/npm
  ports:
    - '127.0.0.1:${API_HOST_PORT:-8081}:8080'
  depends_on:
    backend:
      condition: service_healthy
```

- [ ] **Step 4: Preserve infrastructure and data names**

Carry PostgreSQL, PgBouncer, RabbitMQ/init, Redis, ClickHouse, MinIO/init, network and the five existing data volumes into the compact file unchanged. Add only the seven development cache/output volumes.

- [ ] **Step 5: Validate both topologies**

Run:

```powershell
docker compose config --quiet
docker compose -f compose.ha.yaml config --quiet
./scripts/validate-realtime-compose.ps1 -Topology development
./scripts/validate-realtime-compose.ps1 -Topology ha
```

Expected: all commands exit 0; development output reports two application services and HA output reports seven.

- [ ] **Step 6: Commit**

```powershell
git add compose.yaml .dockerignore scripts/validate-realtime-compose.ps1
git commit -m "build: make compact source compose the default"
```

### Task 5: Runtime proof and user documentation

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: both Compose topologies and `DEV_HOT_RELOAD` modes.
- Produces: verified commands and operator-facing documentation.

- [ ] **Step 1: Start the default hot topology**

Run with the configured proxy:

```powershell
$env:HTTP_PROXY='http://127.0.0.1:7897'
$env:HTTPS_PROXY='http://127.0.0.1:7897'
Remove-Item Env:DEV_HOT_RELOAD -ErrorAction SilentlyContinue
docker compose down
docker compose up -d --build
docker compose ps
```

Expected: `backend`, `frontend`, PostgreSQL, PgBouncer, RabbitMQ, Redis, ClickHouse and MinIO are running; backend and frontend are healthy; setup jobs exited 0.

- [ ] **Step 2: Verify routing and version**

Run:

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8081/console/
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8081/notification-center/
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8081/config.js
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8081/healthz
docker compose logs backend --tail 100
```

Expected: all requests return 2xx except an intentional redirect followed automatically; logs show `NOTIFUSE_ROLE=all`, code/database version 45 and all realtime workers started.

- [ ] **Step 3: Prove hot reload**

Record the Go child PID from backend logs/processes, touch a watched Go file without changing content, and confirm the child PID changes while the container ID stays constant. Touch one source file in each React application and confirm each Vite log reports an HMR update while the frontend container ID stays constant.

- [ ] **Step 4: Prove restart-only mode**

Run:

```powershell
$env:DEV_HOT_RELOAD='false'
docker compose up -d --force-recreate backend frontend
```

Touch the same files and confirm child PIDs/build timestamps do not change for at least two polling intervals. Then run `docker compose restart backend frontend` and confirm Go recompiles, both React builds rerun, and all four HTTP probes pass.

- [ ] **Step 5: Run regression verification**

Run:

```powershell
go test ./... -run '^$'
go vet ./internal/app ./internal/http ./internal/service
Set-Location console; npm run build; Set-Location ..
Set-Location notification_center; npm run build; Set-Location ..
docker build -f Dockerfile -t notifuse-production:test .
git diff --check
```

Expected: every command exits 0. Report any unrelated pre-existing test or lint failure explicitly rather than changing unrelated code.

- [ ] **Step 6: Document commands and boundaries**

README must show default hot start, disabling the switch, restart-only workflow, HA start, fixed URLs and the fact that source mounts are development-only. CHANGELOG must record the compact topology and two reload modes.

- [ ] **Step 7: Commit**

```powershell
git add README.md CHANGELOG.md
git commit -m "docs: document compact compose development"
```

- [ ] **Step 8: Final repository and runtime check**

Run `git status --short`, `git log -6 --oneline`, `docker compose ps`, and the four HTTP probes. Expected: clean worktree, all expected services healthy, and current source served from the default development topology.
