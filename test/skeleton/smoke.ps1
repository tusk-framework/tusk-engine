param(
    [string] $FrameworkPath,
    [string] $RoadRunnerPath,
    [switch] $PrerequisitesOnly
)

$ErrorActionPreference = 'Stop'
$engineRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
if (-not $FrameworkPath) {
    $FrameworkPath = Join-Path $engineRoot '../../../tusk-framework/.worktrees/tusk-bootstrap'
}

$missing = @()
foreach ($name in @('php', 'composer', 'go')) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        $missing += $name
    }
}

if (-not $IsWindows) {
    foreach ($name in @('pgrep', 'readlink')) {
        if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
            $missing += $name
        }
    }
    if (-not (Test-Path -LiteralPath '/bin/kill' -PathType Leaf) -or -not (Test-Path -LiteralPath '/proc' -PathType Container)) {
        $missing += 'Linux /bin/kill and /proc process inspection'
    }
}

if ($RoadRunnerPath) {
    if (-not (Test-Path -LiteralPath $RoadRunnerPath -PathType Leaf)) {
        $missing += "RoadRunner ($RoadRunnerPath)"
    }
} elseif (-not (Get-Command rr -ErrorAction SilentlyContinue)) {
    $missing += 'RoadRunner (rr)'
}

if (-not $FrameworkPath -or -not (Test-Path -LiteralPath (Join-Path $FrameworkPath 'tusk-cli/src/Generator/ProjectGenerator.php') -PathType Leaf)) {
    $missing += 'checked-out Framework path (-FrameworkPath)'
} elseif (-not (Test-Path -LiteralPath (Join-Path $FrameworkPath 'tusk-cli/stubs/bootstrap-app.stub') -PathType Leaf)) {
    throw "Framework checkout at '$FrameworkPath' lacks the required modern generator (tusk-cli/stubs/bootstrap-app.stub); use the coordinated modern Framework ref"
} elseif (-not (Test-Path -LiteralPath (Join-Path $FrameworkPath 'vendor/autoload.php') -PathType Leaf)) {
    $missing += 'Framework Composer dependencies (vendor/autoload.php)'
}

if ($missing.Count) {
    Write-Output "SKIP skeleton smoke: missing prerequisites: $($missing -join ', ')"
    exit 0
}
if ($PrerequisitesOnly) {
    Write-Output 'PASS skeleton smoke prerequisites: PHP, Composer, Go, RoadRunner, and Framework checkout'
    exit 0
}

if ($IsWindows) {
    Write-Output 'SKIP skeleton smoke: graceful Unix SIGTERM and child-process inspection are unavailable on Windows; run with PowerShell on Linux.'
    exit 0
}

function Get-DescendantIds {
    param([int] $ParentId)
    foreach ($child in @(& pgrep -P $ParentId 2>$null)) {
        if ($child -match '^\d+$') {
            [int] $child
            Get-DescendantIds -ParentId ([int] $child)
        }
    }
}

function Wait-ProcessTreeExit {
    param([int[]] $ProcessIds, [int] $Seconds = 10)
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        $active = @($ProcessIds | Where-Object {
            $stat = "/proc/$_/stat"
            if (-not (Test-Path -LiteralPath $stat)) { return $false }
            try {
                $state = Get-Content -LiteralPath $stat -Raw -ErrorAction Stop
                return $state -match '^\d+ \(.+\) ([^Z ]) '
            } catch { return $false }
        })
        if (-not $active.Count) { return $true }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    return $false
}

function Get-RoadRunnerProcessIds {
    param([int] $EngineId, [string] $ProjectRoot, [int[]] $KnownProcessIds = @())
    $engineDescendantIds = @()
    if ($EngineId -and (Test-Path -LiteralPath "/proc/$EngineId")) {
        $engineDescendantIds = @(Get-DescendantIds -ParentId $EngineId)
    }
    $scopedIds = @($KnownProcessIds) + @($engineDescendantIds)
    $candidateIds = @($scopedIds) + @(Get-ChildItem -LiteralPath '/proc' -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^\d+$' } |
        ForEach-Object { [int] $_.Name })
    foreach ($processId in @($candidateIds | Sort-Object -Unique)) {
        try {
            $binary = (& readlink -f "/proc/$processId/exe" 2>$null).Trim()
            if ([System.IO.Path]::GetFileName($binary) -ne 'rr') { continue }
            $cwd = (& readlink -f "/proc/$processId/cwd" 2>$null).Trim()
            if ($scopedIds -contains $processId -or $cwd -eq $ProjectRoot) { [int] $processId }
        } catch { }
    }
}

function Wait-RoadRunnerProcess {
    param(
        [int] $EngineId,
        [string] $ProjectRoot,
        [int] $Seconds = 10,
        [System.Diagnostics.Process] $EngineProcess,
        [string[]] $Diagnostics = @()
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        $matches = @(Get-RoadRunnerProcessIds -EngineId $EngineId -ProjectRoot $ProjectRoot)
        if ($matches.Count -eq 1) { return [int] $matches[0] }
        if ($matches.Count -gt 1) { throw "expected one RoadRunner process in $ProjectRoot, found $($matches.Count)" }
        if ($EngineProcess -and $EngineProcess.HasExited) {
            $details = @(
                foreach ($path in $Diagnostics) {
                    if (Test-Path -LiteralPath $path) {
                        "$path`n$(Get-Content -Raw -LiteralPath $path)"
                    }
                }
            ) -join "`n"
            throw "Tusk Engine exited $($EngineProcess.ExitCode) before RoadRunner was independently identifiable in $ProjectRoot$(if ($details) { ":`n$details" })"
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "RoadRunner process was not independently identifiable in $ProjectRoot after $Seconds seconds"
}

function Stop-And-ReapRoadRunner {
    param([int[]] $KnownProcessIds, [int] $EngineId, [string] $ProjectRoot)
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    do {
        $processIds = @(Get-RoadRunnerProcessIds -EngineId $EngineId -ProjectRoot $ProjectRoot -KnownProcessIds $KnownProcessIds)
        if (-not $processIds.Count) { return }
        $processTreeIds = @($processIds) + @($processIds | ForEach-Object { Get-DescendantIds -ParentId $_ })
        $processTreeIds = @($processTreeIds | Sort-Object -Unique)
        foreach ($processId in $processIds) { & /bin/kill -TERM $processId 2>$null }
        if (-not (Wait-ProcessTreeExit -ProcessIds $processTreeIds -Seconds 2)) {
            foreach ($processId in $processTreeIds) { & /bin/kill -KILL $processId 2>$null }
            [void] (Wait-ProcessTreeExit -ProcessIds $processTreeIds -Seconds 2)
        }
        if (-not (Wait-ProcessTreeExit -ProcessIds $processTreeIds -Seconds 2)) { $script:cleanupSafe = $false }
        $remaining = @(Get-RoadRunnerProcessIds -EngineId $EngineId -ProjectRoot $ProjectRoot -KnownProcessIds $KnownProcessIds)
        if (-not $remaining.Count) { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    $script:cleanupSafe = $false
}

function Invoke-Bounded {
    param([string] $File, [string[]] $Arguments, [string] $Directory, [int] $Seconds, [string] $Log, [hashtable] $Environment = @{})

    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $File
    $start.WorkingDirectory = $Directory
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.Environment['TMPDIR'] = $scratch
    $start.Environment['GOTMPDIR'] = $scratch
    $start.Environment['GOCACHE'] = Join-Path $scratch 'gocache'
    $start.Environment['GOMODCACHE'] = Join-Path $scratch 'gomodcache'
    $start.Environment['GOPATH'] = Join-Path $scratch 'gopath'
    $start.Environment['GOTOOLCHAIN'] = 'local'
    $start.Environment['GOTELEMETRY'] = 'off'
    $start.Environment['GOENV'] = 'off'
    $start.Environment['COMPOSER_HOME'] = Join-Path $scratch 'composer-home'
    $start.Environment['COMPOSER_CACHE_DIR'] = Join-Path $scratch 'composer-cache'
    foreach ($argument in $Arguments) { [void] $start.ArgumentList.Add($argument) }
    foreach ($key in $Environment.Keys) { $start.Environment[$key] = [string] $Environment[$key] }
    $process = [System.Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit($Seconds * 1000)) {
            $descendants = @(Get-DescendantIds -ParentId $process.Id)
            try { $process.Kill($true) } catch {
                $script:cleanupSafe = $false
                throw
            }
            if (-not $process.WaitForExit(10000) -or -not (Wait-ProcessTreeExit -ProcessIds $descendants)) {
                $script:cleanupSafe = $false
                throw "$File timed out and its process tree did not exit after Kill"
            }
            if (-not [System.Threading.Tasks.Task]::WaitAll(@($stdout, $stderr), 10000)) {
                $script:cleanupSafe = $false
                throw "$File timed out and its process-tree output did not close after Kill"
            }
            throw "$File timed out after $Seconds seconds"
        }
        if (-not [System.Threading.Tasks.Task]::WaitAll(@($stdout, $stderr), 10000)) {
            $script:cleanupSafe = $false
            throw "$File exited but its process-tree output did not close"
        }
        $output = $stdout.GetAwaiter().GetResult() + $stderr.GetAwaiter().GetResult()
        [System.IO.File]::WriteAllText($Log, $output)
        if ($process.ExitCode -ne 0) { throw "$File exited $($process.ExitCode): $output" }
        return $output
    } finally {
        if ($script:cleanupSafe) { $process.Dispose() }
    }
}

function New-LoopbackPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try { return $listener.LocalEndpoint.Port } finally { $listener.Stop() }
}

$php = (Get-Command php).Source
$composer = (Get-Command composer).Source
$rr = if ($RoadRunnerPath) { (Resolve-Path -LiteralPath $RoadRunnerPath).Path } else { (Get-Command rr).Source }
$framework = (Resolve-Path -LiteralPath $FrameworkPath).Path
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ('tusk-skeleton-' + [guid]::NewGuid().ToString('N'))
$project = Join-Path $scratch 'smoke-app'
$engine = Join-Path $scratch 'tusk'
$server = $null
$rrPid = $null
$cleanupSafe = $true
New-Item -ItemType Directory -Path $scratch | Out-Null

try {
    Invoke-Bounded 'go' @('build', '-mod=readonly', '-buildvcs=false', '-o', $engine, './cmd/tusk') $engineRoot 240 (Join-Path $scratch 'build.log') | Out-Null

    # The official generator writes only inside the new project below this scratch directory.
    $generate = 'require getenv("TUSK_FRAMEWORK_AUTOLOAD"); (new \Tusk\Cli\Generator\ProjectGenerator())->generate("smoke-app", "api");'
    Invoke-Bounded $php @('-r', $generate) $scratch 20 (Join-Path $scratch 'generate.log') @{ TUSK_FRAMEWORK_AUTOLOAD = (Join-Path $framework 'vendor/autoload.php') } | Out-Null
    foreach ($relative in @('bootstrap/app.php', 'config/app.php', 'routes/web.php', 'public/index.php', 'composer.json')) {
        if (-not (Test-Path -LiteralPath (Join-Path $project $relative))) { throw "generator omitted $relative" }
    }
    if (Test-Path -LiteralPath (Join-Path $project '.tusk/runtime/worker.php')) { throw 'generator supplied a runtime worker' }

    $composerFile = Join-Path $project 'composer.json'
    $composerConfig = Get-Content -Raw -LiteralPath $composerFile | ConvertFrom-Json -AsHashtable
    $composerConfig['repositories'] = @(@{ type = 'path'; url = $framework; options = @{ symlink = $false; versions = @{ 'tusk/framework' = 'dev-main' } } })
    [System.IO.File]::WriteAllText($composerFile, ($composerConfig | ConvertTo-Json -Depth 20), [System.Text.UTF8Encoding]::new($false))
    Invoke-Bounded $composer @('install', '--no-interaction', '--no-progress', '--prefer-dist') $project 180 (Join-Path $scratch 'composer.log') @{ COMPOSER_ALLOW_SUPERUSER = '1' } | Out-Null

    # Engine init is intentionally exercised after removing the generator's minimal config.
    Remove-Item -LiteralPath (Join-Path $project 'tusk.json')
    $init = Invoke-Bounded $engine @('init') $project 15 (Join-Path $scratch 'init.log')
    if ($init -notmatch 'Created tusk.json') { throw 'Engine init did not create tusk.json' }
    $httpPort, $controlPort, $statusPort, $rpcPort, $metricsPort = @(1..5 | ForEach-Object { New-LoopbackPort })
    if (@(@($httpPort, $controlPort, $statusPort, $rpcPort, $metricsPort) | Select-Object -Unique).Count -ne 5) { throw 'failed to allocate distinct ports' }
    $configFile = Join-Path $project 'tusk.json'
    $config = Get-Content -Raw -LiteralPath $configFile | ConvertFrom-Json -AsHashtable
    $config['address'] = '127.0.0.1'
    $config['port'] = $httpPort
    $config['worker_count'] = 1
    $config['php_binary'] = $php
    $config['control']['enabled'] = $true
    $config['control']['address'] = '127.0.0.1'
    $config['control']['port'] = $controlPort
    $config['runtime']['status_address'] = "127.0.0.1:$statusPort"
    $config['runtime']['rpc_address'] = "tcp://127.0.0.1:$rpcPort"
    $config['runtime']['metrics_address'] = "127.0.0.1:$metricsPort"
    [System.IO.File]::WriteAllText($configFile, ($config | ConvertTo-Json -Depth 20), [System.Text.UTF8Encoding]::new($false))

    $frameworkBin = Join-Path $project 'vendor/bin/tusk'
    if (-not (Test-Path -LiteralPath $frameworkBin)) { throw 'Composer did not install the Framework build command' }
    New-Item -ItemType Directory -Path (Join-Path $project '.tusk') -Force | Out-Null
    Invoke-Bounded $php @($frameworkBin, 'build') $project 60 (Join-Path $scratch 'framework-build.log') | Out-Null

    $stdout = Join-Path $scratch 'server.stdout.log'
    $stderr = Join-Path $scratch 'server.stderr.log'
    $previousPath = $env:PATH
    $previousTemp = $env:TMPDIR
    $previousGo = @{}
    foreach ($name in @('GOTMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOPATH', 'GOTOOLCHAIN', 'GOTELEMETRY', 'GOENV')) {
        $previousGo[$name] = [Environment]::GetEnvironmentVariable($name)
    }
    $env:PATH = (Split-Path -Parent $rr) + [System.IO.Path]::PathSeparator + $previousPath
    $env:TMPDIR = $scratch
    $env:GOTMPDIR = $scratch
    $env:GOCACHE = Join-Path $scratch 'gocache'
    $env:GOMODCACHE = Join-Path $scratch 'gomodcache'
    $env:GOPATH = Join-Path $scratch 'gopath'
    $env:GOTOOLCHAIN = 'local'
    $env:GOTELEMETRY = 'off'
    $env:GOENV = 'off'
    try {
        $server = Start-Process -FilePath $engine -ArgumentList 'start' -WorkingDirectory $project -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    } finally {
        $env:PATH = $previousPath
        $env:TMPDIR = $previousTemp
        foreach ($name in $previousGo.Keys) { [Environment]::SetEnvironmentVariable($name, $previousGo[$name]) }
    }
    $rrPid = Wait-RoadRunnerProcess -EngineId $server.Id -ProjectRoot $project -EngineProcess $server -Diagnostics @($stdout, $stderr)
    $childBinary = (& readlink -f "/proc/$rrPid/exe").Trim()
    if ([System.IO.Path]::GetFileName($childBinary) -ne 'rr') { throw "independently identified process is not RoadRunner: $childBinary" }
    $client = [System.Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromSeconds(2)
    try {
        $deadline = [DateTime]::UtcNow.AddSeconds(45)
        $ready = $false
        while ([DateTime]::UtcNow -lt $deadline) {
            if ($server.HasExited) { throw "tusk start exited $($server.ExitCode) before readiness" }
            try {
                $probe = $client.GetAsync("http://127.0.0.1:$controlPort/v1/readyz").GetAwaiter().GetResult()
                if ($probe.IsSuccessStatusCode) { $ready = $true; break }
            } catch { }
            Start-Sleep -Milliseconds 250
        }
        if (-not $ready) { throw 'RoadRunner readiness timed out after 45 seconds' }
        $worker = Join-Path $project '.tusk/runtime/worker.php'
        if (-not (Test-Path -LiteralPath $worker -PathType Leaf)) { throw 'Engine did not generate .tusk/runtime/worker.php' }
        $workerText = Get-Content -Raw -LiteralPath $worker
        if ($workerText -notmatch 'bootstrap/app.php' -or $workerText -notmatch 'runWorker\(' -or $workerText -match 'NativeLoopAdapter|NDJSON') { throw 'generated worker does not use the Framework RoadRunner application' }
        $response = $client.GetAsync("http://127.0.0.1:$httpPort/").GetAwaiter().GetResult()
        $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode -or $body -ne 'Welcome to Tusk!') { throw "unexpected PHP application response: $([int] $response.StatusCode) $body" }
    } finally {
        $client.Dispose()
    }

    & /bin/kill -TERM $server.Id
    if ($LASTEXITCODE -ne 0 -or -not $server.WaitForExit(15000)) { throw 'Engine did not exit within 15 seconds of SIGTERM' }
    & /bin/kill -0 $rrPid 2>$null
    if ($LASTEXITCODE -eq 0) { throw "RoadRunner child $rrPid remained after Engine shutdown" }
    if (Test-Path -LiteralPath (Join-Path $project '.tusk/runtime/worker.php')) { throw 'generated worker remained active after shutdown' }
    $quarantine = @(Get-ChildItem -LiteralPath (Join-Path $project '.tusk/runtime') -Filter '.worker-quarantine-*' -File)
    if ($server.ExitCode -eq 0) {
        if ($quarantine.Count) { throw 'Engine exited successfully but left an unreported worker quarantine artifact' }
        Write-Output 'PASS skeleton smoke: generated PHP response through RoadRunner; Engine, child, and worker shut down cleanly'
    } else {
        $shutdownLog = (Get-Content -Raw -LiteralPath $stderr) + (Get-Content -Raw -LiteralPath $stdout)
        $unexpectedShutdown = @(
            'RoadRunner did not become ready',
            'RoadRunner failed',
            'control server failed',
            'control server stopped unexpectedly',
            'stop control server:',
            'stop runtime:',
            'clean RoadRunner config:'
        )
        $expectedQuarantinePattern = 'worker preserved in quarantine at "[^"]+": cannot conditionally unlink "[^"]+" by file identity on Linux'
        $expectedRuntimeFailurePattern = 'Runtime failed:\s*clean generated worker:\s*' + $expectedQuarantinePattern
        if ($quarantine.Count -ne 1 -or
            $shutdownLog -notmatch $expectedRuntimeFailurePattern -or
            $shutdownLog -notmatch 'Shutting down gracefully\.\.\.' -or $shutdownLog -notmatch 'Server stopped\.' -or
            @($unexpectedShutdown | Where-Object { $shutdownLog -match [regex]::Escape($_) }).Count) {
            throw "Engine exited $($server.ExitCode) after SIGTERM without the documented quarantine outcome: $shutdownLog"
        }
        Write-Output "PASS skeleton smoke: generated PHP response through RoadRunner; Engine and child exited; preserved quarantine $($quarantine[0].Name) reported"
    }
} catch {
    foreach ($log in @('build.log', 'generate.log', 'composer.log', 'init.log', 'framework-build.log', 'server.stdout.log', 'server.stderr.log')) {
        $path = Join-Path $scratch $log
        if (Test-Path -LiteralPath $path) { Write-Error "$log`n$(Get-Content -Raw -LiteralPath $path)" -ErrorAction Continue }
    }
    throw
} finally {
    $knownRoadRunnerPids = @($rrPid) + @(Get-RoadRunnerProcessIds -EngineId $(if ($server) { $server.Id } else { 0 }) -ProjectRoot $project)
    if ($server -and -not $server.HasExited) {
        & /bin/kill -TERM $server.Id 2>$null
        if (-not $server.WaitForExit(10000)) {
            try { $server.Kill($true) } catch { $cleanupSafe = $false }
            if (-not $server.WaitForExit(10000) -or ($rrPid -and -not (Wait-ProcessTreeExit -ProcessIds @($rrPid)))) { $cleanupSafe = $false }
        }
    }
    Stop-And-ReapRoadRunner -KnownProcessIds $knownRoadRunnerPids -EngineId $(if ($server) { $server.Id } else { 0 }) -ProjectRoot $project
    if ($server -and $cleanupSafe) { $server.Dispose() }
    if ($cleanupSafe -and (Test-Path -LiteralPath $scratch)) {
        if (-not $IsWindows) { & /bin/chmod -R u+w -- $scratch 2>$null }
        Remove-Item -LiteralPath $scratch -Recurse -Force
    }
    if (-not $cleanupSafe) { Write-Warning "Preserved fixture at $scratch because a process did not exit after Kill" }
}
