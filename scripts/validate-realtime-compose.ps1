$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $repoRoot 'compose.yaml'

$renderedJSON = & docker compose --project-directory $repoRoot -f $composeFile config --format json
if ($LASTEXITCODE -ne 0) {
    throw "docker compose config failed with exit code $LASTEXITCODE"
}

$rendered = $renderedJSON | ConvertFrom-Json
$required = @(
    'postgres',
    'pgbouncer',
    'rabbitmq',
    'redis',
    'clickhouse',
    'minio',
    'api',
    'outbox-relay',
    'rule-worker',
    'journey-worker',
    'delivery-worker',
    'analytics-worker',
    'scheduler'
)

$serviceNames = @($rendered.services.PSObject.Properties.Name)
foreach ($name in $required) {
    if ($name -notin $serviceNames) {
        throw "missing service: $name"
    }
}

if ($rendered.services.api.environment.NOTIFUSE_ROLE -ne 'api') {
    throw 'api role mismatch'
}

if ($rendered.services.pgbouncer.environment.PGBOUNCER_POOL_MODE -ne 'transaction') {
    throw 'PgBouncer must use transaction pooling'
}

if ($rendered.services.pgbouncer.environment.PGBOUNCER_DATABASE -ne '*') {
    throw 'PgBouncer must route dynamic workspace databases with a wildcard'
}

$definitionsPath = Join-Path $repoRoot 'deploy/rabbitmq/definitions.json'
$definitions = Get-Content -LiteralPath $definitionsPath -Raw | ConvertFrom-Json
$queueNames = @($definitions.queues.name)
foreach ($name in @('notifuse.rule', 'notifuse.journey', 'notifuse.delivery', 'notifuse.analytics', 'notifuse.dead-letter')) {
    if ($name -notin $queueNames) {
        throw "missing durable queue definition: $name"
    }
}
foreach ($queue in $definitions.queues) {
    if ($queue.arguments.'x-queue-type' -ne 'quorum') {
        throw "queue must be quorum: $($queue.name)"
    }
}
if (-not ($definitions.queues | Where-Object { $_.arguments.'x-message-ttl' -gt 0 })) {
    throw 'at least one TTL retry queue is required'
}

$managedServices = @(
    $rendered.services.rabbitmq,
    $rendered.services.clickhouse,
    $rendered.services.minio
)
foreach ($service in $managedServices) {
    foreach ($binding in @($service.ports)) {
        if ($binding.host_ip -and $binding.host_ip -notin @('127.0.0.1', '::1')) {
            throw "management port must bind to localhost: $($binding.host_ip):$($binding.published)"
        }
    }
}

Write-Output 'Realtime Compose structure is valid.'
