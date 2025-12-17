; ViiB MediaHub NSIS Installer Script
; Requires NSIS 3.x with MUI2
;
; To build: makensis viib.nsi
; Or use NSIS Menu > Compile NSI scripts

!include "MUI2.nsh"
!include "FileFunc.nsh"

;--------------------------------
; General Configuration

!define PRODUCT_NAME "ViiB MediaHub"
!define PRODUCT_PUBLISHER "ViiB"
!define PRODUCT_WEB_SITE "https://github.com/ajbergh/ViiB-MediaHub"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub"
!define PRODUCT_UNINST_ROOT_KEY "HKLM"

Name "${PRODUCT_NAME}"
OutFile "ViiB-MediaHub-Setup.exe"
InstallDir "$PROGRAMFILES64\ViiB MediaHub"
InstallDirRegKey HKLM "${PRODUCT_UNINST_KEY}" "InstallLocation"
RequestExecutionLevel admin
ShowInstDetails show
ShowUnInstDetails show

;--------------------------------
; Version Info (update for each release)

!define PRODUCT_VERSION "1.0.0.0"
VIProductVersion "${PRODUCT_VERSION}"
VIAddVersionKey "ProductName" "${PRODUCT_NAME}"
VIAddVersionKey "CompanyName" "${PRODUCT_PUBLISHER}"
VIAddVersionKey "FileDescription" "${PRODUCT_NAME} Installer"
VIAddVersionKey "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey "LegalCopyright" "Copyright ${PRODUCT_PUBLISHER}"

;--------------------------------
; Interface Settings

!define MUI_ABORTWARNING
!define MUI_ICON "..\backend\cmd\wails\build\appicon.ico"
!define MUI_UNICON "..\backend\cmd\wails\build\appicon.ico"

; Welcome page
!define MUI_WELCOMEPAGE_TITLE "Welcome to ${PRODUCT_NAME} Setup"
!define MUI_WELCOMEPAGE_TEXT "This wizard will guide you through the installation of ${PRODUCT_NAME}.$\r$\n$\r$\nViiB MediaHub is a local media player with a modern interface.$\r$\n$\r$\nClick Next to continue."

; Finish page
!define MUI_FINISHPAGE_RUN "$INSTDIR\ViiB-MediaHub.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${PRODUCT_NAME}"
!define MUI_FINISHPAGE_LINK "Visit ${PRODUCT_NAME} on GitHub"
!define MUI_FINISHPAGE_LINK_LOCATION "${PRODUCT_WEB_SITE}"

;--------------------------------
; Pages

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

;--------------------------------
; Languages

!insertmacro MUI_LANGUAGE "English"

;--------------------------------
; Installer Sections

Section "Main Application" SecMain
    SectionIn RO ; Read-only, always installed
    
    SetOutPath "$INSTDIR"
    
    ; Copy main executable
    File "..\backend\cmd\wails\build\bin\ViiB-MediaHub.exe"
    
    ; Create Start Menu shortcuts
    CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk" "$INSTDIR\ViiB-MediaHub.exe" "" "$INSTDIR\ViiB-MediaHub.exe" 0
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall.lnk" "$INSTDIR\Uninstall.exe" "" "$INSTDIR\Uninstall.exe" 0
    
    ; Create Desktop shortcut
    CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\ViiB-MediaHub.exe" "" "$INSTDIR\ViiB-MediaHub.exe" 0
    
    ; Create uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    
    ; Write registry keys for Add/Remove Programs
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "UninstallString" "$INSTDIR\Uninstall.exe"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayIcon" "$INSTDIR\ViiB-MediaHub.exe"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "InstallLocation" "$INSTDIR"
    
    ; Get installed size
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

;--------------------------------
; Uninstaller Section

Section "Uninstall"
    ; Kill running instance
    nsExec::ExecToLog 'taskkill /F /IM "ViiB-MediaHub.exe"'
    Sleep 1000
    
    ; Remove files
    Delete "$INSTDIR\ViiB-MediaHub.exe"
    Delete "$INSTDIR\Uninstall.exe"
    
    ; Remove directories
    RMDir "$INSTDIR"
    
    ; Remove Start Menu items
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall.lnk"
    RMDir "$SMPROGRAMS\${PRODUCT_NAME}"
    
    ; Remove Desktop shortcut
    Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
    
    ; Remove registry keys
    DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}"
    
    ; Note: User data in %APPDATA%\ViiB-MediaHub is NOT removed
    ; This preserves user's music library and settings
SectionEnd

;--------------------------------
; Functions

Function .onInit
    ; Check if already running
    FindWindow $0 "" "ViiB MediaHub"
    StrCmp $0 0 notRunning
        MessageBox MB_ICONEXCLAMATION|MB_OKCANCEL "ViiB MediaHub is currently running. Click OK to close it and continue, or Cancel to abort." IDOK closeApp IDCANCEL abortInstall
    closeApp:
        nsExec::ExecToLog 'taskkill /F /IM "ViiB-MediaHub.exe"'
        Sleep 2000
        Goto notRunning
    abortInstall:
        Abort
    notRunning:
FunctionEnd

Function un.onInit
    MessageBox MB_ICONQUESTION|MB_YESNO "Are you sure you want to completely remove ${PRODUCT_NAME}?$\r$\n$\r$\nNote: Your music library and settings in %APPDATA% will be preserved." IDYES +2
    Abort
FunctionEnd
