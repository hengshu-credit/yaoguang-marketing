$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $repoRoot 'compose.yaml'
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
$required = @(
    'postgres',
    'pgbouncer',
    'rabbitmq',
    'rabbitmq-init',
    'redis',
    'clickhouse',
    'minio',
    'minio-init',
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

foreach ($name in @('outbox-relay', 'rule-worker', 'journey-worker', 'delivery-worker', 'analytics-worker', 'scheduler')) {
    if ($rendered.services.$name.depends_on.'rabbitmq-init'.condition -ne 'service_completed_successfully') {
        throw "$name must wait for RabbitMQ definitions to be imported"
    }
    if ($rendered.services.$name.depends_on.api.condition -ne 'service_healthy') {
        throw "$name must wait for the API migration gate"
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
    $rendered.services.api,
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

Write-Output 'Realtime Compose structure is valid.'
