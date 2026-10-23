//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsIconFramesDecode(t *testing.T) {
	data := trayIconData
	if len(data) < 6 || binary.LittleEndian.Uint16(data[:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		t.Fatal("invalid ICO header")
	}
	wantSizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count != len(wantSizes) || len(data) < 6+16*count {
		t.Fatalf("invalid icon directory: count=%d bytes=%d", count, len(data))
	}
	end := uint64(6 + 16*count)
	for i, size := range wantSizes {
		entry := data[6+16*i : 6+16*(i+1)]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		length := uint64(binary.LittleEndian.Uint32(entry[8:12]))
		offset := uint64(binary.LittleEndian.Uint32(entry[12:16]))
		if width != size || height != size || binary.LittleEndian.Uint16(entry[6:8]) != 32 || offset != end || length == 0 || offset+length > uint64(len(data)) {
			t.Fatalf("invalid %dpx frame directory entry: %v", size, entry)
		}
		// Decode the full image, not just its header: the previous embedded ICO
		// had a valid directory and PNG header but corrupt IDAT/filter data.
		decoded, err := png.Decode(bytes.NewReader(data[offset : offset+length]))
		if err != nil {
			t.Fatalf("%dpx PNG payload is corrupt: %v", size, err)
		}
		if decoded.Bounds().Dx() != size || decoded.Bounds().Dy() != size {
			t.Fatalf("%dpx payload has bounds %v", size, decoded.Bounds())
		}
		end = offset + length
	}
	if end != uint64(len(data)) {
		t.Fatal("unexpected data after ICO frames")
	}
}

func TestWindowsCanLoadSmallIcons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appicon.ico")
	if err := os.WriteFile(path, trayIconData, 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	user32 := syscall.NewLazyDLL("user32.dll")
	loadImage := user32.NewProc("LoadImageW")
	destroyIcon := user32.NewProc("DestroyIcon")
	for _, size := range []uintptr{0, 16, 20, 24, 32, 40, 48} {
		// Match systray's native loader: IMAGE_ICON + LR_LOADFROMFILE.
		flags := uintptr(0x10)
		if size == 0 {
			flags |= 0x40 // LR_DEFAULTSIZE
		}
		icon, _, loadErr := loadImage.Call(0, uintptr(unsafe.Pointer(name)), 1, size, size, flags)
		if icon == 0 {
			t.Fatalf("Windows could not load %dpx icon: %v", size, loadErr)
		}
		destroyIcon.Call(icon)
	}
}
