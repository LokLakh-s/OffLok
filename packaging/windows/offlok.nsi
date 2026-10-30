; OffLok's Windows installer (NSIS 3). Built by packaging/windows/build.ps1 (or build.sh) from a staging folder:
;   makensis -DVERSION=<x.y.z> -DSTAGE=<folder> -DOUTFILE=<setup.exe> offlok.nsi
; The staging folder holds offlokd.exe, offlok.exe, offlok-desktop.exe, LICENSE.txt, offlok.ico and unbound\
; (NLnet Labs' unbound.exe and its LICENSE). Silent: setup.exe /S, uninstall.exe /S.

Unicode true
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
!include "nsDialogs.nsh"

!ifndef VERSION
  !error "VERSION is not defined"
!endif
!ifndef STAGE
  !error "STAGE is not defined"
!endif
!ifndef OUTFILE
  !define OUTFILE "offlok-${VERSION}-setup.exe"
!endif

!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\OffLok"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

Name "OffLok"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\OffLok"
RequestExecutionLevel admin
BrandingText "OffLok ${VERSION}"
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "OffLok"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "OffLok installer"
VIAddVersionKey "LegalCopyright" "MIT, LokLakh"

!define MUI_ICON "${STAGE}\offlok.ico"
!define MUI_UNICON "${STAGE}\offlok.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\offlok-desktop.exe"
!insertmacro MUI_PAGE_LICENSE "${STAGE}\LICENSE.txt"
Page custom NoticePage NoticeLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "French"

LangString TooOld ${LANG_ENGLISH} "OffLok needs 64-bit Windows 10 version 2004 (build 19041) or later."
LangString TooOld ${LANG_FRENCH} "OffLok demande Windows 10 64 bits, version 2004 (build 19041) ou plus récente."
LangString ServiceFailed ${LANG_ENGLISH} "The OffLok service could not start. Your DNS is unchanged."
LangString ServiceFailed ${LANG_FRENCH} "Le service OffLok n'a pas pu démarrer. Ton DNS n'a pas changé."
LangString NoticeTitle ${LANG_ENGLISH} "Before installing"
LangString NoticeTitle ${LANG_FRENCH} "Avant l'installation"
LangString NoticeSubtitle ${LANG_ENGLISH} "What OffLok changes on this computer, and what it sends."
LangString NoticeSubtitle ${LANG_FRENCH} "Ce qu'OffLok change sur cet ordinateur, et ce qu'il envoie."
LangString NoticeDNS ${LANG_ENGLISH} "OffLok sets the DNS of every network connection of this computer to itself (127.0.0.1 and ::1): names are then looked up from the root servers, by this computer. Each connection's original DNS is kept: turning OffLok off or uninstalling it gives it back."
LangString NoticeDNS ${LANG_FRENCH} "OffLok règle le DNS de chaque connexion réseau de cet ordinateur sur lui-même (127.0.0.1 et ::1) : les noms sont alors cherchés depuis les serveurs racine, par cet ordinateur. Le DNS d'origine de chaque connexion est gardé : éteindre ou désinstaller OffLok le rend."
LangString NoticeUpdates ${LANG_ENGLISH} "Look for updates once a day, on GitHub, which then sees this computer's address. An update's signature is checked before it installs."
LangString NoticeUpdates ${LANG_FRENCH} "Chercher les mises à jour une fois par jour, sur GitHub, qui voit alors l'adresse de cet ordinateur. La signature d'une mise à jour est vérifiée avant son installation."
LangString NoticePrivacy ${LANG_ENGLISH} "Nothing else leaves this computer: no statistics, no record of what you look up. The blocking lists you switch on are downloaded from their authors."
LangString NoticePrivacy ${LANG_FRENCH} "Rien d'autre ne sort de cet ordinateur : ni statistiques, ni trace de ce que tu cherches. Les listes de blocage que tu actives sont téléchargées chez leurs auteurs."

Var Updates ; "on" or "off": the notice page's box
Var UpdatesBox

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "$(TooOld)" /SD IDOK
    Abort
  ${EndIf}
  SetRegView 64
  ReadRegStr $0 HKLM "Software\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
  ${If} $0 < 19041
    MessageBox MB_ICONSTOP "$(TooOld)" /SD IDOK
    Abort
  ${EndIf}
  StrCpy $Updates "on"
FunctionEnd

; What OffLok changes and sends, with the update check's box, before the first install. An upgrade keeps the
; settings it has: the page is skipped. A silent install shows no page and keeps the check on.
Function NoticePage
  SetShellVarContext all
  ${If} ${FileExists} "$APPDATA\OffLok\settings.json"
    Abort
  ${EndIf}
  !insertmacro MUI_HEADER_TEXT "$(NoticeTitle)" "$(NoticeSubtitle)"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 48u "$(NoticeDNS)"
  Pop $0
  ${NSD_CreateCheckbox} 0 56u 100% 30u "$(NoticeUpdates)"
  Pop $UpdatesBox
  ${If} $Updates == "on"
    ${NSD_Check} $UpdatesBox
  ${EndIf}
  ${NSD_CreateLabel} 0 94u 100% 30u "$(NoticePrivacy)"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function NoticeLeave
  ${NSD_GetState} $UpdatesBox $0
  ${If} $0 == ${BST_CHECKED}
    StrCpy $Updates "on"
  ${Else}
    StrCpy $Updates "off"
  ${EndIf}
FunctionEnd

Section "OffLok"
  SetShellVarContext all
  SetRegView 64
  ; An upgrade: the service gives the DNS back and lets go of its files. The tray app (and the command) may run:
  ; Windows lets a running program's file be renamed, not replaced, so the old one steps aside as .old, removed by
  ; the next install, and the tray app, seeing the new version, starts again from the new file.
  Delete "$INSTDIR\offlok-desktop.exe.old"
  Delete "$INSTDIR\offlok.exe.old"
  ${If} ${FileExists} "$INSTDIR\offlokd.exe"
    nsExec::ExecToLog '"$INSTDIR\offlokd.exe" uninstall'
  ${EndIf}
  ${If} ${FileExists} "$INSTDIR\offlok-desktop.exe"
    Rename "$INSTDIR\offlok-desktop.exe" "$INSTDIR\offlok-desktop.exe.old"
  ${EndIf}
  ${If} ${FileExists} "$INSTDIR\offlok.exe"
    Rename "$INSTDIR\offlok.exe" "$INSTDIR\offlok.exe.old"
  ${EndIf}

  SetOutPath "$INSTDIR"
  File "${STAGE}\offlokd.exe"
  File "${STAGE}\offlok.exe"
  File "${STAGE}\offlok-desktop.exe"
  File "${STAGE}\offlok.ico"
  File "${STAGE}\LICENSE.txt"
  SetOutPath "$INSTDIR\unbound"
  File "${STAGE}\unbound\unbound.exe"
  File "${STAGE}\unbound\LICENSE"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ${If} $Updates == "off"
    nsExec::ExecToLog '"$INSTDIR\offlokd.exe" install updates=off'
  ${Else}
    nsExec::ExecToLog '"$INSTDIR\offlokd.exe" install'
  ${EndIf}
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "$(ServiceFailed)" /SD IDOK
  ${EndIf}

  ; The tray icon starts with every session, and the window from the Start menu.
  WriteRegStr HKLM "${RUN_KEY}" "OffLok" '"$INSTDIR\offlok-desktop.exe" --hidden'
  CreateShortcut "$SMPROGRAMS\OffLok.lnk" "$INSTDIR\offlok-desktop.exe" "" "$INSTDIR\offlok.ico"

  WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayName" "OffLok"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "Publisher" "LokLakh"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "URLInfoAbout" "https://github.com/LokLakh-s/OffLok"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\offlok.ico"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  SetShellVarContext all
  SetRegView 64
  nsExec::Exec 'taskkill /F /IM offlok-desktop.exe'
  ; Stops and removes the service, and gives every interface its original DNS back.
  nsExec::ExecToLog '"$INSTDIR\offlokd.exe" uninstall'
  Pop $0

  DeleteRegValue HKLM "${RUN_KEY}" "OffLok"
  Delete "$SMPROGRAMS\OffLok.lnk"
  Delete "$INSTDIR\offlokd.exe"
  Delete "$INSTDIR\offlok.exe"
  Delete "$INSTDIR\offlok-desktop.exe"
  Delete "$INSTDIR\offlok-desktop.exe.old"
  Delete "$INSTDIR\offlok.exe.old"
  Delete "$INSTDIR\offlok.ico"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\unbound\unbound.exe"
  Delete "$INSTDIR\unbound\LICENSE"
  RMDir "$INSTDIR\unbound"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "${UNINSTALL_KEY}"
  ; OffLok's own files (settings, lists, the saved DNS settings), only once the DNS is back: otherwise they hold
  ; what a second try needs.
  ${If} $0 == 0
    RMDir /r "$APPDATA\OffLok" ; ProgramData, with SetShellVarContext all
  ${EndIf}
SectionEnd
