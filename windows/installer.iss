#ifndef Mode
  #error Mode must be service or user
#endif
#ifndef AppVersion
  #error AppVersion must match VERSION
#endif
#ifndef BuildNumber
  #define BuildNumber "0"
#endif
#if Mode == "service"
  #define ModeName "Windows service"
  #define WebPort "8264"
#else
  #if Mode != "user"
    #error Invalid Mode
  #endif
  #define ModeName "Current user"
  #define WebPort "8265"
#endif

[Setup]
AppId=Mediarium-{#Mode}
AppName=Mediarium ({#ModeName})
AppVersion={#AppVersion}
AppPublisher=Mediarium Windows fork
AppPublisherURL=https://github.com/amitbet/Mediarium
AppSupportURL=https://github.com/amitbet/Mediarium/issues
AppUpdatesURL=https://github.com/amitbet/Mediarium/releases
VersionInfoVersion={#AppVersion}.{#BuildNumber}
#if Mode == "service"
DefaultDirName={autopf}\Mediarium
PrivilegesRequired=admin
#else
DefaultDirName={localappdata}\Programs\Mediarium
PrivilegesRequired=lowest
#endif
DefaultGroupName=Mediarium ({#ModeName})
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
LicenseFile=..\LICENSE
OutputDir=..\dist\installers
OutputBaseFilename=Mediarium-{#AppVersion}.{#BuildNumber}-windows-x64-{#Mode}-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\mediarium.exe
CloseApplications=no
RestartApplications=no
SetupLogging=yes
DisableProgramGroupPage=yes

[Files]
Source: "..\dist\mediarium.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\tools\*"; DestDir: "{app}\tools"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "..\LICENSE"; DestDir: "{app}"
Source: "..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"
Source: "README.md"; DestDir: "{app}"; DestName: "README-Windows.md"
Source: "install.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "setup.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "setup.ps1"; Flags: dontcopy

#if Mode == "user"
[Tasks]
Name: "autostart"; Description: "Start Mediarium when I sign in"; Flags: unchecked
#endif

[Icons]
Name: "{group}\Open Mediarium"; Filename: "http://localhost:{#WebPort}/"
#if Mode == "user"
Name: "{group}\Start Mediarium"; Filename: "{app}\mediarium.exe"; Parameters: "--user-mode"
Name: "{group}\Stop Mediarium"; Filename: "{app}\mediarium.exe"; Parameters: "--stop-user-mode"
Name: "{userstartup}\Mediarium"; Filename: "{app}\mediarium.exe"; Parameters: "--user-mode"; Tasks: autostart
#endif
Name: "{group}\Uninstall Mediarium"; Filename: "{uninstallexe}"

[Run]
Filename: "http://localhost:{#WebPort}/"; Description: "Open Mediarium"; Flags: shellexec postinstall skipifsilent runasoriginaluser

[Code]
var
  Prepared, Installed: Boolean;

function Lifecycle(Script, Action: String): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'),
    '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + Script +
    '" -Mode {#Mode} -Action ' + Action + ' -InstallDir "' + ExpandConstant('{app}') + '"',
    '', SW_HIDE, ewWaitUntilTerminated, ResultCode) and (ResultCode = 0);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  ExtractTemporaryFile('setup.ps1');
  if Lifecycle(ExpandConstant('{tmp}\setup.ps1'), 'Prepare') then
    Prepared := True
  else
    Result := 'Could not prepare Mediarium. Check that its port is free and the existing installation can stop and be backed up. See the setup log.';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    if not Lifecycle(ExpandConstant('{app}\setup.ps1'), 'Install') then
      RaiseException('Mediarium could not start. Check its configuration log before retrying setup.');
    Installed := True;
  end;
end;

procedure DeinitializeSetup;
begin
  if Prepared and not Installed then
    Lifecycle(ExpandConstant('{tmp}\setup.ps1'), 'Resume');
end;

function InitializeUninstall: Boolean;
begin
  Result := Lifecycle(ExpandConstant('{app}\setup.ps1'), 'Uninstall');
  if not Result then
    MsgBox('Mediarium could not stop safely. Uninstall was cancelled; try again after checking its log.', mbError, MB_OK);
end;
