# Wails Windows Standalone Implementation Plan

**Status:** Phase 2 In Progress (CI & Installer)  
**Last Updated:** December 17, 2025  
**Project:** ViiB MediaHub

---

## 📋 Overview

ViiB MediaHub now supports two build modes:

| Build Mode | Script | Output | Description |
|------------|--------|--------|-------------|
| **Web** | `.\scripts\build.ps1` | `build\ViiB-MediaHub.exe` | Opens in system browser |
| **Wails** | `.\scripts\build-wails.ps1` | `backend\cmd\wails\build\bin\ViiB-MediaHub.exe` | Native WebView2 window |

---

## ✅ Phase 0 — COMPLETE (PoC)

**Completed:** December 17, 2025

**Deliverables:**
- ✅ Wails v2.11.0 installed and verified (`wails doctor` passes)
- ✅ CGO/SQLite compilation working with mingw-w64
- ✅ WebView2 integration verified
- ✅ HTTP server + WebView loading pattern implemented

**Decision:** Proceed with Option A (HTTP server model, not direct bindings)

---

## ✅ Phase 1 — COMPLETE (Windows Build)

**Completed:** December 17, 2025

**Deliverables:**

| File | Description |
|------|-------------|
| `backend/cmd/wails/main.go` | Wails entry point with API proxy (281 lines) |
| `backend/cmd/wails/wails.json` | Wails configuration |
| `scripts/build-wails.ps1` | Production build script |
| `scripts/dev-wails.ps1` | Development mode script |
| `docs/wails-windows-setup.md` | Developer setup guide |

**Build Output:**
- Executable: `backend/cmd/wails/build/bin/ViiB-MediaHub.exe`
- Size: 22.4 MB (Release mode)

**Architecture:**
```
┌─────────────────────────────────────────────────────────┐
│                    Wails Application                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────────┐      ┌─────────────────────────┐ │
│  │  WebView2 Window  │      │    HTTP Server          │ │
│  │  (Frontend UI)    │      │    (127.0.0.1:random)   │ │
│  └────────┬──────────┘      └────────────▲────────────┘ │
│           │                              │              │
│           │ /api/* requests              │              │
│           └──────────────────────────────┘              │
│                  (API Proxy via Handler)                │
│                                                         │
│  • Frontend served from embedded distFS                 │
│  • API requests proxied to HTTP server                  │
│  • SSE connections work (library events, downloads)     │
│  • Database: %APPDATA%/ViiB-MediaHub/library.db         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Verified Working:**
- ✅ App launches in native WebView2 window
- ✅ Frontend loads correctly from embedded assets
- ✅ API requests proxied to HTTP server
- ✅ SSE connections work (library events, download progress)
- ✅ Database initialization
- ✅ Logging to file

---

## 🔄 Phase 2 — IN PROGRESS (CI & Installer)

**Goal:** Automated builds, code signing, and professional installer.

### 2.1 GitHub Actions Workflow

Create `.github/workflows/build-windows.yml`:

```yaml
name: Build Windows (Wails)

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

env:
  CGO_ENABLED: 1

jobs:
  build-windows:
    runs-on: windows-latest
    
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache-dependency-path: backend/go.sum

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install MSYS2
        uses: msys2/setup-msys2@v2
        with:
          msystem: MINGW64
          update: true
          install: mingw-w64-x86_64-gcc

      - name: Add MSYS2 to PATH
        run: echo "C:\msys64\mingw64\bin" >> $env:GITHUB_PATH

      - name: Install Wails
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: Install dependencies
        run: npm ci

      - name: Build frontend
        run: npm run build

      - name: Copy frontend to Wails
        run: |
          $dest = "backend\cmd\wails\frontend\dist"
          if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
          New-Item -ItemType Directory -Path "backend\cmd\wails\frontend" -Force
          Copy-Item -Recurse "dist" $dest

      - name: Build Wails app
        working-directory: backend/cmd/wails
        run: wails build -platform windows/amd64 -ldflags "-s -w"

      - name: Sign executable
        if: env.WIN_CERT_PFX != ''
        env:
          WIN_CERT_PFX: ${{ secrets.WIN_CERT_PFX }}
          WIN_CERT_PASSWORD: ${{ secrets.WIN_CERT_PASSWORD }}
        run: |
          $cert = "cert.pfx"
          [Convert]::FromBase64String($env:WIN_CERT_PFX) | Set-Content $cert -AsByteStream
          signtool sign /f $cert /p $env:WIN_CERT_PASSWORD /tr http://timestamp.digicert.com /td sha256 /fd sha256 "backend\cmd\wails\build\bin\ViiB-MediaHub.exe"
          Remove-Item $cert

      - name: Create archive
        run: |
          $version = "${{ github.ref_name }}" -replace '^v', ''
          if (-not $version) { $version = "dev" }
          Compress-Archive -Path "backend\cmd\wails\build\bin\ViiB-MediaHub.exe" -DestinationPath "ViiB-MediaHub-$version-windows-x64.zip"

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: ViiB-MediaHub-windows-x64
          path: ViiB-MediaHub-*.zip

      - name: Create Release
        if: startsWith(github.ref, 'refs/tags/')
        uses: softprops/action-gh-release@v1
        with:
          files: ViiB-MediaHub-*.zip
          draft: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 2.2 NSIS Installer

Create `installer/viib.nsi`:

```nsis
!include "MUI2.nsh"

Name "ViiB MediaHub"
OutFile "ViiB-MediaHub-Setup.exe"
InstallDir "$PROGRAMFILES64\ViiB MediaHub"
RequestExecutionLevel admin

!define MUI_ICON "..\backend\cmd\viib\icon.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "Install"
    SetOutPath $INSTDIR
    File "..\backend\cmd\wails\build\bin\ViiB-MediaHub.exe"
    
    CreateDirectory "$SMPROGRAMS\ViiB MediaHub"
    CreateShortcut "$SMPROGRAMS\ViiB MediaHub\ViiB MediaHub.lnk" "$INSTDIR\ViiB-MediaHub.exe"
    CreateShortcut "$DESKTOP\ViiB MediaHub.lnk" "$INSTDIR\ViiB-MediaHub.exe"
    
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub" "DisplayName" "ViiB MediaHub"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub" "UninstallString" "$INSTDIR\Uninstall.exe"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub" "DisplayIcon" "$INSTDIR\ViiB-MediaHub.exe"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub" "Publisher" "ViiB"
SectionEnd

Section "Uninstall"
    Delete "$INSTDIR\ViiB-MediaHub.exe"
    Delete "$INSTDIR\Uninstall.exe"
    RMDir "$INSTDIR"
    
    Delete "$SMPROGRAMS\ViiB MediaHub\*.lnk"
    RMDir "$SMPROGRAMS\ViiB MediaHub"
    Delete "$DESKTOP\ViiB MediaHub.lnk"
    
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ViiBMediaHub"
SectionEnd
```

### 2.3 Smoke Test Script

Create `scripts/smoke-test.ps1`:

```powershell
# Smoke test for ViiB MediaHub Wails build
$ErrorActionPreference = "Stop"

$exePath = "backend\cmd\wails\build\bin\ViiB-MediaHub.exe"

if (-not (Test-Path $exePath)) {
    Write-Error "Executable not found: $exePath"
    exit 1
}

Write-Host "Starting ViiB MediaHub..." -ForegroundColor Cyan
$process = Start-Process -FilePath $exePath -PassThru

Start-Sleep -Seconds 5

try {
    if ($process.HasExited) {
        Write-Error "Process exited unexpectedly with code: $($process.ExitCode)"
        exit 1
    }
    Write-Host "✅ App started successfully" -ForegroundColor Green
} finally {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
}
```

### 2.4 Code Signing Setup

**Secrets Required:**
| Secret | Description |
|--------|-------------|
| `WIN_CERT_PFX` | Base64-encoded .pfx certificate |
| `WIN_CERT_PASSWORD` | Certificate password |

**To encode certificate:**
```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("cert.pfx")) | Set-Clipboard
```

### Phase 2 Checklist

- [x] Create `.github/workflows/build-windows.yml`
- [ ] Test CI build on `windows-latest`
- [x] Create `installer/viib.nsi`
- [x] Create `scripts/smoke-test.ps1`
- [ ] Obtain code signing certificate (optional for alpha)
- [ ] Test install/uninstall flow
- [ ] Create first release

---

## 📋 Phase 3 — PLANNED (Auto-update & Distribution)

**Goal:** In-app update mechanism and distribution channels.

### Tasks

1. **Update Checker**
   - Check GitHub Releases API for new versions
   - Compare semantic versions
   - Show notification in UI

2. **Update Installer**
   - Download new version in background
   - Apply update on next launch
   - Verify signature before applying

3. **Distribution Channels**
   - GitHub Releases (primary)
   - WinGet (optional)
   - Chocolatey (optional)

### Phase 3 Checklist

- [ ] Implement version checking against GitHub API
- [ ] Add update notification UI component
- [ ] Implement download and apply logic
- [ ] Test full update cycle
- [ ] Submit to WinGet (optional)

---

## 📚 Quick Reference

### Build Commands

```powershell
# Web build (browser)
.\scripts\build.ps1

# Wails build (native window)
.\scripts\build-wails.ps1

# Wails build with debug tools
.\scripts\build-wails.ps1 -Debug

# Wails dev mode
.\scripts\dev-wails.ps1
```

### Project Structure

```
backend/cmd/
├── viib/           # Web build entry point
│   └── main.go
└── wails/          # Wails build entry point
    ├── main.go
    ├── wails.json
    └── frontend/dist/

scripts/
├── build.ps1       # Web build
├── build-wails.ps1 # Wails build
├── dev.ps1         # Web dev
└── dev-wails.ps1   # Wails dev
```

### CLI Flags (Wails Build)

```
-data <path>    Custom data directory
-port <n>       API server port (0 = auto)
-debug          Enable dev tools
```

---

## 📚 References

- [Wails Documentation](https://wails.io/docs/introduction)
- [NSIS Documentation](https://nsis.sourceforge.io/Docs/)
- [GitHub Actions](https://docs.github.com/en/actions)
- [Code Signing](https://docs.microsoft.com/en-us/windows/win32/seccrypto/signtool)

---

*Last updated: December 17, 2025*
