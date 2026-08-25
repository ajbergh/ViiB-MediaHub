package plex

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAuthenticationRequired = errors.New("plex authentication required")
	ErrInvalidToken           = errors.New("plex authentication token is invalid or expired")
	ErrNotPlexServer          = errors.New("endpoint is not a Plex Media Server")
	ErrDNSFailure             = errors.New("plex server DNS lookup failed")
	ErrConnectionFailed       = errors.New("plex server connection failed")
	ErrConnectionTimeout      = errors.New("plex server connection timed out")
	ErrTLSFailure             = errors.New("plex server TLS validation failed")
)

// Client communicates with one PMS. Tokens are sent only as X-Plex-Token
// headers and are never added to URLs.
type Client struct {
	baseURL          *url.URL
	token            string
	clientIdentifier string
	httpClient       *http.Client
}

func NewClient(rawBaseURL, token, clientIdentifier string) (*Client, error) {
	normalized, err := NormalizeServerURL(rawBaseURL)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse Plex server URL: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	return &Client{
		baseURL:          parsed,
		token:            token,
		clientIdentifier: clientIdentifier,
		httpClient:       &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

// NewClientWithHTTP is intended for tests and dependency injection.
func NewClientWithHTTP(rawBaseURL, token, clientIdentifier string, httpClient *http.Client) (*Client, error) {
	client, err := NewClient(rawBaseURL, token, clientIdentifier)
	if err != nil {
		return nil, err
	}
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client, nil
}

func (c *Client) BaseURL() string { return strings.TrimRight(c.baseURL.String(), "/") }

// NormalizeServerURL accepts either a bare host/IP (defaulting to PMS 32400) or
// a complete HTTP(S) URL. Complete URLs are authoritative so reverse proxies on
// normal scheme ports remain usable.
func NormalizeServerURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("plex server address is required")
	}
	explicitURL := strings.Contains(input, "://")
	if !explicitURL {
		input = "http://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid Plex server address: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("plex server URL must use http or https")
	}
	if u.User != nil {
		return "", errors.New("plex server URL must not contain credentials")
	}
	if u.Hostname() == "" {
		return "", errors.New("plex server hostname or IP is required")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("plex server URL must not contain a query or fragment")
	}
	port := u.Port()
	if port == "" && !explicitURL {
		port = strconv.Itoa(DefaultPort)
		u.Host = net.JoinHostPort(u.Hostname(), port)
	} else if port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", errors.New("plex server port is invalid")
		}
		u.Host = net.JoinHostPort(u.Hostname(), port)
	}
	if u.Path == "/" {
		u.Path = ""
	} else if u.Path != "" {
		u.Path = "/" + strings.Trim(strings.ReplaceAll(path.Clean(u.Path), "\\", "/"), "/")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) resolveKey(parent, key string) (*url.URL, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("empty Plex API key")
	}
	candidate, err := url.Parse(key)
	if err != nil {
		return nil, fmt.Errorf("invalid Plex API key: %w", err)
	}
	if candidate.IsAbs() {
		if candidate.Scheme != "http" && candidate.Scheme != "https" {
			return nil, fmt.Errorf("unsupported Plex API key scheme %q", candidate.Scheme)
		}
		return candidate, nil
	}
	base := *c.baseURL
	prefix := strings.TrimRight(base.Path, "/")
	if strings.HasPrefix(key, "/") {
		base.Path = prefix + candidate.Path
		base.RawQuery = candidate.RawQuery
		return &base, nil
	}
	if parent == "" {
		parent = "/"
	}
	parentURL, err := url.Parse(parent)
	if err != nil {
		return nil, err
	}
	if parentURL.IsAbs() {
		base = *parentURL
	} else {
		base.Path = prefix + "/" + strings.TrimPrefix(parentURL.Path, "/")
		base.RawQuery = parentURL.RawQuery
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	return base.ResolveReference(candidate), nil
}

func (c *Client) sameServer(u *url.URL) bool {
	return strings.EqualFold(u.Scheme, c.baseURL.Scheme) && strings.EqualFold(u.Host, c.baseURL.Host)
}

func (c *Client) newRequest(ctx context.Context, method, parent, key string, authenticated bool) (*http.Request, error) {
	u, err := c.resolveKey(parent, key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", ProductName)
	req.Header.Set("X-Plex-Version", "1")
	req.Header.Set("X-Plex-Pms-Api-Version", PMSAPIVersion)
	if c.clientIdentifier != "" {
		req.Header.Set("X-Plex-Client-Identifier", c.clientIdentifier)
	}
	if authenticated && c.token != "" && c.sameServer(u) {
		req.Header.Set("X-Plex-Token", c.token)
	}
	return req, nil
}

func classifyHTTPError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, 498:
		return ErrInvalidToken
	default:
		return fmt.Errorf("plex server returned HTTP %d", resp.StatusCode)
	}
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var dnsErr *net.DNSError
		var netErr net.Error
		var tlsErr *tls.CertificateVerificationError
		switch {
		case errors.As(err, &dnsErr):
			return fmt.Errorf("%w: %v", ErrDNSFailure, dnsErr)
		case errors.As(err, &tlsErr):
			return fmt.Errorf("%w: %v", ErrTLSFailure, tlsErr)
		case errors.As(err, &netErr) && netErr.Timeout():
			return fmt.Errorf("%w: %v", ErrConnectionTimeout, netErr)
		default:
			return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return classifyHTTPError(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid Plex server response: %w", err)
	}
	return nil
}

type identityEnvelope struct {
	MediaContainer struct {
		Claimed           bool   `json:"claimed"`
		MachineIdentifier string `json:"machineIdentifier"`
		Version           string `json:"version"`
	} `json:"MediaContainer"`
}

type rootEnvelope struct {
	MediaContainer struct {
		FriendlyName      string `json:"friendlyName"`
		MachineIdentifier string `json:"machineIdentifier"`
		Version           string `json:"version"`
		Claimed           bool   `json:"claimed"`
	} `json:"MediaContainer"`
}

// ValidateServer uses the documented unauthenticated /identity endpoint to
// prove that the address is a PMS, then optionally enriches the result via root.
func (c *Client) ValidateServer(ctx context.Context) (Server, error) {
	var identity identityEnvelope
	req, err := c.newRequest(ctx, http.MethodGet, "", "/identity", false)
	if err != nil {
		return Server{}, err
	}
	if err := c.doJSON(req, &identity); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return Server{}, ErrAuthenticationRequired
		}
		if errors.Is(err, ErrDNSFailure) || errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrConnectionTimeout) || errors.Is(err, ErrTLSFailure) {
			return Server{}, err
		}
		return Server{}, fmt.Errorf("%w: %v", ErrNotPlexServer, err)
	}
	if strings.TrimSpace(identity.MediaContainer.MachineIdentifier) == "" {
		return Server{}, ErrNotPlexServer
	}
	port, _ := strconv.Atoi(c.baseURL.Port())
	if port == 0 {
		if c.baseURL.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}
	server := Server{
		Host:              c.baseURL.Hostname(),
		Port:              port,
		Scheme:            c.baseURL.Scheme,
		URL:               c.BaseURL(),
		MachineIdentifier: identity.MediaContainer.MachineIdentifier,
		Version:           identity.MediaContainer.Version,
		Claimed:           identity.MediaContainer.Claimed,
		AuthRequired:      identity.MediaContainer.Claimed && c.token == "",
	}
	if c.token == "" {
		server.Name = server.Host
		return server, nil
	}
	var root rootEnvelope
	rootReq, err := c.newRequest(ctx, http.MethodGet, "", "/", true)
	if err != nil {
		return Server{}, err
	}
	if err := c.doJSON(rootReq, &root); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			server.AuthRequired = true
			return server, ErrInvalidToken
		}
		return server, err
	}
	if root.MediaContainer.MachineIdentifier != "" && root.MediaContainer.MachineIdentifier != server.MachineIdentifier {
		return Server{}, errors.New("plex server identity changed during validation")
	}
	server.Name = root.MediaContainer.FriendlyName
	if server.Name == "" {
		server.Name = server.Host
	}
	if root.MediaContainer.Version != "" {
		server.Version = root.MediaContainer.Version
	}
	server.Claimed = root.MediaContainer.Claimed
	server.AuthRequired = false
	return server, nil
}

type contentDirectory struct {
	Key       string `json:"key"`
	HubKey    string `json:"hubKey"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Agent     string `json:"agent"`
	UUID      string `json:"uuid"`
	ID        any    `json:"id"`
	SectionID any    `json:"librarySectionID"`
}

type providerFeature struct {
	Type      string             `json:"type"`
	Key       string             `json:"key"`
	Directory []contentDirectory `json:"Directory"`
}

type provider struct {
	Identifier string            `json:"identifier"`
	Title      string            `json:"title"`
	Feature    []providerFeature `json:"Feature"`
}

type providersEnvelope struct {
	MediaContainer struct {
		MediaProvider []provider `json:"MediaProvider"`
	} `json:"MediaContainer"`
}

type sectionPivot struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type sectionEnvelope struct {
	MediaContainer struct {
		Directory []sectionPivot `json:"Directory"`
		Type      []sectionPivot `json:"Type"`
	} `json:"MediaContainer"`
}

func interfaceString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatInt(int64(value), 10)
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func isMusicLibraryType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "artist" || value == "music" || value == "audio"
}

// ListMusicLibraries follows the documented /media/providers content feature
// and returns only music sections. It intentionally ignores video/clip pivots.
func (c *Client) ListMusicLibraries(ctx context.Context) ([]Library, error) {
	var response providersEnvelope
	req, err := c.newRequest(ctx, http.MethodGet, "", "/media/providers", true)
	if err != nil {
		return nil, err
	}
	if err := c.doJSON(req, &response); err != nil {
		return nil, err
	}
	libraries := make([]Library, 0)
	for _, p := range response.MediaContainer.MediaProvider {
		if p.Identifier != "com.plexapp.plugins.library" {
			continue
		}
		for _, feature := range p.Feature {
			if feature.Type != "content" {
				continue
			}
			for _, directory := range feature.Directory {
				if !isMusicLibraryType(directory.Type) || directory.Key == "" {
					continue
				}
				id := interfaceString(directory.ID)
				if id == "" {
					id = interfaceString(directory.SectionID)
				}
				if id == "" {
					id = strings.Trim(strings.TrimPrefix(directory.Key, feature.Key), "/")
					if slash := strings.IndexByte(id, '/'); slash >= 0 {
						id = id[:slash]
					}
				}
				library := Library{ID: id, Title: directory.Title, Type: directory.Type, ContentKey: directory.Key}
				trackKey, err := c.findTrackKey(ctx, library.ContentKey)
				if err != nil {
					return nil, fmt.Errorf("inspect Plex music library %q: %w", library.Title, err)
				}
				library.TrackKey = trackKey
				libraries = append(libraries, library)
			}
		}
	}
	return libraries, nil
}

func (c *Client) findTrackKey(ctx context.Context, sectionKey string) (string, error) {
	u, err := c.resolveKey("", sectionKey)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("includeDetails", "1")
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Pms-Api-Version", PMSAPIVersion)
	req.Header.Set("X-Plex-Product", ProductName)
	if c.clientIdentifier != "" {
		req.Header.Set("X-Plex-Client-Identifier", c.clientIdentifier)
	}
	if c.token != "" && c.sameServer(u) {
		req.Header.Set("X-Plex-Token", c.token)
	}
	var response sectionEnvelope
	if err := c.doJSON(req, &response); err != nil {
		return "", err
	}
	for _, pivots := range [][]sectionPivot{response.MediaContainer.Type, response.MediaContainer.Directory} {
		for _, pivot := range pivots {
			if strings.EqualFold(pivot.Type, "track") && pivot.Key != "" {
				resolved, err := c.resolveKey(sectionKey, pivot.Key)
				if err != nil {
					return "", err
				}
				return resolved.String(), nil
			}
		}
	}
	return "", errors.New("documented track type key not found")
}

type plexTag struct{ Tag string `json:"tag"` }
type plexPart struct {
	Key       string `json:"key"`
	Container string `json:"container"`
}
type plexMedia struct {
	Container  string     `json:"container"`
	AudioCodec string     `json:"audioCodec"`
	Part       []plexPart `json:"Part"`
}
type plexTrack struct {
	RatingKey        string      `json:"ratingKey"`
	Key              string      `json:"key"`
	Title            string      `json:"title"`
	ParentTitle      string      `json:"parentTitle"`
	GrandparentTitle string      `json:"grandparentTitle"`
	OriginalTitle    string      `json:"originalTitle"`
	Index            int         `json:"index"`
	ParentIndex      int         `json:"parentIndex"`
	Year             int         `json:"year"`
	Duration         int64       `json:"duration"`
	Thumb            string      `json:"thumb"`
	ParentThumb      string      `json:"parentThumb"`
	AddedAt          int64       `json:"addedAt"`
	UpdatedAt        int64       `json:"updatedAt"`
	Genre            []plexTag   `json:"Genre"`
	Media            []plexMedia `json:"Media"`
	Type             string      `json:"type"`
}
type trackEnvelope struct {
	MediaContainer struct {
		Size      int         `json:"size"`
		TotalSize int         `json:"totalSize"`
		Offset    int         `json:"offset"`
		Metadata  []plexTrack `json:"Metadata"`
	} `json:"MediaContainer"`
}

func mapPlexTrack(raw plexTrack) (Track, bool) {
	if raw.Type != "" && !strings.EqualFold(raw.Type, "track") {
		return Track{}, false
	}
	if raw.RatingKey == "" || raw.Title == "" || len(raw.Media) == 0 || len(raw.Media[0].Part) == 0 || raw.Media[0].Part[0].Key == "" {
		return Track{}, false
	}
	genres := make([]string, 0, len(raw.Genre))
	for _, genre := range raw.Genre {
		if value := strings.TrimSpace(genre.Tag); value != "" {
			genres = append(genres, value)
		}
	}
	artwork := raw.Thumb
	if artwork == "" {
		artwork = raw.ParentThumb
	}
	albumArtist := raw.GrandparentTitle
	if albumArtist == "" {
		albumArtist = raw.OriginalTitle
	}
	return Track{
		RatingKey:       raw.RatingKey,
		MetadataKey:     raw.Key,
		Title:           raw.Title,
		Artist:          raw.GrandparentTitle,
		Album:           raw.ParentTitle,
		AlbumArtist:     albumArtist,
		TrackNumber:     raw.Index,
		DiscNumber:      raw.ParentIndex,
		Genres:          genres,
		Year:            raw.Year,
		DurationSeconds: float64(raw.Duration) / 1000,
		ArtworkKey:      artwork,
		MediaKey:        raw.Media[0].Part[0].Key,
		Container:       firstNonEmpty(raw.Media[0].Part[0].Container, raw.Media[0].Container),
		AudioCodec:      raw.Media[0].AudioCodec,
		AddedAt:         raw.AddedAt * 1000,
		UpdatedAt:       raw.UpdatedAt * 1000,
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// FetchTracks pages through the section's documented track type key. A result
// is authoritative only if every page succeeds; callers must not delete cached
// tracks when this method returns an error.
func (c *Client) FetchTracks(ctx context.Context, library Library) (SyncResult, error) {
	started := time.Now()
	if library.TrackKey == "" {
		trackKey, err := c.findTrackKey(ctx, library.ContentKey)
		if err != nil {
			return SyncResult{}, err
		}
		library.TrackKey = trackKey
	}
	const pageSize = 500
	tracks := make([]Track, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		req, err := c.newRequest(ctx, http.MethodGet, "", library.TrackKey, true)
		if err != nil {
			return SyncResult{}, err
		}
		req.Header.Set("X-Plex-Container-Start", strconv.Itoa(offset))
		req.Header.Set("X-Plex-Container-Size", strconv.Itoa(pageSize))
		var response trackEnvelope
		if err := c.doJSON(req, &response); err != nil {
			return SyncResult{}, fmt.Errorf("read Plex music library page at offset %d: %w", offset, err)
		}
		for _, raw := range response.MediaContainer.Metadata {
			if track, ok := mapPlexTrack(raw); ok {
				tracks = append(tracks, track)
			}
		}
		count := len(response.MediaContainer.Metadata)
		if count == 0 || count < pageSize || (response.MediaContainer.TotalSize > 0 && offset+count >= response.MediaContainer.TotalSize) {
			break
		}
	}
	return SyncResult{Tracks: tracks, Started: started, Finished: time.Now()}, nil
}

// MediaRequest builds a same-origin PMS request for a media/artwork key. Tokens
// are kept in headers and never put in browser-visible URLs.
func (c *Client) MediaRequest(ctx context.Context, key string) (*http.Request, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "", key, true)
	if err != nil {
		return nil, err
	}
	if !c.sameServer(req.URL) {
		req.Header.Del("X-Plex-Token")
	}
	return req, nil
}

// MediaHTTPClient has no whole-response timeout so long audio streams are not
// interrupted; connection/header timeouts remain enforced by the transport.
func (c *Client) MediaHTTPClient() *http.Client {
	clone := *c.httpClient
	clone.Timeout = 0
	return &clone
}
