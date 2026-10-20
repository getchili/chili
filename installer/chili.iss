; Installateur Windows de Chili (Inno Setup 6).
;
; Construit par build.ps1 -Installer, ou à la main :
;   ISCC.exe installer\chili.iss
;
; Installation pour l'utilisateur courant, sans droits administrateur :
;   - chili.exe dans %LOCALAPPDATA%\Programs\Chili\bin, ajouté au PATH ;
;   - icônes des fichiers .chl / .chlb ;
;   - extension VS Code (si VS Code est installé) ;
;   - exemples, et désinstallateur dans « Applications installées ».

#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif

[Setup]
AppId={{6E2B7C1A-3C4F-4F1E-9B7A-C41C1C0DE001}
AppName=Chili
AppVersion={#AppVersion}
AppVerName=Chili {#AppVersion}
AppPublisher=getchili
AppPublisherURL=https://getchili.github.io
AppSupportURL=https://github.com/getchili
DefaultDirName={autopf}\Chili
DefaultGroupName=Chili
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ChangesEnvironment=yes
ChangesAssociations=yes
OutputDir=..\dist
OutputBaseFilename=ChiliSetup-{#AppVersion}
SetupIconFile=..\editors\windows\chili.ico
UninstallDisplayIcon={app}\bin\chili.exe
UninstallDisplayName=Chili {#AppVersion}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "french"; MessagesFile: "compiler:Languages\French.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
french.TaskPath=Ajouter chili au PATH (commande « chili » dans tous les terminaux)
english.TaskPath=Add chili to PATH (use the "chili" command in any terminal)
french.TaskAssoc=Associer les fichiers .chl et .chlb à Chili (icônes)
english.TaskAssoc=Associate .chl and .chlb files with Chili (icons)
french.TaskVSCode=Installer l'extension Chili pour VS Code (coloration et icônes)
english.TaskVSCode=Install the Chili extension for VS Code (highlighting and icons)
french.InstallingVSCode=Installation de l'extension VS Code…
english.InstallingVSCode=Installing the VS Code extension…
french.SourceType=Programme Chili
english.SourceType=Chili program
french.BytecodeType=Bytecode Chili
english.BytecodeType=Chili bytecode

[Messages]
french.FinishedLabel=Chili est installé.%n%nOuvre un NOUVEAU terminal (PowerShell ou cmd) et tape :%n%n    chili check-port%n%npour détecter ta carte ESP32. Les exemples sont dans le dossier d'installation, sous « examples ».
english.FinishedLabel=Chili is installed.%n%nOpen a NEW terminal (PowerShell or cmd) and type:%n%n    chili check-port%n%nto detect your ESP32 board. Examples are in the installation folder, under "examples".

[Tasks]
Name: "addtopath"; Description: "{cm:TaskPath}"
Name: "associate"; Description: "{cm:TaskAssoc}"
Name: "vscode"; Description: "{cm:TaskVSCode}"; Check: VSCodeFound

[InstallDelete]
; Exemples retirés : ne pas les laisser traîner après une mise à jour.
Type: files; Name: "{app}\examples\sos.chl"
Type: files; Name: "{app}\examples\fib.chl"

[Files]
Source: "..\chili.exe"; DestDir: "{app}\bin"; Flags: ignoreversion
Source: "..\editors\windows\chili.ico"; DestDir: "{app}\icons"; Flags: ignoreversion
Source: "..\editors\windows\chili-bytecode.ico"; DestDir: "{app}\icons"; Flags: ignoreversion
Source: "..\examples\*.chl"; DestDir: "{app}\examples"; Flags: ignoreversion
Source: "..\editors\vscode\chili-lang-0.2.0.vsix"; DestDir: "{tmp}"; Flags: deleteafterinstall; Tasks: vscode

[Registry]
; PATH de l'utilisateur
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}\bin"; \
  Check: NeedsAddPath(ExpandConstant('{app}\bin')); Tasks: addtopath

; Associations de fichiers (icônes, nom du type dans l'Explorateur)
Root: HKCU; Subkey: "Software\Classes\.chl"; ValueType: string; ValueData: "Chili.Source"; Flags: uninsdeletevalue; Tasks: associate
Root: HKCU; Subkey: "Software\Classes\Chili.Source"; ValueType: string; ValueData: "{cm:SourceType}"; Flags: uninsdeletekey; Tasks: associate
Root: HKCU; Subkey: "Software\Classes\Chili.Source\DefaultIcon"; ValueType: string; ValueData: """{app}\icons\chili.ico"",0"; Tasks: associate
Root: HKCU; Subkey: "Software\Classes\.chlb"; ValueType: string; ValueData: "Chili.Bytecode"; Flags: uninsdeletevalue; Tasks: associate
Root: HKCU; Subkey: "Software\Classes\Chili.Bytecode"; ValueType: string; ValueData: "{cm:BytecodeType}"; Flags: uninsdeletekey; Tasks: associate
Root: HKCU; Subkey: "Software\Classes\Chili.Bytecode\DefaultIcon"; ValueType: string; ValueData: """{app}\icons\chili-bytecode.ico"",0"; Tasks: associate

[Run]
Filename: "{cmd}"; Parameters: "/c """"{code:VSCodeCli}"" --install-extension ""{tmp}\chili-lang-0.2.0.vsix"" --force"""; \
  StatusMsg: "{cm:InstallingVSCode}"; Flags: runhidden waituntilterminated; Tasks: vscode

[Code]
const
  EnvKey = 'Environment';

{ Vrai si dir n'est pas déjà dans le PATH de l'utilisateur. }
function NeedsAddPath(Dir: string): Boolean;
var
  Path: string;
begin
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Path) + ';') = 0;
end;

{ Chemin de la commande « code » de VS Code, ou '' s'il n'est pas installé. }
function VSCodeCli(Param: string): string;
begin
  Result := ExpandConstant('{localappdata}\Programs\Microsoft VS Code\bin\code.cmd');
  if FileExists(Result) then exit;
  Result := ExpandConstant('{commonpf64}\Microsoft VS Code\bin\code.cmd');
  if FileExists(Result) then exit;
  Result := '';
end;

function VSCodeFound: Boolean;
begin
  Result := VSCodeCli('') <> '';
end;

// À la désinstallation, retire le dossier bin du PATH.
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Path, Dir: string;
  P: Integer;
begin
  if CurUninstallStep <> usPostUninstall then exit;
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then exit;
  Dir := ExpandConstant('{app}\bin');
  P := Pos(';' + Uppercase(Dir), Uppercase(Path));
  if P > 0 then
    Delete(Path, P, Length(Dir) + 1)
  else if Pos(Uppercase(Dir), Uppercase(Path)) = 1 then
    Delete(Path, 1, Length(Dir) + 1);
  RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Path);
end;
