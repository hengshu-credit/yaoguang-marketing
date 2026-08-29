$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$requiredFiles = @(
    'Dockerfile.dev',
    '.air.toml',
    'deploy/dev/backend-entrypoint.sh',
    'deploy/dev/frontend-entrypoint.sh',
    'deploy/dev/nginx.hot.conf',
    'deploy/dev/nginx.static.conf'
)

foreach ($relativePath in $requiredFiles) {
    $path = Join-Path $repoRoot $relativePath
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "missing development runtime file: $relativePath"
    }
}

function Assert-Contains {
    param([string]$RelativePath, [string]$Pattern, [string]$Message)
    $content = Get-Content -LiteralPath (Join-Path $repoRoot $RelativePath) -Raw
    if ($content -notmatch $Pattern) {
        throw $Message
    }
}

Assert-Contains 'Dockerfile.dev' 'AS backend-dev' 'Dockerfile.dev must define backend-dev'
Assert-Contains 'Dockerfile.dev' 'AS frontend-dev' 'Dockerfile.dev must define frontend-dev'
Assert-Contains 'Dockerfile.dev' 'ARG FRONTEND_BASE_IMAGE=node:22-alpine' 'frontend base image must remain overridable'
Assert-Contains 'Dockerfile.dev' 'air@v[0-9]+\.[0-9]+\.[0-9]+' 'Air must use a pinned version'
Assert-Contains '.air.toml' '(?m)^\s*poll\s*=\s*true\s*$' 'Air polling must be enabled'
Assert-Contains 'deploy/dev/backend-entrypoint.sh' 'DEV_HOT_RELOAD must be true or false' 'backend switch validation is required'
Assert-Contains 'deploy/dev/backend-entrypoint.sh' 'exec air -c \.air\.toml' 'backend hot mode must run Air'
Assert-Contains 'deploy/dev/backend-entrypoint.sh' 'exec go run ./cmd/api' 'backend restart-only mode must compile once'
Assert-Contains 'deploy/dev/frontend-entrypoint.sh' 'npm run dev' 'frontend hot mode must run Vite'
Assert-Contains 'deploy/dev/frontend-entrypoint.sh' 'npm run build' 'frontend restart-only mode must build once'
Assert-Contains 'deploy/dev/nginx.hot.conf' 'location /console/' 'hot gateway must route Console'
Assert-Contains 'deploy/dev/nginx.hot.conf' 'location /notification-center/' 'hot gateway must route Notification Center'
Assert-Contains 'deploy/dev/nginx.static.conf' 'alias /workspace/console/dist/' 'static gateway must serve Console output'
Assert-Contains 'deploy/dev/nginx.static.conf' 'alias /workspace/notification_center/dist/' 'static gateway must serve Notification Center output'
Assert-Contains 'console/vite.config.ts' 'loadEnv' 'Console Vite config must load environment settings'
Assert-Contains 'console/vite.config.ts' 'VITE_USE_POLLING' 'Console Vite config must expose polling control'
Assert-Contains 'console/vite.config.ts' 'VITE_DEV_HTTPS' 'Console Vite HTTPS must be optional'
Assert-Contains 'notification_center/vite.config.ts' 'base:\s*[''"]?/notification-center/' 'Notification Center must use its gateway base path'
Assert-Contains 'notification_center/vite.config.ts' 'VITE_USE_POLLING' 'Notification Center Vite config must expose polling control'

foreach ($script in @('backend-entrypoint.sh', 'frontend-entrypoint.sh')) {
    & docker run --rm -v "${repoRoot}:/workspace" alpine:3.19 sh -n "/workspace/deploy/dev/$script"
    if ($LASTEXITCODE -ne 0) {
        throw "$script failed sh syntax validation"
    }
}

Write-Output 'Development runtime files are valid.'
