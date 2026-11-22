[Setup]
AppId=ReiaClash.Windows
AppName=ReiaClash
AppVerName=ReiaClash
DefaultDirName={localappdata}\Programs\ReiaClash
DisableDirPage=no
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=..\ico\reia-clash.ico
UninstallDisplayIcon={app}\reia.exe
OutputBaseFilename=ReiaClash-Setup
ArchiveExtraction=enhanced/nopassword
CloseApplications=no
RestartApplications=no

[Languages]
Name: "zhcn"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
zhcn.CreateStartMenuShortcut=创建开始菜单快捷方式
en.CreateStartMenuShortcut=Create a Start Menu shortcut

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; Flags: unchecked
Name: "startmenuicon"; Description: "{cm:CreateStartMenuShortcut}"

[Files]
Source: "{#ArchivePath}"; Flags: dontcopy noencryption nocompression
Source: "{tmp}\ReiaClash-payload.7z"; DestDir: "{app}"; ExternalSize: {#PayloadBytes}; Flags: external extractarchive recursesubdirs createallsubdirs ignoreversion

[Icons]
Name: "{userdesktop}\ReiaClash"; Filename: "{app}\reia.exe"; WorkingDir: "{app}"; AppUserModelID: "ReiaClash.Windows"; Tasks: desktopicon
Name: "{userprograms}\ReiaClash"; Filename: "{app}\reia.exe"; WorkingDir: "{app}"; AppUserModelID: "ReiaClash.Windows"; Tasks: startmenuicon

[Run]
Filename: "{app}\reia.exe"; WorkingDir: "{app}"; Description: "{cm:LaunchProgram,ReiaClash}"; Flags: postinstall nowait skipifsilent unchecked runasoriginaluser

[Code]
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  try
    ExtractTemporaryFile('ReiaClash-payload.7z');
  except
    Result := GetExceptionMessage;
  end;
end;
