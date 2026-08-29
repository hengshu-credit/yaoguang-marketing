param(
    [ValidateSet('development', 'ha')]
    [string]$Topology = 'development'
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$composeName = if ($Topology -eq 'ha') { 'compose.ha.yaml' } else { 'compose.yaml' }
$composeFile = Join-Path $repoRoot $composeName
$dockerIgnoreFile = Join-Path $repoRoot '.dockerignore'

$dockerIgnore = Get-Content -LiteralPath $dockerIgnoreFile -Raw
foreach ($pattern in @('**/node_modules', '**/dist', '.git')) {
    if ($dockerIgnore -notmatch [regex]::Escape($pattern)) {
        throw ".dockerignore must exclude $pattern"
    }
}

$renderedJSON = & docker compose --project-directory $repoRoot -f $composeFile config --format json
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config failed with exit code $LASTEXITCODE"
}

$rendered = $renderedJSON | ConvertFrom-Json
$infrastructureServices = @(
    'postgres',
    'pgbouncer',
    'rabbitmq',
    'rabbitmq-init',
    'redis',
    'clickhouse',
    'minio',
    'minio-init'
)
$applicationServices = if ($Topology -eq 'ha') {
    @('api', 'outbox-relay', 'rule-worker', 'journey-worker', 'delivery-worker', 'analytics-worker', 'scheduler')
} else {
    @('backend', 'frontend')
}
$required = @($infrastructureServices) + @($applicationServices)

$serviceNames = @($rendered.services.PSObject.Properties.Name)
foreach ($name in $required) {
    if ($name -notin $serviceNames) {
        throw "missing service: $name"
    }
}

if ($Topology -eq 'ha') {
    if ($rendered.services.api.environment.NOTIFUSE_ROLE -ne 'api') {
        throw 'api role mismatch'
    }
    foreach ($name in $applicationServices) {
        $workspaceMount = @($rendered.services.$name.volumes | Where-Object { $_.type -eq 'bind' -and $_.target -eq '/workspace' })
        if ($workspaceMount.Count -gt 0) {
            throw "HA service must not mount source: $name"
        }
    }
} else {
    if ($rendered.services.backend.environment.NOTIFUSE_ROLE -ne 'all') {
        throw 'development backend must run the all role'
    }
    if ($rendered.services.backend.build.target -ne 'backend-dev') {
        throw 'backend-dev target is required'
    }
    if ($rendered.services.frontend.build.target -ne 'frontend-dev') {
        throw 'frontend-dev target is required'
    }
    if ($rendered.services.backend.environment.DEV_HOT_RELOAD -ne 'true' -or
        $rendered.services.frontend.environment.DEV_HOT_RELOAD -ne 'true') {
        throw 'hot reload must default to true'
    }
    foreach ($name in $applicationServices) {
        $workspaceMount = @($rendered.services.$name.volumes | Where-Object { $_.type -eq 'bind' -and $_.target -eq '/workspace' })
        if ($workspaceMount.Count -ne 1) {
            throw "development service must mount source once at /workspace: $name"
        }
    }
}

if ($rendered.services.pgbouncer.environment.POOL_MODE -ne 'transaction') {
    throw 'PgBouncer must use transaction pooling'
}

if ($rendered.services.pgbouncer.environment.DB_NAME -ne '*') {
    throw 'PgBouncer must route dynamic workspace databases with a wildcard'
}

if ($rendered.services.pgbouncer.environment.AUTH_TYPE -ne 'scram-sha-256') {
    throw 'PgBouncer and PostgreSQL must use SCRAM authentication'
}

if ([int]$rendered.services.pgbouncer.environment.MAX_PREPARED_STATEMENTS -lt 1) {
    throw 'PgBouncer transaction pooling must track protocol-level prepared statements'
}

if ($Topology -eq 'ha') {
    foreach ($name in @('outbox-relay', 'rule-worker', 'journey-worker', 'delivery-worker', 'analytics-worker', 'scheduler')) {
        if ($rendered.services.$name.depends_on.'rabbitmq-init'.condition -ne 'service_completed_successfully') {
            throw "$name must wait for RabbitMQ definitions to be imported"
        }
        if ($rendered.services.$name.depends_on.api.condition -ne 'service_healthy') {
            throw "$name must wait for the API migration gate"
        }
    }
} else {
    if ($rendered.services.backend.depends_on.'rabbitmq-init'.condition -ne 'service_completed_successfully') {
        throw 'development backend must wait for RabbitMQ definitions'
    }
    if ($rendered.services.backend.depends_on.'minio-init'.condition -ne 'service_completed_successfully') {
        throw 'development backend must wait for MinIO initialization'
    }
    if ($rendered.services.frontend.depends_on.backend.condition -ne 'service_healthy') {
        throw 'development frontend must wait for backend health'
    }
}

if ($rendered.services.'minio-init'.entrypoint[0] -ne '/bin/sh') {
    throw 'MinIO initialization must override the mc image entrypoint with /bin/sh'
}

$definitionsPath = Join-Path $repoRoot 'deploy/rabbitmq/definitions.json'
$definitions = Get-Content -LiteralPath $definitionsPath -Raw | ConvertFrom-Json
$queueNames = @($definitions.queues.name)
foreach ($name in @(
    'notifuse.rule',
    'notifuse.journey',
    'notifuse.delivery',
    'notifuse.analytics',
    'notifuse.rule.dead',
    'notifuse.journey.dead',
    'notifuse.delivery.dead',
    'notifuse.analytics.dead'
)) {
    if ($name -notin $queueNames) {
        throw "missing durable queue definition: $name"
    }
}
foreach ($queue in $definitions.queues) {
    if ($queue.arguments.'x-queue-type' -ne 'quorum') {
        throw "queue must be quorum: $($queue.name)"
    }
}
$retryQueues = @($definitions.queues | Where-Object { $_.arguments.'x-message-ttl' -gt 0 })
if ($retryQueues.Count -ne 16) {
    throw "four workers must each define four TTL retry queues; found $($retryQueues.Count)"
}

$publishedServices = @(
    $(if ($Topology -eq 'ha') { $rendered.services.api } else { $rendered.services.frontend }),
    $rendered.services.postgres,
    $rendered.services.pgbouncer,
    $rendered.services.rabbitmq,
    $rendered.services.redis,
    $rendered.services.clickhouse,
    $rendered.services.minio
)
foreach ($service in $publishedServices) {
    foreach ($binding in @($service.ports)) {
        if ($binding.host_ip -and $binding.host_ip -notin @('127.0.0.1', '::1')) {
            throw "published port must bind to localhost: $($binding.host_ip):$($binding.published)"
        }
    }
}

if ($rendered.services.redis.ports[0].published -ne 16380) {
    throw 'Redis host port must default to the conflict-resistant development port 16380'
}

Write-Output "Realtime Compose $Topology structure is valid."
