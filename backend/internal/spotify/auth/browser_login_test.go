package auth

import (
	"context"
	"errors"
	"github.com/chromedp/cdproto/network"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoginCookieRequiresSpotifyScopeAndLiveSession(t *testing.T) {
	valid := network.Cookie{Name: "sp_dc", Value: "fixture", Domain: ".spotify.com", Path: "/", Secure: true, HTTPOnly: true, Session: true}
	if !validLoginCookie(&valid) {
		t.Fatal("valid HttpOnly cookie rejected")
	}
	for _, change := range []func(*network.Cookie){
		func(c *network.Cookie) { c.Name = "other" }, func(c *network.Cookie) { c.Value = "" },
		func(c *network.Cookie) { c.Domain = "spotify.com.evil.test" }, func(c *network.Cookie) { c.Domain = "accounts.spotify.com" },
		func(c *network.Cookie) { c.Path = "/login" }, func(c *network.Cookie) { c.Secure = false },
		func(c *network.Cookie) { c.Session = false; c.Expires = float64(time.Now().Add(-time.Hour).Unix()) },
	} {
		c := valid
		change(&c)
		if validLoginCookie(&c) {
			t.Fatalf("invalid cookie accepted: %s %s", c.Name, c.Domain)
		}
	}
	if validLoginCookie(nil) {
		t.Fatal("nil accepted")
	}
}

func TestBrowserCaptureHttpOnlyAndProfileCleanup(t *testing.T) {
	if os.Getenv("VIIB_TEST_LOGIN_BROWSER") != "1" {
		t.Skip("opt-in real Chromium fixture")
	}
	executable := loginBrowserExecutable()
	if executable == "" {
		t.Fatal("no installed test browser")
	}
	for _, mode := range []string{"capture", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var profile string
			cookie, err := captureBrowserSession(ctx, browserCaptureOptions{
				executable: executable, headless: true, loginURL: "about:blank",
				profileCreated: func(path string) { profile = path },
				ready: func(ctx context.Context) error {
					if mode == "cancel" {
						cancel()
						return nil
					}
					return network.SetCookie("sp_dc", "fixture-cookie").WithURL("https://open.spotify.com/").WithDomain(".spotify.com").WithPath("/").WithHTTPOnly(true).WithSecure(true).Do(ctx)
				},
			})
			if mode == "capture" && (err != nil || cookie != "fixture-cookie") {
				t.Fatalf("capture failed: %v", err)
			}
			if mode == "cancel" && (cookie != "" || err == nil) {
				t.Fatal("canceled capture returned credential")
			}
			if _, statErr := os.Stat(profile); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("temporary profile remains")
			}
		})
	}
}

func TestLoginBrowserDiscoveryAcrossPlatforms(t *testing.T) {
	for _, tc := range []struct {
		name, goos, userDirectory, available string
		env                                  map[string]string
	}{
		{"windows-installed-edge", "windows", "", filepath.Join("fixture-programs", "Microsoft", "Edge", "Application", "msedge.exe"), map[string]string{"PROGRAMFILES": "fixture-programs"}},
		{"mac-user-chrome", "darwin", "/Users/fixture", "/Users/fixture/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", nil},
		{"mac-user-edge", "darwin", "/Users/fixture", "/Users/fixture/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", nil},
		{"linux-edge-restricted-path", "linux", "/home/fixture", "/opt/microsoft/msedge/msedge", nil},
		{"linux-chrome-restricted-path", "linux", "/home/fixture", "/opt/google/chrome/chrome", nil},
		{"path-chromium", "linux", "", "chromium", nil},
		{"missing", "linux", "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			lookPath := func(candidate string) (string, error) {
				if candidate == tc.available && candidate != "" {
					return candidate, nil
				}
				return "", os.ErrNotExist
			}
			got := findLoginBrowserExecutable(tc.goos, tc.userDirectory, getenv, lookPath)
			if got != tc.available {
				t.Fatalf("browser %q want %q", got, tc.available)
			}
		})
	}
}
func TestLoginBrowserDiscoveryPreservesPathPriority(t *testing.T) {
	got := findLoginBrowserExecutable("darwin", "/Users/fixture", func(string) string { return "" }, func(candidate string) (string, error) {
		if candidate == "chromium" || candidate == "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" {
			return candidate, nil
		}
		return "", os.ErrNotExist
	})
	if got != "chromium" {
		t.Fatalf("changed PATH priority: %s", got)
	}
}
