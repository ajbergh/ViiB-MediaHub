package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/plex"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
)

// PlexRoutes exposes audio-only Plex source configuration. Authentication
// secrets never leave the backend; all returned structures are sanitized.
func (a *API) PlexRoutes() chi.Router {
	r := chi.NewRouter()
	if err := a.db.EnsurePlexSchema(); err != nil {
		logger.API("Failed to initialize Plex schema: %v", err)
	}
	r.Post("/discover", a.discoverPlexServers)
	r.Post("/connect", a.connectPlexServer)
	r.Get("/config", a.getPlexConfig)
	r.Delete("/config", a.disconnectPlex)
	r.Post("/auth/start", a.startPlexAuth)
	r.Get("/auth/status", a.getPlexAuthStatus)
	r.Get("/servers", a.getPlexAccountServers)
	r.Post("/servers/select", a.connectPlexAccountServer)
	r.Get("/artist-artwork/{name}", a.proxyPlexArtistArtwork)
	r.Get("/libraries", a.getPlexLibraries)
	r.Put("/library", a.selectPlexLibrary)
	r.Post("/sync", a.startPlexSync)
	r.Get("/sync/status", a.getPlexSyncStatus)
	return r
}

func (a *API) loadPlexCredentials() (plex.Credentials, error) {
	raw, err := a.db.GetSetting(db.PlexCredentialsSettingKey)
	if err != nil {
		return plex.Credentials{}, err
	}
	if raw == "" {
		return plex.Credentials{ServerTokens: map[string]string{}}, nil
	}
	var credentials plex.Credentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return plex.Credentials{}, errors.New("stored Plex credentials are invalid")
	}
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	return credentials, nil
}

func (a *API) savePlexCredentials(credentials plex.Credentials) error {
	payload, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	return a.db.SetSetting(db.PlexCredentialsSettingKey, string(payload))
}

func classifyPlexError(err error) (status int, code, message string, retryable bool) {
	status, code, message, retryable = http.StatusBadGateway, "plex_request_failed", "Plex Media Server request failed", true
	// Preserve wrapped sentinel identity for classification. Redaction creates a
	// new error value and therefore must happen only after errors.Is checks.
	switch {
	case errors.Is(err, plex.ErrAuthenticationRequired), errors.Is(err, plex.ErrInvalidToken):
		status, code, message, retryable = http.StatusUnauthorized, "plex_auth_required", "Plex authentication is required or has expired", false
	case errors.Is(err, plex.ErrNotPlexServer):
		status, code, message, retryable = http.StatusBadRequest, "not_plex_server", "The configured endpoint is not a Plex Media Server", false
	case errors.Is(err, plex.ErrDNSFailure):
		code, message = "plex_dns_failed", "Plex server DNS lookup failed"
	case errors.Is(err, plex.ErrConnectionTimeout):
		status, code, message = http.StatusGatewayTimeout, "plex_timeout", "Timed out connecting to Plex Media Server"
	case errors.Is(err, plex.ErrTLSFailure):
		code, message = "plex_tls_failed", "Plex server TLS validation failed; check the certificate and hostname"
	case errors.Is(err, plex.ErrConnectionFailed):
		code, message = "plex_connection_failed", "Could not connect to Plex Media Server; check the address, port, and firewall"
	default:
		if redacted := plex.RedactError(err); redacted != nil && strings.TrimSpace(redacted.Error()) != "" {
			message = strings.TrimSpace(redacted.Error())
		}
	}
	return
}

func respondPlexAPIError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message, retryable := classifyPlexError(err)
	respondV2Error(w, r, status, code, message, retryable, nil)
}

func respondPlexAPIMessage(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool) {
	respondV2Error(w, r, status, code, message, retryable, nil)
}

func respondPlexStreamMessage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func respondPlexStreamError(w http.ResponseWriter, err error) {
	status, _, message, _ := classifyPlexError(err)
	respondPlexStreamMessage(w, status, message)
}

func (a *API) discoverPlexServers(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TimeoutMS int `json:"timeoutMs"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	servers, err := plex.Discover(r.Context(), timeout)
	if err != nil {
		// Discovery failures are non-fatal because manual configuration is a
		// supported first-class fallback.
		respondV2JSON(w, http.StatusOK, map[string]any{"servers": []plex.Server{}, "warning": err.Error()})
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (a *API) tokenForSource(ctx context.Context, source *db.PlexSource) (string, error) {
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		return "", err
	}
	if token := credentials.ServerTokens[source.MachineIdentifier]; token != "" {
		return token, nil
	}
	if credentials.AccountToken == "" {
		return "", nil
	}
	auth := plex.NewAuthClient()
	if err := auth.EnsureFreshToken(ctx, &credentials); err != nil {
		return "", err
	}
	token, preferredURL, resolveErr := auth.ResolveServerToken(ctx, &credentials, source.MachineIdentifier)
	if resolveErr == nil && token != "" {
		if preferredURL != "" {
			if normalized, err := plex.NormalizeServerURL(preferredURL); err == nil {
				_ = a.db.UpdatePlexConnection(source.ID, normalized, source.Name, source.Version, true)
				source.BaseURL = normalized
			}
		}
		_ = a.savePlexCredentials(credentials)
		return token, nil
	}
	// Plex JWTs can also be presented as X-Plex-Token to PMS. This fallback
	// keeps a manually configured local server usable if resources discovery is
	// temporarily unavailable.
	_ = a.savePlexCredentials(credentials)
	return credentials.AccountToken, nil
}

func (a *API) connectPlexServer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.URL) == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_plex_server", "A Plex server URL, hostname, or IP is required", false)
		return
	}
	client, err := plex.NewClient(request.URL, "", "")
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	server, err := client.ValidateServer(r.Context())
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}

	credentials, err := a.loadPlexCredentials()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	if token := credentials.ServerTokens[server.MachineIdentifier]; token != "" {
		authenticatedClient, clientErr := plex.NewClient(server.URL, token, credentials.ClientIdentifier)
		if clientErr == nil {
			if authenticated, validationErr := authenticatedClient.ValidateServer(r.Context()); validationErr == nil {
				server = authenticated
			}
		}
	}

	if err := a.activatePlexServer(server, &credentials); err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if err := a.savePlexCredentials(credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_save_failed", "Plex server was connected but authentication state could not be saved", true)
		return
	}
	respondV2JSON(w, http.StatusOK, server)
}

// activatePlexServer persists a validated server as ViiB's single active Plex
// source. The cached media removal is local-only; it never alters PMS data.
func (a *API) activatePlexServer(server plex.Server, credentials *plex.Credentials) error {
	source := db.PlexSource{
		ID: plex.StableSourceID(server.MachineIdentifier), MachineIdentifier: server.MachineIdentifier,
		BaseURL: server.URL, Name: server.Name, Version: server.Version, ConnectedAt: time.Now().UnixMilli(),
		LastSyncStatus: "never", Available: true, Active: true,
	}
	if previous, _ := a.db.GetPlexSource(source.ID); previous != nil {
		source.LibraryID, source.LibraryTitle = previous.LibraryID, previous.LibraryTitle
		source.LastSyncAt, source.LastSyncStatus, source.LastSyncError = previous.LastSyncAt, previous.LastSyncStatus, previous.LastSyncError
	}
	// The current settings UX manages one active Plex source. Replacing it
	// removes only ViiB's prior catalog/cache and never sends a delete to PMS.
	if active, _ := a.db.GetActivePlexSource(); active != nil && active.ID != source.ID {
		if err := a.db.RemovePlexSource(active.ID); err != nil {
			return fmt.Errorf("replace previous Plex source: %w", err)
		}
		delete(credentials.ServerTokens, active.MachineIdentifier)
	}
	if err := a.db.SavePlexSource(source); err != nil {
		return fmt.Errorf("save Plex server configuration: %w", err)
	}
	return nil
}

func (a *API) getPlexConfig(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_load_failed", "Failed to load Plex configuration", true)
		return
	}
	credentials, _ := a.loadPlexCredentials()
	authenticated := credentials.AccountToken != ""
	if source != nil {
		authenticated = authenticated || credentials.ServerTokens[source.MachineIdentifier] != ""
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"source": source, "authenticated": authenticated})
}

func (a *API) disconnectPlex(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_load_failed", "Failed to load Plex configuration", true)
		return
	}
	if source != nil {
		if err := a.db.RemovePlexSource(source.ID); err != nil {
			respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_remove_failed", "Failed to remove Plex source from ViiB", true)
			return
		}
	}
	if err := a.db.SetSetting(db.PlexCredentialsSettingKey, ""); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_credentials_clear_failed", "Plex source removed but credentials could not be cleared", true)
		return
	}
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex source removed"})
	}
	respondV2JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (a *API) startPlexAuth(w http.ResponseWriter, r *http.Request) {
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	start, err := plex.NewAuthClient().StartPIN(r.Context(), &credentials)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if err := a.savePlexCredentials(credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_save_failed", "Failed to securely save Plex authentication state", true)
		return
	}
	respondV2JSON(w, http.StatusOK, start)
}

func (a *API) getPlexAuthStatus(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_load_failed", "Failed to load Plex configuration", true)
		return
	}
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	status, err := plex.NewAuthClient().PollPIN(r.Context(), &credentials)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if status.Authenticated && source != nil {
		auth := plex.NewAuthClient()
		token, preferredURL, resolveErr := auth.ResolveServerToken(r.Context(), &credentials, source.MachineIdentifier)
		if resolveErr != nil || token == "" {
			token = credentials.AccountToken
		}
		baseURL := source.BaseURL
		if preferredURL != "" {
			if normalized, normalizeErr := plex.NormalizeServerURL(preferredURL); normalizeErr == nil {
				baseURL = normalized
			}
		}
		client, clientErr := plex.NewClient(baseURL, token, credentials.ClientIdentifier)
		if clientErr != nil {
			respondPlexAPIError(w, r, clientErr)
			return
		}
		validated, validationErr := client.ValidateServer(r.Context())
		if validationErr != nil && baseURL != source.BaseURL {
			client, _ = plex.NewClient(source.BaseURL, token, credentials.ClientIdentifier)
			validated, validationErr = client.ValidateServer(r.Context())
		}
		if validationErr != nil {
			respondPlexAPIError(w, r, validationErr)
			return
		}
		credentials.ServerTokens[source.MachineIdentifier] = token
		if err := a.db.UpdatePlexConnection(source.ID, validated.URL, validated.Name, validated.Version, true); err != nil {
			respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_state_update_failed", "Authenticated, but failed to update Plex server state", true)
			return
		}
	}
	if err := a.savePlexCredentials(credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_save_failed", "Failed to securely save Plex authentication state", true)
		return
	}
	respondV2JSON(w, http.StatusOK, status)
}

// getPlexAccountServers queries Plex's resource directory after account sign-in.
// The directory includes both servers owned by the account and servers shared
// with it; access tokens remain encrypted in the backend.
func (a *API) getPlexAccountServers(w http.ResponseWriter, r *http.Request) {
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	servers, err := plex.NewAuthClient().ListServers(r.Context(), &credentials)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if err := a.savePlexCredentials(credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_save_failed", "Plex authentication state could not be saved", true)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"servers": servers})
}

// connectPlexAccountServer validates and activates one PMS returned by the
// signed-in account's resource directory. This is what makes remote and shared
// Plex libraries usable without knowing a LAN address.
func (a *API) connectPlexAccountServer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		MachineIdentifier string `json:"machineIdentifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.MachineIdentifier) == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Choose a Plex account server first", false)
		return
	}
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	auth := plex.NewAuthClient()
	token, preferredURL, err := auth.ResolveAccountServerToken(r.Context(), &credentials, strings.TrimSpace(request.MachineIdentifier))
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if preferredURL == "" {
		respondPlexAPIMessage(w, r, http.StatusBadGateway, "plex_connection_failed", "Plex did not provide a usable connection for this server", true)
		return
	}
	client, err := plex.NewClient(preferredURL, token, credentials.ClientIdentifier)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	server, err := client.ValidateServer(r.Context())
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if server.MachineIdentifier != strings.TrimSpace(request.MachineIdentifier) {
		respondPlexAPIMessage(w, r, http.StatusBadGateway, "plex_server_identity_mismatch", "Plex returned a different server than the one selected", true)
		return
	}
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	credentials.ServerTokens[server.MachineIdentifier] = token
	if err := a.activatePlexServer(server, &credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_save_failed", "Failed to save Plex server configuration", true)
		return
	}
	if err := a.savePlexCredentials(credentials); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_save_failed", "Plex server was connected but authentication state could not be saved", true)
		return
	}
	respondV2JSON(w, http.StatusOK, server)
}

func (a *API) plexClientForSource(ctx context.Context, source *db.PlexSource) (*plex.Client, error) {
	token, err := a.tokenForSource(ctx, source)
	if err != nil {
		return nil, err
	}
	credentials, _ := a.loadPlexCredentials()
	return plex.NewClient(source.BaseURL, token, credentials.ClientIdentifier)
}

func (a *API) getPlexLibraries(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil || source == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Connect a Plex server first", false)
		return
	}
	client, err := a.plexClientForSource(r.Context(), source)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	libraries, err := client.ListMusicLibraries(r.Context())
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"libraries": libraries})
}

func (a *API) selectPlexLibrary(w http.ResponseWriter, r *http.Request) {
	var request struct {
		LibraryID string `json:"libraryId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.LibraryID == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_library_required", "A Plex music library is required", false)
		return
	}
	source, err := a.db.GetActivePlexSource()
	if err != nil || source == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Connect a Plex server first", false)
		return
	}
	client, err := a.plexClientForSource(r.Context(), source)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	libraries, err := client.ListMusicLibraries(r.Context())
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	var selected *plex.Library
	for i := range libraries {
		if libraries[i].ID == request.LibraryID {
			selected = &libraries[i]
			break
		}
	}
	if selected == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_plex_music_library", "Selected Plex library is not a music library", false)
		return
	}
	if err := a.db.SetPlexLibrary(source.ID, selected.ID, selected.Title); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_library_save_failed", "Failed to save Plex music library selection", true)
		return
	}
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex music library selection changed"})
	}
	respondV2JSON(w, http.StatusOK, selected)
}

func (a *API) startPlexSync(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil || source == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Connect a Plex server first", false)
		return
	}
	if source.LibraryID == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_library_required", "Select a Plex music library before synchronizing", false)
		return
	}
	if source.LastSyncStatus == "running" && source.LastSyncAt > 0 && time.Since(time.UnixMilli(source.LastSyncAt)) < 30*time.Minute {
		respondPlexAPIMessage(w, r, http.StatusConflict, "plex_sync_running", "Plex synchronization is already running", true)
		return
	}
	_ = a.db.SetPlexSyncState(source.ID, "running", "", source.Available, time.Now().UnixMilli())
	go a.runPlexSync(source.ID)
	respondV2JSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (a *API) runPlexSync(sourceID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	source, err := a.db.GetPlexSource(sourceID)
	if err != nil || source == nil {
		return
	}
	client, err := a.plexClientForSource(ctx, source)
	if err != nil {
		a.failPlexSync(source, err)
		return
	}
	libraries, err := client.ListMusicLibraries(ctx)
	if err != nil {
		a.failPlexSync(source, err)
		return
	}
	var library *plex.Library
	for i := range libraries {
		if libraries[i].ID == source.LibraryID {
			library = &libraries[i]
			break
		}
	}
	if library == nil {
		a.failPlexSync(source, errors.New("selected Plex music library no longer exists"))
		return
	}
	result, err := client.FetchTracks(ctx, *library)
	if err != nil {
		a.failPlexSync(source, err)
		return
	}

	catalog := make([]db.PlexCatalogTrack, 0, len(result.Tracks))
	for _, track := range result.Tracks {
		catalog = append(catalog, db.PlexCatalogTrack{
			SongID: plex.StableTrackID(source.MachineIdentifier, track.RatingKey), SourceID: source.ID,
			LibraryID: source.LibraryID, MachineID: source.MachineIdentifier, RatingKey: track.RatingKey,
			MetadataKey: track.MetadataKey, MediaKey: track.MediaKey, ArtworkKey: track.ArtworkKey,
			ArtistArtworkKey: track.ArtistArtworkKey,
			Container:        track.Container, AudioCodec: track.AudioCodec, UpdatedAt: track.UpdatedAt,
			Title: track.Title, Artist: track.Artist, Album: track.Album, AlbumArtist: track.AlbumArtist,
			TrackNumber: track.TrackNumber, DiscNumber: track.DiscNumber, Genres: track.Genres,
			Year: track.Year, Duration: track.DurationSeconds, AddedAt: track.AddedAt,
		})
	}
	added, updated, removed, err := a.db.SyncPlexLibrary(source.ID, source.LibraryID, catalog)
	if err != nil {
		a.failPlexSync(source, err)
		return
	}
	if err := a.db.UpdateGenreStats(); err != nil {
		logger.API("Failed to rebuild genre stats after Plex sync: %v", err)
	}
	message := fmt.Sprintf("Plex sync complete: %d added, %d updated, %d removed", added, updated, removed)
	_ = a.db.SetPlexSyncState(source.ID, "complete", "", true, time.Now().UnixMilli())
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: message})
	}
	logger.API("%s", message)
}

func (a *API) failPlexSync(source *db.PlexSource, err error) {
	status := "error"
	if errors.Is(err, plex.ErrInvalidToken) || errors.Is(err, plex.ErrAuthenticationRequired) {
		status = "auth_required"
	}
	redacted := plex.RedactError(err)
	message := "Plex synchronization failed"
	if redacted != nil && strings.TrimSpace(redacted.Error()) != "" {
		message = redacted.Error()
	}
	_ = a.db.SetPlexSyncState(source.ID, status, message, false, 0)
	logger.API("Plex sync failed for source %s: %s", source.ID, message)
}

func (a *API) getPlexSyncStatus(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_sync_status_failed", "Failed to load Plex sync status", true)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"source": source})
}

var proxyHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control", "Content-Disposition"}

func copyPlexProxyHeaders(dst http.Header, src http.Header) {
	for _, header := range proxyHeaders {
		if value := src.Get(header); value != "" {
			dst.Set(header, value)
		}
	}
}

// proxyPlexArtistArtwork serves a PMS artist portrait through ViiB so the
// account/server token never reaches the browser.
func (a *API) proxyPlexArtistArtwork(w http.ResponseWriter, r *http.Request) {
	artistName := strings.TrimSpace(chi.URLParam(r, "name"))
	if artistName == "" {
		respondPlexStreamMessage(w, http.StatusBadRequest, "Plex artist name is required")
		return
	}
	artwork, err := a.db.GetActivePlexArtistArtwork(artistName)
	if err != nil {
		respondPlexStreamMessage(w, http.StatusInternalServerError, "Plex artist artwork is unavailable")
		return
	}
	if artwork == nil || artwork.ArtworkKey == "" {
		respondPlexStreamMessage(w, http.StatusNotFound, "Plex artist artwork is unavailable")
		return
	}
	source, err := a.db.GetPlexSource(artwork.SourceID)
	if err != nil || source == nil {
		respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex source is not configured")
		return
	}
	a.proxyPlexSourceAsset(w, r, source, artwork.ArtworkKey, "", false)
}

func (a *API) proxyPlexAsset(w http.ResponseWriter, r *http.Request, track *db.PlexTrackSource, key string, rangeRequest bool) {
	if key == "" {
		respondPlexStreamMessage(w, http.StatusNotFound, "Plex media asset is unavailable")
		return
	}
	source, err := a.db.GetPlexSource(track.SourceID)
	if err != nil || source == nil {
		respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex source is not configured")
		return
	}
	a.proxyPlexSourceAsset(w, r, source, key, track.MetadataKey, rangeRequest)
}

func plexMediaRequest(r *http.Request, client *plex.Client, key string, rangeRequest bool) (*http.Request, error) {
	upstream, err := client.MediaRequest(r.Context(), key)
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Accept", "*/*")
	if rangeRequest {
		client.ApplyPlaybackIdentity(upstream)
	}
	if rangeRequest && r.Header.Get("Range") != "" {
		upstream.Header.Set("Range", r.Header.Get("Range"))
	}
	return upstream, nil
}

func (a *API) proxyPlexSourceAsset(w http.ResponseWriter, r *http.Request, source *db.PlexSource, key, metadataKey string, rangeRequest bool) {
	client, err := a.plexClientForSource(r.Context(), source)
	if err != nil {
		respondPlexStreamError(w, err)
		return
	}
	upstream, err := plexMediaRequest(r, client, key, rangeRequest)
	if err != nil {
		respondPlexStreamError(w, err)
		return
	}
	resp, err := client.MediaHTTPClient().Do(upstream)
	if err != nil {
		_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, "Plex server unreachable during playback", false, 0)
		respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex Media Server is unreachable")
		return
	}
	// PMS 1.43 can reject a media-part request with 503 when it has no active
	// playback decision. Ask PMS for a direct-play decision once, then repeat
	// the exact original (including Range) request. We intentionally do not
	// enable transcoding: ViiB's player contract remains direct audio playback.
	if resp.StatusCode == http.StatusServiceUnavailable && rangeRequest && metadataKey != "" {
		resp.Body.Close()
		if err := client.PrepareDirectPlay(r.Context(), metadataKey); err != nil {
			respondPlexStreamError(w, err)
			return
		}
		upstream, err = plexMediaRequest(r, client, key, rangeRequest)
		if err != nil {
			respondPlexStreamError(w, err)
			return
		}
		resp, err = client.MediaHTTPClient().Do(upstream)
		if err != nil {
			_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, "Plex server unreachable during playback", false, 0)
			respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex Media Server is unreachable")
			return
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == 498 {
		_ = a.db.SetPlexSyncState(source.ID, "auth_required", "Plex authentication expired", false, 0)
		respondPlexStreamMessage(w, http.StatusUnauthorized, "Plex authentication expired; reconnect in Settings")
		return
	}
	// Preserve RFC 7233 range semantics. In particular, a seek beyond the end of
	// the media must remain 416 with PMS's Content-Range (for example bytes */N)
	// rather than being rewritten as a generic upstream 502.
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		copyPlexProxyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respondPlexStreamMessage(w, http.StatusBadGateway, fmt.Sprintf("Plex media request returned HTTP %d", resp.StatusCode))
		return
	}
	copyPlexProxyHeaders(w.Header(), resp.Header)
	if rangeRequest && w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(w, resp.Body)
	if copyErr != nil {
		// A client cancellation is not evidence that PMS is unavailable. A
		// mid-stream upstream/read failure while the client is still connected is.
		if r.Context().Err() == nil {
			_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, "Plex server connection interrupted during playback", false, 0)
		}
		return
	}
	if !source.Available {
		_ = a.db.SetPlexSyncState(source.ID, source.LastSyncStatus, source.LastSyncError, true, 0)
	}
}

// ServeAudioSourceAware keeps /api/audio/{id} as the one playback contract for
// both local and Plex tracks. Local behavior is delegated unchanged.
func (a *API) ServeAudioSourceAware(w http.ResponseWriter, r *http.Request) {
	songID := strings.TrimPrefix(r.URL.Path, "/api/audio/")
	track, err := a.db.GetPlexTrackSource(songID)
	if err == nil && track != nil {
		a.proxyPlexAsset(w, r, track, track.MediaKey, true)
		return
	}
	a.serveAudio(w, r)
}

// ServeCoverSourceAware similarly keeps the existing /api/cover/{id} URL and
// proxies authenticated Plex artwork without exposing its token to the browser.
func (a *API) ServeCoverSourceAware(w http.ResponseWriter, r *http.Request) {
	songID := strings.TrimPrefix(r.URL.Path, "/api/cover/")
	track, err := a.db.GetPlexTrackSource(songID)
	if err == nil && track != nil && track.ArtworkKey != "" {
		a.proxyPlexAsset(w, r, track, track.ArtworkKey, false)
		return
	}
	a.serveCover(w, r)
}
