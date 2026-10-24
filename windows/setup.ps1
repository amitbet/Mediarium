[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('service','user')][string]$Mode,
    [Parameter(Mandatory)][ValidateSet('Prepare','Install','Uninstall','Resume')][string]$Action,
    [Parameter(Mandatory)][string]$InstallDir
)
$ErrorActionPreference = 'Stop'
$exe = Join-Path $InstallDir 'mediarium.exe'
$configDir = if ($Mode -eq 'service') { Join-Path $env:ProgramData 'Mediarium' } else { Join-Path $env:LOCALAPPDATA 'Mediarium\config' }
$port = if ($Mode -eq 'service') { 8264 } else { 8265 }
$service = if ($Mode -eq 'service') { Get-Service Mediarium -ErrorAction SilentlyContinue }
if ($service) {
    $serviceInfo = Get-CimInstance Win32_Service -Filter "Name='Mediarium'"
    if ($serviceInfo.PathName.Trim('"') -ne $exe) { throw 'The existing Mediarium service uses another installation folder. Install to that folder to upgrade it.' }
    $environment = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Services\Mediarium').Environment
    foreach ($entry in $environment) {
        if ($entry -like 'CONFIG_DIR=*') { $configDir = $entry.Substring(11) }
        if ($entry -like 'APP_PORT=*') { $port = [int]$entry.Substring(9) }
    }
}
function Stop-Mediarium {
    if ($Mode -eq 'service' -and $service) {
        $oldProcess = if ($serviceInfo.ProcessId) { Get-Process -Id $serviceInfo.ProcessId -ErrorAction SilentlyContinue }
        Stop-Service Mediarium
        (Get-Service Mediarium).WaitForStatus('Stopped', [TimeSpan]::FromSeconds(45))
        if ($oldProcess -and !$oldProcess.WaitForExit(45000)) { throw 'The service did not exit after stopping' }
    } elseif ($Mode -eq 'user' -and (Test-Path -LiteralPath $exe)) {
        $stop = Start-Process -FilePath $exe -ArgumentList '--stop-user-mode' -PassThru -Wait -WindowStyle Hidden
        if ($stop.ExitCode) { throw 'Could not stop the user application gracefully. See config\user.log.' }
    }
}
function Start-Mediarium {
    if ($Mode -eq 'service') { Start-Service Mediarium }
    else { Start-Process -FilePath $exe -ArgumentList '--user-mode' -WindowStyle Hidden }
}
switch ($Action) {
    'Prepare' {
        Stop-Mediarium
        try {
            # Test before replacing files. Never open an existing app's web UI
            # and mistake it for the instance being installed.
            $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Any, $port)
            try { $listener.Start() } finally { $listener.Stop() }
            if (Test-Path -LiteralPath (Join-Path $configDir 'app.db')) {
                $backupRoot = if ($Mode -eq 'service') { Join-Path $env:ProgramData 'Mediarium-installer-backups' } else { Join-Path $env:LOCALAPPDATA 'Mediarium\backups' }
                New-Item -ItemType Directory -Path $backupRoot -Force | Out-Null
                if ($Mode -eq 'service') {
                    & icacls.exe $backupRoot /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' /Q | Out-Null
                    if ($LASTEXITCODE) { throw 'Could not protect the configuration backup' }
                }
                $snapshot = Join-Path $backupRoot ((Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N').Substring(0,8))
                New-Item -ItemType Directory -Path $snapshot | Out-Null
                Copy-Item -LiteralPath $configDir -Destination (Join-Path $snapshot 'config') -Recurse
                if (Test-Path -LiteralPath $exe) { Copy-Item -LiteralPath $exe -Destination $snapshot }
            }
        } catch {
            if (Test-Path -LiteralPath $exe) { Start-Mediarium }
            throw
        }
    }
    'Install' {
        if ($Mode -eq 'service' -and !$service) {
            # A fresh installation must work on machines without a D: drive.
            & (Join-Path $InstallDir 'install.ps1') -Binary $exe -InstallDir $InstallDir -ConfigDir $configDir -DataDir (Join-Path $env:ProgramData 'MediariumData') -Port $port
        } else {
            # Existing service identity, environment, media paths and ACLs stay
            # intact; an upgrade only replaces the installed program and tools.
            Start-Mediarium
        }
        $ready = $false
        for ($attempt = 0; $attempt -lt 45; $attempt++) {
            try {
                $response = Invoke-RestMethod "http://127.0.0.1:$port/api/version" -TimeoutSec 2
                if ($response.version) { $ready = $true; break }
            } catch {}
            Start-Sleep -Seconds 1
        }
        if (!$ready) { throw "Mediarium did not start. See $configDir for its log." }
    }
    'Uninstall' {
        Stop-Mediarium
        if ($Mode -eq 'service' -and $service) {
            & sc.exe delete Mediarium | Out-Null
            if ($LASTEXITCODE) { throw 'Could not remove the service' }
            Get-NetFirewallRule -Name 'Mediarium-Web','Mediarium-Peers-TCP','Mediarium-Peers-UDP' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
        }
        # Configuration, encryption keys, backups and media are never removed.
    }
    'Resume' {
        if ((Test-Path -LiteralPath $exe) -and ($Mode -eq 'user' -or $service)) { Start-Mediarium }
    }
}
