#Requires -RunAsAdministrator
[CmdletBinding()]
param(
    [string]$Binary = '',
    [string]$InstallDir = 'C:\Program Files\Mediarium',
    [string]$ConfigDir = 'C:\ProgramData\Mediarium',
    [string]$DataDir = 'D:\Mediarium\data',
    [int]$Port = 8264
)
$ErrorActionPreference = 'Stop'
if (!$Binary) { $Binary = Join-Path $PSScriptRoot '..\dist\mediarium.exe' }
$Binary = (Resolve-Path -LiteralPath $Binary).Path
$existing = Get-Service Mediarium -ErrorAction SilentlyContinue
if ($existing) {
    $serviceProcessId = (Get-CimInstance Win32_Service -Filter "Name='Mediarium'").ProcessId
    $serviceProcess = if ($serviceProcessId) { Get-Process -Id $serviceProcessId -ErrorAction SilentlyContinue }
    Stop-Service Mediarium
    $existing.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    # SCM can report Stopped just before the process releases its executable.
    if ($serviceProcess -and !$serviceProcess.WaitForExit(30000)) { throw 'Previous Mediarium process did not exit' }
}
New-Item -ItemType Directory -Force -Path $InstallDir,$ConfigDir,$DataDir | Out-Null
if ($Binary -ne (Join-Path $InstallDir 'mediarium.exe')) {
    Copy-Item -LiteralPath $Binary -Destination "$InstallDir\mediarium.exe" -Force
}
# A custom installation folder may inherit private per-user ACLs. The service
# needs read/execute access to its program and bundled tools, never write access.
& icacls.exe $InstallDir /grant '*S-1-5-19:(OI)(CI)RX' /T /Q
if ($LASTEXITCODE) { throw 'Program access for LocalService failed' }
foreach ($folder in 'downloads','movies','series','music','ebooks','audiobooks') {
    New-Item -ItemType Directory -Force -Path (Join-Path $DataDir $folder) | Out-Null
}
if (!$existing) {
    New-Service -Name Mediarium -DisplayName 'Mediarium' -Description 'Mediarium media manager (native Windows service)' -BinaryPathName ('"' + $InstallDir + '\mediarium.exe"') -StartupType Automatic | Out-Null
}
& sc.exe config Mediarium start= delayed-auto obj= 'NT AUTHORITY\LocalService'
if ($LASTEXITCODE) { throw 'Service configuration failed' }
& sc.exe failure Mediarium reset= 86400 actions= restart/10000/restart/30000/restart/60000
if ($LASTEXITCODE) { throw 'Service recovery configuration failed' }
& sc.exe failureflag Mediarium 1
if ($LASTEXITCODE) { throw 'Service failure recovery flag failed' }
$environment = @(
    "APP_PORT=$Port", "CONFIG_DIR=$ConfigDir", "DOWNLOADS_DIR=$DataDir\downloads",
    "MOVIES_DIR=$DataDir\movies", "TV_DIR=$DataDir\series", "MUSIC_DIR=$DataDir\music",
    "EBOOKS_DIR=$DataDir\ebooks", "AUDIOBOOKS_DIR=$DataDir\audiobooks",
    'MEDIARIUM_SUPERVISED=1', 'TZ=Asia/Jerusalem'
    ('PATH=' + $InstallDir + '\tools;' + [Environment]::GetEnvironmentVariable('Path', 'Machine'))
)
New-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Services\Mediarium' -Name Environment -PropertyType MultiString -Value $environment -Force | Out-Null
# The configuration contains encrypted credentials and their key. Only administrators,
# SYSTEM and the service account can read it. Media remains at its existing location.
& icacls.exe $ConfigDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-19:(OI)(CI)M' /Q
if ($LASTEXITCODE) { throw 'Configuration ACL failed' }
& icacls.exe (Join-Path $ConfigDir '*') /reset /T /Q
if ($LASTEXITCODE) { throw 'Configuration file inheritance failed' }
& icacls.exe $DataDir /grant '*S-1-5-19:(OI)(CI)M' /T /Q
if ($LASTEXITCODE) { throw 'Media ACL failed' }
if (!(Get-NetFirewallRule -Name Mediarium-Web -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name Mediarium-Web -DisplayName 'Mediarium web (local subnet)' -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port -RemoteAddress LocalSubnet -Program "$InstallDir\mediarium.exe" | Out-Null
}
foreach ($protocol in 'TCP','UDP') {
    if (!(Get-NetFirewallRule -Name "Mediarium-Peers-$protocol" -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -Name "Mediarium-Peers-$protocol" -DisplayName "Mediarium torrent peers ($protocol)" -Direction Inbound -Action Allow -Protocol $protocol -LocalPort 58264 -Program "$InstallDir\mediarium.exe" | Out-Null
    }
}
Start-Service Mediarium
$ready = $false
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
        $response = Invoke-RestMethod "http://127.0.0.1:$Port/api/version" -TimeoutSec 2
        if ($response.version) { $ready = $true; break }
    } catch { Start-Sleep -Seconds 1 }
}
if (!$ready) { throw "Mediarium did not become ready. Check $ConfigDir\service.log" }
Get-Service Mediarium
