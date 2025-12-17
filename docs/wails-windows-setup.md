# Wails Windows Development Setup Guide

This guide covers setting up your Windows development environment for building ViiB MediaHub as a native desktop application using Wails.

## Overview

ViiB MediaHub supports two build modes:

| Build Mode | Command | Output | Description |
|------------|---------|--------|-------------|
| **Web** | `.\scripts\build.ps1` | `build\ViiB-MediaHub.exe` | Opens in system browser |
| **Wails** | `.\scripts\build-wails.ps1` | `backend\cmd\wails\build\bin\ViiB-MediaHub.exe` | Native WebView2 window |

The Wails build provides a native desktop experience with a WebView2 window instead of opening in the browser.

## Prerequisites

### 1. Go (1.22 or higher)

Download and install from [go.dev/dl](https://go.dev/dl/)

```powershell
# Verify installation
go version
# Expected: go version go1.22.x windows/amd64 (or higher)
```

### 2. Node.js (20 LTS recommended)

Download and install from [nodejs.org](https://nodejs.org/)

```powershell
# Verify installation
node --version
npm --version
```

### 3. MSYS2 and MinGW-w64 (for CGO/SQLite)

ViiB MediaHub uses SQLite which requires CGO. On Windows, this needs a GCC compiler.

1. **Download MSYS2** from [msys2.org](https://www.msys2.org/)
2. **Run the installer** and follow prompts
3. **Open MSYS2 MINGW64** terminal (from Start menu)
4. **Install GCC**:
   ```bash
   pacman -S mingw-w64-x86_64-gcc
   ```
5. **Add to Windows PATH**:
   ```powershell
   # Run as Administrator
   [Environment]::SetEnvironmentVariable(
       "Path",
       $env:Path + ";C:\msys64\mingw64\bin",
       "Machine"
   )
   ```
6. **Restart your terminal** and verify:
   ```powershell
   gcc --version
   ```

### 4. Wails CLI

Install the Wails command-line tool:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Verify installation and system readiness:

```powershell
wails doctor
```

You should see output like:
```
 SUCCESS  Your system is ready for Wails development!
```

### 5. Set CGO_ENABLED

CGO must be enabled for SQLite support:

```powershell
# Set for current session
$env:CGO_ENABLED = "1"

# Or set permanently (run once)
[Environment]::SetEnvironmentVariable("CGO_ENABLED", "1", "User")
```

## Project Setup

Clone the repository and install dependencies:

```powershell
git clone https://github.com/ajbergh/ViiB-MediaHub.git
cd ViiB-MediaHub
npm ci
```

## Development

### Web Development (Browser-based)

For normal development with hot reload in the browser:

```powershell
.\scripts\dev.ps1
```

This starts:
- Vite dev server on port 3000
- Go backend on port 8080

### Wails Development (Native Window)

For testing the native desktop experience:

```powershell
.\scripts\dev-wails.ps1
```

This starts:
- Wails in dev mode with hot reload
- Native WebView2 window
- Go backend rebuilds on file changes

## Building

### Web Build

Produces a single executable that opens in the browser:

```powershell
.\scripts\build.ps1
```

Output: `build\ViiB-MediaHub.exe`

### Wails Build

Produces a native desktop application:

```powershell
# Standard release build
.\scripts\build-wails.ps1

# With debug mode (dev tools enabled)
.\scripts\build-wails.ps1 -Debug

# Clean build (removes previous artifacts)
.\scripts\build-wails.ps1 -Clean

# Skip frontend rebuild (faster if only Go changed)
.\scripts\build-wails.ps1 -SkipFrontend
```

Output: `backend\cmd\wails\build\bin\ViiB-MediaHub.exe`

## Running the Wails Build

```powershell
# Normal mode
.\backend\cmd\wails\build\bin\ViiB-MediaHub.exe

# With debug tools
.\backend\cmd\wails\build\bin\ViiB-MediaHub.exe -debug

# Custom data directory
.\backend\cmd\wails\build\bin\ViiB-MediaHub.exe -data "D:\MyData"
```

## Troubleshooting

### "gcc not found" or CGO errors

Ensure MSYS2's MinGW is in your PATH:

```powershell
$env:Path += ";C:\msys64\mingw64\bin"
gcc --version
```

### Wails build fails with "cannot find package"

Run `go mod tidy` in the backend directory:

```powershell
cd backend
go mod tidy
cd ..
```

### WebView2 not found

WebView2 should be installed on Windows 10/11. If missing, Wails will prompt to install it, or download from [Microsoft](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).

### Frontend not loading in Wails

Ensure the frontend is built and copied:

```powershell
npm run build
# The build-wails.ps1 script handles copying automatically
```

### Blank window or rendering issues

Try disabling GPU acceleration by modifying `main.go`:

```go
Windows: &windows.Options{
    WebviewGpuIsDisabled: true,  // Add this line
}
```

## File Structure

```
backend/
├── cmd/
│   ├── viib/           # Web-embedded build (browser)
│   │   ├── main.go
│   │   └── dist/       # Embedded frontend
│   └── wails/          # Wails build (native window)
│       ├── main.go     # Wails entry point
│       ├── wails.json  # Wails configuration
│       └── frontend/
│           └── dist/   # Frontend for Wails
scripts/
├── build.ps1           # Web build script
├── build-wails.ps1     # Wails build script
├── dev.ps1             # Web dev mode
└── dev-wails.ps1       # Wails dev mode
```

## IDE Setup

### VS Code

Recommended extensions:
- Go (golang.go)
- Wails (AnjaliCode.wails)

### GoLand / IntelliJ

The project should work out of the box. Ensure the Go SDK is configured in Project Settings.

## Additional Resources

- [Wails Documentation](https://wails.io/docs/introduction)
- [Wails v2 Guide](https://wails.io/docs/guides/windows)
- [WebView2 Documentation](https://docs.microsoft.com/en-us/microsoft-edge/webview2/)
- [MSYS2 Documentation](https://www.msys2.org/docs/what-is-msys2/)
