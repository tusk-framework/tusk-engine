$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'smoke.ps1') -PrerequisitesOnly

$versions = Get-FrameworkPathRepositoryVersions
if ($versions['tusk-framework/framework'] -ne 'dev-main') {
    throw 'Composer path repository must map tusk-framework/framework to dev-main'
}
Write-Output 'PASS skeleton package mapping: tusk-framework/framework => dev-main'
