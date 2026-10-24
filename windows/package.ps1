[CmdletBinding()]
param([int]$BuildNumber = 0, [string]$ISCC = '')
$ErrorActionPreference = 'Stop'
$repoDir = Split-Path $PSScriptRoot
if (!$ISCC) {
    $ISCC = @("${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe") | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}
if (!$ISCC) { throw 'Install Inno Setup, or pass -ISCC with the compiler path.' }
$version = (Get-Content (Join-Path $repoDir 'VERSION') -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$' -or $BuildNumber -lt 0 -or $BuildNumber -gt 65535) { throw 'Windows installers need a three-part release version and a build number from 0 to 65535.' }
if (!(Test-Path (Join-Path $repoDir 'dist\mediarium.exe'))) { throw 'Build dist\mediarium.exe first.' }
# Include tools and their licences so target machines need no network download.
& (Join-Path $PSScriptRoot 'install-tools.ps1') -InstallDir (Join-Path $repoDir 'dist')
foreach ($mode in 'service','user') {
    & $ISCC "/DMode=$mode" "/DAppVersion=$version" "/DBuildNumber=$BuildNumber" (Join-Path $PSScriptRoot 'installer.iss')
    if ($LASTEXITCODE) { throw "Inno Setup failed for $mode" }
}
$outDir = Join-Path $repoDir 'dist\installers'
$sums = Get-ChildItem $outDir -Filter '*.exe' | Sort-Object Name | ForEach-Object { ((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower()) + '  ' + $_.Name }
$sums | Set-Content -Encoding ascii (Join-Path $outDir 'sha256sums.txt')
