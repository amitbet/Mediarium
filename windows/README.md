# Windows installers and native runtime

Download installers from the [public Windows releases](https://github.com/amitbet/Mediarium/releases).
Both x64 installers bundle the web interface, 7-Zip, PAR2 and their licences.
No Docker, WSL, Go, Node or online tool download is needed on the target PC.

| Installer | Privileges and startup | Configuration | Default media | Web UI |
| --- | --- | --- | --- | --- |
| `service-setup.exe` | Administrator; LocalService, delayed automatic startup, crash recovery | `%ProgramData%\Mediarium` | `%ProgramData%\MediariumData` for new installs | `http://localhost:8264` |
| `user-setup.exe` | Current user; Start/Stop shortcuts, optional startup at sign-in | `%LOCALAPPDATA%\Mediarium\config` | `%LOCALAPPDATA%\Mediarium\data` | `http://localhost:8265` |

User mode installs under `%LOCALAPPDATA%\Programs\Mediarium` and never creates a
Windows service or changes firewall rules. The different default ports and data
directories allow both modes to coexist; they do not share a library database.
To move an existing installation between modes, stop it and migrate its matching
database and encryption key deliberately. Do not point two instances at one DB.

Upgrades stop the matching instance gracefully, snapshot its configuration and
old executable, replace the program/tools, then restart and check HTTP readiness.
Service snapshots are in `%ProgramData%\Mediarium-installer-backups` (administrators
and SYSTEM only); user snapshots are in `%LOCALAPPDATA%\Mediarium\backups`.
Existing service environment values, identity, custom media paths and permissions
are retained. Uninstall removes the application, shortcuts and its service/firewall
rules where applicable; configuration, encryption keys, backups and media remain.
These community installers are not Authenticode-signed.

## User-mode controls

The Start menu has **Start Mediarium**, **Stop Mediarium** and **Open Mediarium**.
The executable also accepts `--user-mode` and `--stop-user-mode`; the latter waits
for a graceful shutdown. A named event prevents two copies from opening the same
configuration in one Windows session. The UI's Restart action restarts in process.
Environment overrides such as `CONFIG_DIR` and `APP_PORT` apply to both commands;
use the same overrides when stopping a customized instance. Logs are in `user.log`
inside its configuration folder. User mode stops at sign-out; use service mode
for unattended operation.

## Build and release CI

The maintained fork branch is `main`; upstream code is tracked by the `upstream`
remote. `.github/workflows/windows-release.yml` builds both installers on pushes
to `main`, relevant pull requests, and manual runs. It tests service/user lifecycle,
migration, actual install, duplicate user launch, upgrade, embedded UI, retained
encryption key/environment and uninstall on a disposable Windows runner.

Push a tag `windows-v<VERSION>.<build>`, such as `windows-v3.7.2.1`, to publish
the verified installers and SHA-256 checksums. Alternatively run **Windows
installers** in GitHub Actions with **publish** enabled; the run number becomes
the fourth Windows version component. The tag's first three components must match
the root `VERSION`, and the build component must be between 0 and 65535.
Ordinary branch/PR runs keep downloadable CI artifacts without publishing a release.
Windows tags are separate from upstream's `v*` Linux/Docker release pipeline.

For a local package build, install Inno Setup 6.7 or newer, then run:

```powershell
.\windows\build.ps1 -Windowed
.\windows\package.ps1 -BuildNumber 1
```

The build disables CGO so no compiler runtime DLLs are needed. Use
`-ISCC C:\path\to\ISCC.exe` if the compiler is outside its standard directory.
Do not run `test-installers.ps1` on a production account: it installs real programs
and a real service, and refuses existing installations. It is intended for CI.

## Manual service installation and Linux migration

Based on stable upstream v3.7.2. Go 1.26.10 and Node/npm are required to build.
See [fork_nodes.md](../fork_nodes.md) for the complete inventory of fork changes.
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

Upstream currently publishes Linux updates. Use this fork's Windows installer to
upgrade Windows. `install-tools.ps1` stages checksum-verified 7-Zip 26.04 and par2cmdline
1.4.0 into the application's tools directory; the installer puts it on the service's
PATH. The tool installer requires x64 Windows and 7-Zip (preinstalled on CI), or a
Windows `tar.exe` with LZMA support. ZIP files use PowerShell's built-in extractor.
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
