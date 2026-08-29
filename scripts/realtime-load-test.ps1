param(
    [Parameter(Mandatory = $true)] [string] $ApiEndpoint,
    [Parameter(Mandatory = $true)] [string] $WorkspaceID,
    [Parameter(Mandatory = $true)] [string] $Token,
    [int] $Requests = 100,
    [int] $BatchSize = 50,
    [int] $Concurrency = 16,
    [int] $MaxP95Milliseconds = 200
)

$ErrorActionPreference = 'Stop'

if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw 'This load test requires PowerShell 7 or newer for bounded parallel execution.'
}
if ($Requests -le 0 -or $BatchSize -le 0 -or $BatchSize -gt 500 -or $Concurrency -le 0) {
    throw 'Requests and Concurrency must be positive; BatchSize must be between 1 and 500.'
}

$url = $ApiEndpoint.TrimEnd('/') + '/api/ingest.batch'
$runID = [guid]::NewGuid().ToString('N')
$results = 0..($Requests - 1) | ForEach-Object -Parallel {
    $requestIndex = $_
    $events = for ($itemIndex = 0; $itemIndex -lt $using:BatchSize; $itemIndex++) {
        $externalID = "$($using:runID)-$requestIndex-$itemIndex"
        @{
            id = $externalID
            email = "load-$($itemIndex % 1000)@example.com"
            event_name = 'load.event'
            external_id = $externalID
            occurred_at = [DateTime]::UtcNow.ToString('o')
            properties = @{
                request = $requestIndex
                item = $itemIndex
            }
        }
    }
    $body = @{
        workspace_id = $using:WorkspaceID
        events = $events
    } | ConvertTo-Json -Depth 8 -Compress
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $response = Invoke-WebRequest -Method Post -Uri $using:url -Headers @{
            Authorization = "Bearer $($using:Token)"
            'Content-Type' = 'application/json'
        } -Body $body -SkipHttpErrorCheck
        $timer.Stop()
        [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Milliseconds = $timer.Elapsed.TotalMilliseconds
            Error = if ([int]$response.StatusCode -eq 200) { $null } else { $response.Content }
        }
    }
    catch {
        $timer.Stop()
        [pscustomobject]@{
            StatusCode = 0
            Milliseconds = $timer.Elapsed.TotalMilliseconds
            Error = $_.Exception.Message
        }
    }
} -ThrottleLimit $Concurrency

$ordered = @($results.Milliseconds | Sort-Object)
$p95Index = [Math]::Max(0, [Math]::Ceiling($ordered.Count * 0.95) - 1)
$p95 = [Math]::Round($ordered[$p95Index], 2)
$failed = @($results | Where-Object { $_.StatusCode -ne 200 })
$totalRecords = $Requests * $BatchSize

Write-Output "Requests=$Requests Records=$totalRecords Concurrency=$Concurrency P95Ms=$p95 Failed=$($failed.Count)"
if ($failed.Count -gt 0) {
    $failed | Select-Object -First 5 | Format-Table -AutoSize
    throw "$($failed.Count) ingest requests failed."
}
if ($p95 -gt $MaxP95Milliseconds) {
    throw "Ingest p95 ${p95}ms exceeds target ${MaxP95Milliseconds}ms."
}
