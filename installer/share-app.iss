; Compile through installer/build.ps1 so all inputs and sources are verified.
#ifndef AppVersion
  #error AppVersion must be supplied
#endif
#ifndef AppFileVersion
  #error AppFileVersion must be supplied
#endif
#ifndef DistDir
  #error DistDir must be supplied
#endif
#ifndef OutputDir
  #error OutputDir must be supplied
#endif

[Setup]
AppId={{9E7D147B-C72B-4B55-86C9-1D9A64379EE5}
AppName=Share App
AppVersion={#AppVersion}
VersionInfoVersion={#AppFileVersion}
AppPublisher=tanetakumi
AppPublisherURL=https://github.com/tanetakumi/remote-window-control
AppSupportURL=https://github.com/tanetakumi/remote-window-control/issues
AppUpdatesURL=https://github.com/tanetakumi/remote-window-control/releases
DefaultDirName={localappdata}\Programs\ShareApp
DefaultGroupName=Share App
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.26100
DisableDirPage=yes
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\share-host.exe
AppMutex=Local\ShareApp.Running.9E7D147B-C72B-4B55-86C9-1D9A64379EE5
SetupMutex=Local\ShareApp.Setup.9E7D147B-C72B-4B55-86C9-1D9A64379EE5
CloseApplications=yes
RestartApplications=no
OutputDir={#OutputDir}
OutputBaseFilename=ShareApp-{#AppVersion}-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[InstallDelete]
; Replace generated assets on upgrade so old web bundles / native DLLs vanish.
; User data lives in a separate directory and is never removed here.
Type: filesandordirs; Name: "{app}\web"
Type: filesandordirs; Name: "{app}\CaptureProbe"
Type: filesandordirs; Name: "{app}\scripts"
Type: filesandordirs; Name: "{app}\licenses"
Type: files; Name: "{app}\はじめにお読みください.txt"

[Files]
Source: "{#DistDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\Share App"; Filename: "{app}\share-host.exe"; WorkingDir: "{app}"
Name: "{autodesktop}\Share App"; Filename: "{app}\share-host.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\share-host.exe"; Description: "{cm:LaunchProgram,Share App}"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent

; There is deliberately no wildcard UninstallDelete: uninstallation removes
; installer-owned files, and preserves settings, logs and RDP credentials.
