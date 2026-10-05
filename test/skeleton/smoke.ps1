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
    $missing += 'modern Framework generator (bootstrap-app.stub)'
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

function Invoke-Bounded {
    param([string] $File, [string[]] $Arguments, [string] $Directory, [int] $Seconds, [string] $Log, [hashtable] $Environment = @{})

    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $File
    $start.WorkingDirectory = $Directory
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.Environment['TMPDIR'] = $scratch
    $start.Environment['GOCACHE'] = Join-Path $scratch 'gocache'
    $start.Environment['COMPOSER_HOME'] = Join-Path $scratch 'composer-home'
    $start.Environment['COMPOSER_CACHE_DIR'] = Join-Path $scratch 'composer-cache'
    foreach ($argument in $Arguments) { [void] $start.ArgumentList.Add($argument) }
    foreach ($key in $Environment.Keys) { $start.Environment[$key] = [string] $Environment[$key] }
    $process = [System.Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit($Seconds * 1000)) {
            $process.Kill($true)
            throw "$File timed out after $Seconds seconds"
        }
        $output = $stdout.GetAwaiter().GetResult() + $stderr.GetAwaiter().GetResult()
        [System.IO.File]::WriteAllText($Log, $output)
        if ($process.ExitCode -ne 0) { throw "$File exited $($process.ExitCode): $output" }
        return $output
    } finally {
        $process.Dispose()
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
    $env:PATH = (Split-Path -Parent $rr) + [System.IO.Path]::PathSeparator + $previousPath
    $env:TMPDIR = $scratch
    try {
        $server = Start-Process -FilePath $engine -ArgumentList 'start' -WorkingDirectory $project -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    } finally {
        $env:PATH = $previousPath
        $env:TMPDIR = $previousTemp
    }
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
        $children = @(& pgrep -P $server.Id 2>$null)
        if ($children.Count -ne 1) { throw "expected one supervised RoadRunner process, found $($children.Count)" }
        $rrPid = [int] $children[0]
        $childBinary = (& readlink -f "/proc/$rrPid/exe").Trim()
        if ([System.IO.Path]::GetFileName($childBinary) -ne 'rr') { throw "supervised child is not RoadRunner: $childBinary" }
    } finally {
        $client.Dispose()
    }

    & /bin/kill -TERM $server.Id
    if ($LASTEXITCODE -ne 0 -or -not $server.WaitForExit(15000)) { throw 'Engine did not exit within 15 seconds of SIGTERM' }
    if ($server.ExitCode -ne 0) { throw "Engine exited $($server.ExitCode) after SIGTERM" }
    & /bin/kill -0 $rrPid 2>$null
    if ($LASTEXITCODE -eq 0) { throw "RoadRunner child $rrPid remained after Engine shutdown" }
    if (Test-Path -LiteralPath (Join-Path $project '.tusk/runtime/worker.php')) { throw 'generated worker remained active after shutdown' }
    Write-Output 'PASS skeleton smoke: generated PHP response through RoadRunner; Engine, child, and worker shut down cleanly'
} catch {
    foreach ($log in @('build.log', 'generate.log', 'composer.log', 'init.log', 'framework-build.log', 'server.stdout.log', 'server.stderr.log')) {
        $path = Join-Path $scratch $log
        if (Test-Path -LiteralPath $path) { Write-Error "$log`n$(Get-Content -Raw -LiteralPath $path)" -ErrorAction Continue }
    }
    throw
} finally {
    if ($server -and -not $server.HasExited) {
        & /bin/kill -TERM $server.Id 2>$null
        if (-not $server.WaitForExit(10000)) { $server.Kill($true); [void] $server.WaitForExit(5000) }
    }
    if ($rrPid) {
        & /bin/kill -0 $rrPid 2>$null
        if ($LASTEXITCODE -eq 0) {
            & /bin/kill -TERM $rrPid 2>$null
            Start-Sleep -Seconds 1
            & /bin/kill -0 $rrPid 2>$null
            if ($LASTEXITCODE -eq 0) { & /bin/kill -KILL $rrPid 2>$null }
        }
    }
    if ($server) { $server.Dispose() }
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
}
