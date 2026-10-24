# Windows fork notes

This file records every local departure from rdborg/Mediarium (the `upstream`
remote). `origin` is the public amitbet/Mediarium fork; the maintained branch is
`main`. The former `windows-service` branch is retained as historical context;
upstream does not need a reserved local main branch.
The original fork base is upstream v3.2.3 (`86dd096`). Local changes include the
committed Windows adaptation (`635c541`) and the pre-existing AudioBook Bay work
preserved before the 2026-10-24 upstream merge.

## Native Windows runtime

- `cmd/app/main.go`: dispatch to SCM when launched as a Windows service and pass
  a parent context into the application so SCM stop/shutdown cancels it normally.
- `cmd/app/service_windows.go`: native SCM handler, startup/running/stopping
  states, service errors, cancellation, in-process UI restarts, and append-only
  `CONFIG_DIR/service.log`. `service_other.go` preserves console behavior elsewhere.
  `service_windows_test.go` covers stop/shutdown, failure and restart behavior.
- `go.mod`: golang.org/x/sys is a direct dependency for Windows service/routing APIs.
- `internal/torrentclient/gateway_windows.go`: discover the default IPv4 gateway
  through Windows routing/adapter APIs for NAT-PMP. `gateway_other.go` retains the
  original /proc/net/route reader; `portmap.go` shares the parser and mapper.
- `internal/organizer/move.go`: clean input/source/destination paths in MapPath
  before matching, making slash styles consistent on Windows.

## Windows packaging and migration

- `windows/build.ps1`: install frontend dependencies, build the embedded UI, then
  compile a trimmed Windows executable using the root VERSION.
- `windows/install.ps1`: install/update the native LocalService service, wait for
  the old process to exit, configure delayed startup/recovery and environment,
  grant configuration/media access, add firewall rules, and check HTTP readiness.
- `windows/install-tools.ps1`: install checksum-verified x64 7-Zip and par2 tools.
- `windows/migrate-config.py`: copy a Linux configuration using SQLite backup,
  preserve its encryption key, check integrity, and translate stored media/helper
  paths without changing source data or encrypted credentials.
  `windows/test_migrate_config.py` verifies migration behavior.
- `windows/README.md`: build, install, migration, operational paths, limitations
  and uninstall instructions. `.gitignore` excludes Python __pycache__ output.

## AudioBook Bay and category-independent connection tests

These changes were already present but uncommitted in the working tree used for
this upgrade; they are preserved as fork changes, not attributed to upstream.

- `internal/indexers/builtin/audiobookbay.yml`: public audiobook definition,
  mirror addresses, request spacing, normalized queries, audio format/size/date
  extraction, audiobook category and details-page info-hash magnet downloads.
  Seeder counts remain unknown because the site does not supply them.
- `internal/indexers/cardigann_engine.go`: test a site's unfiltered feed instead
  of restricting connection tests to movie/TV categories, allowing book-only sites.
- `internal/indexers/builtin_audiobookbay_test.go`: local HTTP fixtures cover
  headers, query normalization, metadata, category filtering, magnet generation
  and the connection-test regression. No real download is needed for these tests.
- `internal/indexers/definitions_test.go`: locate the fixture definition by ID
  rather than assuming it sorts first when additional built-ins are present.
- `docs/indexers.md`: document AudioBook Bay setup and behavior.

## Windows installers and release automation (2026-10-24)

- `.github/workflows/windows-release.yml`: isolated Windows installer CI on main,
  pull requests, manual dispatch and windows-v<VERSION>.<build> tags. Builds the UI
  and a CGO-free Windows GUI executable, runs lifecycle/migration checks, packages
  both installers, tests actual install/upgrade/uninstall and publishes installers
  with SHA-256 checksums only after success. Upstream's Linux/Docker workflow is
  preserved. No signing certificate or upstream private API registration is bundled.
- `windows/installer.iss`: separate service (admin) and current-user (no elevation)
  Inno Setup installers, stable distinct app identities, uninstallers, Start menu
  controls and optional user sign-in startup. Tools and licences are bundled.
- `windows/setup.ps1`: installer lifecycle, clean stop, private configuration and
  binary backups, port checks, readiness, preserving existing service environment
  and paths, uninstall without deleting user data. Fresh service installs use
  ProgramData media and need no D: drive. User mode uses port 8265 and separate data.
- `windows/package.ps1`: compile both installers, stage pinned tools and licences,
  and write checksums. `windows/test-installers.ps1` exercises both complete flows
  on disposable CI accounts, including key/environment preservation.
- `windows/build.ps1`: optional console-free GUI build and explicit CGO=0.
  `install-tools.ps1` can stage tools in a writable folder without elevation;
  `install.ps1` can configure a freshly packaged executable already at its target
  and grants LocalService read/execute access for custom installation folders.
- `cmd/app/user_windows.go`, `user_windows_test.go`, `main.go`, `service_other.go`:
  native --user-mode/--stop-user-mode dispatch, isolated per-user defaults and log,
  a per-user/session/config named event for single instance and graceful stop,
  verified PID/executable identity before waiting for shutdown, in-process restart,
  and focused lifecycle tests. Non-Windows console behavior is preserved.
- `windows/README.md` documents installers, operational paths, release tags and CI.
- `README.md` links the Windows fork's downloads/guide and names main as the
  maintained branch. `CHANGELOG.md` records the fork's Windows installer features.

## Upgrade procedure

### 2026-10-24: upstream v3.7.2

Merged release tag `v3.7.2` (`0fcdaf5`, also upstream/main at fetch time)
into `windows-service` with a merge commit. There were no conflicts. All 22
files differing from upstream are covered by the inventory above. The Windows
README now names v3.7.2 and the required Go 1.26.10 toolchain.

Validation: the Windows build, all 615 frontend tests, Python migration test,
and focused Go service/configuration/organizer/cleanup/indexer/store tests pass.
Selected API restart, folder settings and path-handling tests also pass.
Frontend lint completes with existing upstream warnings. The API filesystem
comparison test `TestFilesystemCheckUsesTheNearestExistingFolder` fails because
upstream's unchanged `internal/organizer/samefs_windows.go` explicitly returns
`supported=false`. Native Windows volume detection remains unsupported; this
is not introduced by the merge. The full torrent suite also retains the Windows
fixture-cleanup limitation described in `windows/README.md`.

Compare `git diff upstream/main -- .` and `git log upstream/main..HEAD` against
this inventory on every merge. Preserve local work before merging a release tag.
Build and test before stopping the installed service. Back up the stopped
configuration (database and encryption key together) and installed executable
before replacement; schema migrations mean binary-only rollback is insufficient.
Native Windows builds retain the upstream VERSION, and do not contain upstream's
private built-in API registrations; configured personal credentials remain in DB.
