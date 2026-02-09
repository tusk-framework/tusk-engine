# Tusk Engine - Windows Auto-Installer
# Usage: iwr -useb https://tusk.sh/install.ps1 | iex

$TuskHome = "$HOME\.tusk"
$BinDir = "$TuskHome\bin"
$PhpDir = "$TuskHome\php"

Write-Host "--- Tusk Engine Auto-Installer ---" -ForegroundColor Cyan

# 1. Create Directories
if (!(Test-Path $BinDir)) { New-Item -ItemType Directory -Force -Path $BinDir }
if (!(Test-Path $PhpDir)) { New-Item -ItemType Directory -Force -Path $PhpDir }

# 2. Download Tusk Engine (Placeholder URL)
$TuskUrl = "https://github.com/tusk-framework/tusk-engine/releases/latest/download/tusk-windows-amd64.exe"
Write-Host "Downloading Tusk Engine..." -ForegroundColor Yellow
try {
    Invoke-WebRequest -Uri $TuskUrl -OutFile "$BinDir\tusk.exe" -ErrorAction Stop
}
catch {
    Write-Warning "Could not download Tusk binary from $TuskUrl. Please ensure the release exists."
}

# 3. Download & Setup PHP (Sidecar)
$PhpUrl = "https://windows.php.net/downloads/releases/php-8.3.3-Win32-vs16-x64.zip"
$PhpZip = "$TuskHome\php.zip"

if (!(Test-Path "$PhpDir\php.exe")) {
    Write-Host "Downloading Portable PHP..." -ForegroundColor Yellow
    Invoke-WebRequest -Uri $PhpUrl -OutFile $PhpZip
    
    Write-Host "Extracting PHP..." -ForegroundColor Yellow
    Expand-Archive -Path $PhpZip -DestinationPath $PhpDir -Force
    Remove-Item $PhpZip
}
else {
    Write-Host "PHP already installed in $PhpDir" -ForegroundColor Green
}

# 4. Update PATH
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($CurrentPath -notlike "*$BinDir*") {
    Write-Host "Adding Tusk bin to User PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$CurrentPath;$BinDir;$PhpDir", "User")
    $env:Path += ";$BinDir;$PhpDir"
}

Write-Host "`nInstallation Complete!" -ForegroundColor Green
Write-Host "Please restart your terminal."
Write-Host "Try: tusk --help"
