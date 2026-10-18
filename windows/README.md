# Native Windows service

Based on stable upstream v3.2.3. Go 1.26.9 and Node/npm are required to build.
The UI is embedded in the executable; Docker, WSL and a service wrapper are unnecessary.

```powershell
.\windows\build.ps1
# Administrator PowerShell:
.\windows\install-tools.ps1
.\windows\install.ps1
```

Default paths: program `C:\Program Files\Mediarium\mediarium.exe`, private configuration
`C:\ProgramData\Mediarium`, media `D:\Mediarium\data`. The web UI uses port 8264.
The service runs as LocalService with delayed automatic startup and crash recovery.
SCM stop/shutdown cancels the application's context and follows its normal shutdown
path, including stopping downloads and closing SQLite. The UI Restart action runs the
application again inside the service. Logs append to `service.log` in the config folder.

```powershell
Get-Service Mediarium
Restart-Service Mediarium
Get-Content C:\ProgramData\Mediarium\service.log -Tail 40
```

Copy `app.db` and `secret.key` together when migrating. Stop the source first, or use
SQLite's backup API; never copy a live SQLite database without its WAL. Translate saved
Linux media paths to the Windows data directory. Do not copy Linux update executables.
Keep only one active application using the same media/download folders. A Windows
service cannot use drive mappings from an interactive login; use local paths or UNC
paths and an account with access to those shares.

Upstream currently publishes Linux updates. Rebuild and rerun the installer to upgrade
Windows. `install-tools.ps1` installs checksum-verified 7-Zip 26.04 and par2cmdline
1.4.0 into the application's tools directory; the installer puts it on the service's
PATH. The tool installer requires x64 Windows and the included `tar.exe`.
FlareSolverr is a separate service; migrate its URL if it remains on
another host. Built-in API registrations from upstream release binaries are not
included in source builds; configured personal API credentials migrate with the DB.

For migration from an extracted Linux configuration snapshot:
```powershell
python windows/migrate-config.py C:/private/pi-snapshot C:/ProgramData/Mediarium --data D:/Mediarium/data --helper-url http://192.168.1.101:8191
python windows/test_migrate_config.py
```

The destination must not already contain a database. The script copies the matching
encryption key and supporting files, uses SQLite backup, checks integrity, rewrites
stored paths (including paths inside JSON), and leaves encrypted values untouched.
Keep the snapshot private. Source configuration and media are never modified.
Windows routing APIs locate the gateway for NAT-PMP; Linux's `/proc` is not used.

Validation on Windows: service lifecycle, configuration, filesystem, organizer,
cleanup, selected API path/settings/restart tests and the migration test pass.
The upstream torrent integration suite transfers fixture data successfully but has
Windows file-lock failures while deleting some temporary fixture directories after
clients close. The full torrent integration suite is not claimed to pass on Windows.

If local PowerShell policy disables scripts, invoke the installer in a process with
`powershell.exe -NoProfile -ExecutionPolicy Bypass -File windows/install.ps1`.

To uninstall (keeps all configuration and media):
```powershell
Stop-Service Mediarium
sc.exe delete Mediarium
Get-NetFirewallRule -Name 'Mediarium-Web','Mediarium-Peers-TCP','Mediarium-Peers-UDP' | Remove-NetFirewallRule
```
