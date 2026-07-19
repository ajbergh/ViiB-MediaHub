// Package mediafetch safely retrieves remote artwork for local caching.
package mediafetch

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultMaxBytes int64 = 12 << 20

func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid remote address: %w", err)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("remote host could not be resolved")
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, errors.New("private, loopback, or link-local remote hosts are blocked")
		}
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, fmt.Errorf("failed to connect to remote host: %w", lastErr)
}

func validateURL(ctx context.Context, raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("only HTTPS artwork URLs are allowed")
	}
	if parsed.User != nil || parsed.Hostname() == "" {
		return nil, errors.New("invalid artwork host")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, errors.New("artwork host could not be resolved")
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, errors.New("private, loopback, or link-local artwork hosts are blocked")
		}
	}
	return parsed, nil
}

// FetchImage retrieves and validates an image. Redirects are revalidated and
// responses are bounded before being decoded.
func FetchImage(ctx context.Context, raw string, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	parsed, err := validateURL(ctx, raw)
	if err != nil {
		return nil, "", err
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           safeDialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many artwork redirects")
			}
			_, err := validateURL(req.Context(), req.URL.String())
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif;q=0.8")
	req.Header.Set("User-Agent", "ViiB-MediaHub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("artwork request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("artwork server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", errors.New("artwork exceeds maximum size")
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read artwork: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", errors.New("artwork exceeds maximum size")
	}

	detected := http.DetectContentType(data)
	if !strings.HasPrefix(detected, "image/") {
		return nil, "", errors.New("remote content is not an image")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", errors.New("remote content is not a decodable image")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 12000 || cfg.Height > 12000 || int64(cfg.Width)*int64(cfg.Height) > 60_000_000 {
		return nil, "", errors.New("artwork dimensions are invalid or excessive")
	}
	return data, detected, nil
}
