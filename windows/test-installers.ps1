# These tests install real programs and a real service on a disposable CI runner.
# Refuse to run against an existing installation or user configuration.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$InstallersDir, [switch]$UserOnly)
$ErrorActionPreference = 'Stop'
$userConfig = Join-Path $env:LOCALAPPDATA 'Mediarium\config'
$serviceConfig = Join-Path $env:ProgramData 'Mediarium'
if (Test-Path -LiteralPath (Join-Path $userConfig 'app.db')) { throw 'Use a disposable account without an existing user-mode database.' }
if (!$UserOnly -and ((Get-Service Mediarium -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath (Join-Path $serviceConfig 'app.db')))) { throw 'Use a disposable runner without an existing Mediarium service/configuration.' }
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('mediarium-installer-test-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $testRoot | Out-Null
function Invoke-Setup([string]$File, [string]$Arguments) {
    # Start-Process -Wait also waits for descendants, including the background
    # user application the installer intentionally leaves running.
    $p = Start-Process -FilePath $File -ArgumentList $Arguments -PassThru
    if (!$p.WaitForExit(180000)) { throw "Installer did not finish within three minutes: $File" }
    if ($p.ExitCode -ne 0) { throw "Installer command failed with exit $($p.ExitCode): $File" }
}
function Assert-Ready([int]$Port) {
    $v = Invoke-RestMethod "http://127.0.0.1:$Port/api/version" -TimeoutSec 5
    if ($v.version -ne (Get-Content (Join-Path $PSScriptRoot '..\VERSION') -Raw).Trim()) { throw 'Incorrect running version' }
    $html = (Invoke-WebRequest "http://127.0.0.1:$Port/" -UseBasicParsing -TimeoutSec 5).Content
    if ($html -notmatch 'assets/' -or $html -match 'Placeholder') { throw 'Embedded frontend is missing' }
}
foreach ($mode in @('user','service')) {
    if ($UserOnly -and $mode -eq 'service') { continue }
    $installers = @(Get-ChildItem $InstallersDir -Filter "*-$mode-setup.exe")
    if ($installers.Count -ne 1) { throw "Expected exactly one $mode installer" }
    $installer = $installers[0]
    $installDir = Join-Path $testRoot $mode
    $configDir = if ($mode -eq 'user') { $userConfig } else { $serviceConfig }
    $port = if ($mode -eq 'user') { 8265 } else { 8264 }
    $setupArgs = '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR="' + $installDir + '" /LOG="' + (Join-Path $testRoot "$mode-install.log") + '"'
    try {
        Invoke-Setup $installer.FullName $setupArgs
        Assert-Ready $port
        if ($mode -eq 'service') {
            $svc = Get-CimInstance Win32_Service -Filter "Name='Mediarium'"
            if ($svc.StartName -ne 'NT AUTHORITY\LocalService' -or $svc.State -ne 'Running') { throw 'Incorrect service identity/state' }
            $beforeEnvironment = (Get-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Services\Mediarium).Environment
        } else {
            if (!$UserOnly -and (Get-Service Mediarium -ErrorAction SilentlyContinue)) { throw 'User installer created a service' }
            # A second launch must not open the same database twice.
            $duplicate = Start-Process (Join-Path $installDir 'mediarium.exe') -ArgumentList '--user-mode' -Wait -PassThru
            if ($duplicate.ExitCode) { throw 'Duplicate user launch failed' }
        }
        $keyHash = (Get-FileHash (Join-Path $configDir 'secret.key')).Hash
        'preserve me' | Set-Content (Join-Path $configDir 'installer-test-sentinel.txt')
        # Upgrade in place: clean shutdown, matching DB/key snapshot, restart.
        Invoke-Setup $installer.FullName $setupArgs
        Assert-Ready $port
        if ((Get-FileHash (Join-Path $configDir 'secret.key')).Hash -ne $keyHash) { throw 'Upgrade changed the encryption key' }
        if (!(Test-Path (Join-Path $configDir 'installer-test-sentinel.txt'))) { throw 'Upgrade removed configuration' }
        if ($mode -eq 'service' -and (Compare-Object $beforeEnvironment (Get-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Services\Mediarium).Environment)) { throw 'Upgrade changed the service environment' }
    } finally {
        $uninstaller = Join-Path $installDir 'unins000.exe'
        if (Test-Path -LiteralPath $uninstaller) { Invoke-Setup $uninstaller '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' }
    }
    if (!(Test-Path (Join-Path $configDir 'app.db')) -or !(Test-Path (Join-Path $configDir 'secret.key'))) { throw 'Uninstall removed configuration' }
    if ($mode -eq 'service' -and (Get-Service Mediarium -ErrorAction SilentlyContinue)) { throw 'Uninstall left the service registered' }
    Write-Output "$mode installer: installation, duplicate launch (user), upgrade, HTTP/frontend, encryption key and uninstall verified."
}
