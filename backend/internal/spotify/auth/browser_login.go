// Captures a Spotify session from an isolated visible browser sign-in and cleans up the temporary profile.
package auth

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

var (
	ErrLoginBrowserUnavailable = errors.New("login browser unavailable")
	ErrLoginBrowserClosed      = errors.New("login browser closed")
	ErrLoginProfileCleanup     = errors.New("login profile cleanup failed")
)

// CaptureBrowserSession uses an isolated browser; it never reads passwords or
// page/network content. Only the resulting Spotify session leaves the profile.
func CaptureBrowserSession(ctx context.Context) (cookie string, err error) {
	return captureBrowserSession(ctx, browserCaptureOptions{executable: loginBrowserExecutable(), loginURL: "https://accounts.spotify.com/login?continue=https%3A%2F%2Fopen.spotify.com%2F"})
}

// Test hooks remain backend-only and never accept renderer-supplied URLs,
// executables, profile paths or browser scripts.
type browserCaptureOptions struct {
	executable     string
	loginURL       string
	headless       bool
	ready          func(context.Context) error
	profileCreated func(string)
}

func captureBrowserSession(ctx context.Context, config browserCaptureOptions) (cookie string, err error) {
	executable := config.executable
	if executable == "" {
		return "", ErrLoginBrowserUnavailable
	}
	profile, err := os.MkdirTemp("", "viib-spotify-login-")
	if err != nil {
		return "", ErrLoginBrowserUnavailable
	}
	defer func() {
		if cleanupErr := os.RemoveAll(profile); cleanupErr != nil {
			cookie = ""
			err = ErrLoginProfileCleanup
		}
	}()
	if config.profileCreated != nil {
		config.profileCreated(profile)
	}
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(executable), chromedp.UserDataDir(profile),
		chromedp.Flag("headless", config.headless), chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-extensions", true), chromedp.Flag("disable-sync", true),
		chromedp.Flag("password-store", "basic"), chromedp.Flag("remote-debugging-address", "127.0.0.1"),
	)
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, options...)
	defer stopAllocator() // waits for the process before removing its profile
	browser, stopBrowser := chromedp.NewContext(allocator)
	defer stopBrowser()
	if err := chromedp.Run(browser, chromedp.Navigate(config.loginURL)); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrLoginBrowserClosed
	}
	if config.ready != nil {
		if err := chromedp.Run(browser, chromedp.ActionFunc(config.ready)); err != nil {
			return "", ErrLoginBrowserClosed
		}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var cookies []*network.Cookie
		err := chromedp.Run(browser, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{"https://open.spotify.com/"}).Do(ctx)
			return err
		}))
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", ErrLoginBrowserClosed
		}
		for _, candidate := range cookies {
			if validLoginCookie(candidate) {
				cookie = candidate.Value
			}
			if candidate != nil {
				candidate.Value = ""
			}
		}
		if cookie != "" {
			return cookie, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-browser.Done():
			return "", ErrLoginBrowserClosed
		case <-ticker.C:
		}
	}
}

func validLoginCookie(cookie *network.Cookie) bool {
	if cookie == nil {
		return false
	}
	domain := strings.TrimPrefix(cookie.Domain, ".")
	return cookie.Name == "sp_dc" && cookie.Value != "" && cookie.Secure && cookie.Path == "/" &&
		(domain == "spotify.com" || domain == "open.spotify.com") &&
		(cookie.Session || cookie.Expires > float64(time.Now().Unix()))
}

func loginBrowserExecutable() string {
	userDirectory, _ := os.UserHomeDir()
	return findLoginBrowserExecutable(runtime.GOOS, userDirectory, os.Getenv, exec.LookPath)
}

// All discovery inputs are backend-owned. Renderer requests cannot select an
// executable, installation directory or browser flags.
func findLoginBrowserExecutable(goos, userDirectory string, getenv func(string) string, lookPath func(string) (string, error)) string {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge"}
	switch goos {
	case "windows":
		for _, root := range []string{getenv("PROGRAMFILES"), getenv("PROGRAMFILES(X86)"), getenv("LOCALAPPDATA")} {
			if root != "" {
				candidates = append(candidates,
					filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(root, "Chromium", "Application", "chrome.exe"))
			}
		}
	case "darwin":
		roots := []string{"/Applications"}
		if userDirectory != "" {
			roots = append(roots, path.Join(userDirectory, "Applications"))
		}
		for _, root := range roots {
			candidates = append(candidates, path.Join(root, "Google Chrome.app/Contents/MacOS/Google Chrome"), path.Join(root, "Chromium.app/Contents/MacOS/Chromium"), path.Join(root, "Microsoft Edge.app/Contents/MacOS/Microsoft Edge"))
		}
	case "linux":
		// Standard stable-channel locations also work with a restricted desktop PATH.
		candidates = append(candidates, "/opt/google/chrome/chrome", "/opt/microsoft/msedge/msedge")
	}
	for _, candidate := range candidates {
		if found, err := lookPath(candidate); err == nil {
			return found
		}
	}
	return ""
}
