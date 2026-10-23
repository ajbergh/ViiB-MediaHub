# Windows application icons

`appicon.png` is the canonical artwork. The tracked
`backend/cmd/wails/appicon.ico` contains 32-bit frames at 16, 20, 24, 32, 40, 48,
64, 128, and 256 pixels for Windows taskbar, notification-area, and DPI scaling.
The Windows tray embeds this ICO directly; the installer uses it too.

After changing the artwork, regenerate the ICO on Windows and commit it:

```powershell
./scripts/generate-windows-icon.ps1
```

Both Windows build scripts and Windows CI/release builds run
`scripts/prepare-windows-icons.ps1`. This copies the canonical PNG and tracked
ICO into Wails' generated build directory. It overwrites `build/windows/icon.ico`
because Wails otherwise reuses that file, even after the artwork changes.

Validate every PNG payload and the native Windows small-icon loader with:

```powershell
cd backend
go test ./cmd/wails -run 'TestWindowsIconFramesDecode|TestWindowsCanLoadSmallIcons'
```

The full PNG decode is deliberate: valid ICO and PNG headers alone did not detect
the corrupt image data in the previous inline base64 tray icon.
