// Registers Plex configuration, discovery, authentication, library-selection, and
// synchronization HTTP routes.

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

	"github.com/ajbergh/viib-mediahub/internal/crypto"
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
	r.Post("/sync/cancel", a.cancelPlexSync)
	r.Get("/sync/status", a.getPlexSyncStatus)
	r.Put("/sync/schedule", a.setPlexSyncSchedule)
	r.Post("/metadata-writeback/preview", a.previewPlexMetadataWriteback)
	r.Post("/metadata-writeback/sync", a.syncPlexMetadataWriteback)
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
		return plex.Credentials{}, fmt.Errorf("stored Plex credentials are invalid: %w", crypto.ErrReauthRequired)
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
	case errors.Is(err, crypto.ErrReauthRequired):
		status, code, message, retryable = http.StatusUnauthorized, "plex_reauth_required", "Stored Plex credentials could not be decrypted after an application move or key change; please sign in to Plex again", false
	case errors.Is(err, plex.ErrAuthenticationRequired), errors.Is(err, plex.ErrInvalidToken):
		status, code, message, retryable = http.StatusUnauthorized, "plex_auth_required", "Plex authentication is required or has expired", false
	case errors.Is(err, plex.ErrNotPlexServer):
		status, code, message, retryable = http.StatusBadRequest, "not_plex_server", "The configured endpoint is not a Plex Media Server", false
	case errors.Is(err, plex.ErrServerIdentityMismatch):
		code, message, retryable = "plex_server_identity_mismatch", err.Error(), false
	case errors.Is(err, plex.ErrMetadataWriteNotAllowed):
		status, code, message, retryable = http.StatusForbidden, "plex_metadata_write_not_allowed", "This Plex account is not allowed to change library metadata", false
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

func (a *API) plexClientForSource(ctx context.Context, source *db.PlexSource) (*plex.Client, error) {
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		return nil, err
	}
	if token := credentials.ServerTokens[source.MachineIdentifier]; token != "" || credentials.AccountToken == "" {
		return plex.NewClient(source.BaseURL, token, credentials.ClientIdentifier)
	}
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err = a.loadPlexCredentials()
	if err != nil {
		return nil, err
	}
	if token := credentials.ServerTokens[source.MachineIdentifier]; token != "" || credentials.AccountToken == "" {
		return plex.NewClient(source.BaseURL, token, credentials.ClientIdentifier)
	}
	auth := a.newPlexAuthClient()
	previous := credentials.AccountToken
	token, _, resolveErr := auth.ResolveServerRoutes(ctx, &credentials, source.MachineIdentifier)
	if resolveErr == nil && token != "" {
		credentials.ServerTokens[source.MachineIdentifier] = token
		if err := a.savePlexCredentials(credentials); err != nil {
			logger.API("Failed to cache Plex server credentials: %v", err)
		}
		return plex.NewClient(source.BaseURL, token, credentials.ClientIdentifier)
	}
	// Plex JWTs can also be presented as X-Plex-Token to PMS. This fallback
	// keeps a manually configured local server usable if resources discovery is
	// temporarily unavailable.
	if credentials.AccountToken != previous {
		_ = a.savePlexCredentials(credentials)
	}
	return plex.NewClient(source.BaseURL, credentials.AccountToken, credentials.ClientIdentifier)
}

func (a *API) connectPlexServer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL             string `json:"url"`
		ReplaceSourceID string `json:"replaceSourceId"`
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

	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		if errors.Is(err, crypto.ErrReauthRequired) {
			credentials = plex.Credentials{ServerTokens: map[string]string{}}
		} else {
			respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
			return
		}
	}
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	if token := credentials.ServerTokens[server.MachineIdentifier]; token != "" {
		authenticatedClient, clientErr := plex.NewClient(server.URL, token, credentials.ClientIdentifier)
		if clientErr == nil {
			if authenticated, validationErr := authenticatedClient.ValidateServerForMachine(r.Context(), server.MachineIdentifier); validationErr == nil {
				server = authenticated
			}
		}
	}

	if err := a.activatePlexServer(server, &credentials, false, request.ReplaceSourceID); err != nil {
		respondPlexActivationError(w, r, err)
		return
	}
	respondV2JSON(w, http.StatusOK, server)
}

// activatePlexServer persists a validated server as ViiB's single active Plex
// source. The cached media removal is local-only; it never alters PMS data.
var errPlexReplaceConfirmation = errors.New("confirm replacement of the current Plex source")

func respondPlexActivationError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errPlexReplaceConfirmation) {
		respondPlexAPIMessage(w, r, http.StatusConflict, "plex_replace_confirmation_required", "Replacing the Plex server removes its synced tracks from ViiB.", false)
		return
	}
	logger.API("Plex activation failed: %v", plex.RedactError(err))
	respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_save_failed", "ViiB could not save the Plex source configuration.", true)
}

func (a *API) activatePlexServer(server plex.Server, credentials *plex.Credentials, accountManaged bool, approvedSourceID string) error {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	source := db.PlexSource{
		ID: plex.StableSourceID(server.MachineIdentifier), MachineIdentifier: server.MachineIdentifier,
		BaseURL: server.URL, Name: server.Name, Version: server.Version, ConnectedAt: time.Now().UnixMilli(),
		LastSyncStatus: "never", Available: true, Active: true, AccountManaged: accountManaged,
	}
	previous, err := a.db.GetPlexSource(source.ID)
	if err != nil {
		return err
	}
	if previous != nil {
		source.LibraryID, source.LibraryTitle = previous.LibraryID, previous.LibraryTitle
		source.LastSyncAt, source.LastSyncStatus, source.LastSyncError = previous.LastSyncAt, previous.LastSyncStatus, previous.LastSyncError
		source.ConnectedAt = max(source.ConnectedAt, previous.ConnectedAt+1)
	}
	if source.LastSyncStatus == "running" {
		source.LastSyncStatus = "interrupted"
		source.LastSyncError = "The Plex connection changed during sync. Synchronize to try again."
	}
	active, err := a.db.GetActivePlexSource()
	if err != nil {
		return err
	}
	var replacedSourceID string
	if active != nil && active.ID != source.ID {
		if approvedSourceID != active.ID {
			return errPlexReplaceConfirmation
		}
		replacedSourceID = active.ID
		delete(credentials.ServerTokens, active.MachineIdentifier)
	}
	payload, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	if err := a.db.ActivatePlexSource(source, replacedSourceID, string(payload)); err != nil {
		return fmt.Errorf("save Plex server configuration: %w", err)
	}
	a.cancelPlexSyncLocked()
	a.plexLastAttemptSource = ""
	delete(a.plexRecoveryAttempts, source.ID)
	if replacedSourceID != "" && a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex source changed"})
	}
	return nil
}

func (a *API) getPlexConfig(w http.ResponseWriter, r *http.Request) {
	source, err := a.plexSyncSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_load_failed", "Failed to load Plex configuration", true)
		return
	}
	credentials, credErr := a.loadPlexCredentials()
	authenticated := credErr == nil && credentials.AccountToken != ""
	if source != nil && credErr == nil {
		authenticated = authenticated || credentials.ServerTokens[source.MachineIdentifier] != ""
	}
	response := map[string]any{"source": source, "authenticated": authenticated, "autoSync": a.plexAutoSyncEnabled()}
	if errors.Is(credErr, crypto.ErrReauthRequired) {
		response["reauthRequired"] = true
		response["message"] = "Stored Plex credentials could not be decrypted; please reconnect to Plex."
	}
	respondV2JSON(w, http.StatusOK, response)
}

func (a *API) disconnectPlex(w http.ResponseWriter, r *http.Request) {
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_config_load_failed", "Failed to load Plex configuration", true)
		return
	}
	var sourceID string
	if source != nil {
		sourceID = source.ID
	}
	if err := a.db.DisconnectPlexSource(sourceID); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_remove_failed", "Failed to remove Plex source from ViiB", true)
		return
	}
	a.cancelPlexSyncLocked()
	delete(a.plexRecoveryAttempts, sourceID)
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex source removed"})
	}
	respondV2JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (a *API) startPlexAuth(w http.ResponseWriter, r *http.Request) {
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		if errors.Is(err, crypto.ErrReauthRequired) {
			// Starting a fresh PIN sign-in replaces unreadable legacy credentials.
			credentials = plex.Credentials{ServerTokens: map[string]string{}}
		} else {
			respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
			return
		}
	}
	start, err := a.newPlexAuthClient().StartPIN(r.Context(), &credentials)
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
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		if errors.Is(err, crypto.ErrReauthRequired) {
			respondPlexAPIError(w, r, err)
			return
		}
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	hadPendingPIN := credentials.PendingPINID != 0
	auth := a.newPlexAuthClient()
	status, err := auth.PollPIN(r.Context(), &credentials)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if !hadPendingPIN || status.Pending {
		respondV2JSON(w, http.StatusOK, status)
		return
	}
	if status.Authenticated {
		credentials.ServerTokens = make(map[string]string)
		clear(a.plexRecoveryAttempts)
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
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		if errors.Is(err, crypto.ErrReauthRequired) {
			respondPlexAPIError(w, r, err)
			return
		}
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	servers, err := a.newPlexAuthClient().ListServers(r.Context(), &credentials)
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
		ReplaceSourceID   string `json:"replaceSourceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.MachineIdentifier) == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Choose a Plex account server first", false)
		return
	}
	a.plexAuthMu.Lock()
	defer a.plexAuthMu.Unlock()
	credentials, err := a.loadPlexCredentials()
	if err != nil {
		if errors.Is(err, crypto.ErrReauthRequired) {
			respondPlexAPIError(w, r, err)
			return
		}
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_auth_state_failed", "Failed to load Plex authentication state", true)
		return
	}
	machine := strings.TrimSpace(request.MachineIdentifier)
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	token, routes, err := a.newPlexAuthClient().ResolveServerRoutes(ctx, &credentials, machine)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	server, err := validatePlexRoutes(ctx, machine, token, credentials.ClientIdentifier, routes)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if credentials.ServerTokens == nil {
		credentials.ServerTokens = map[string]string{}
	}
	credentials.ServerTokens[server.MachineIdentifier] = token
	if err := a.activatePlexServer(server, &credentials, true, request.ReplaceSourceID); err != nil {
		respondPlexActivationError(w, r, err)
		return
	}
	respondV2JSON(w, http.StatusOK, server)
}

func (a *API) getPlexLibraries(w http.ResponseWriter, r *http.Request) {
	source, err := a.db.GetActivePlexSource()
	if err != nil || source == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Connect a Plex server first", false)
		return
	}
	libraries, err := retryPlexRead(a, r.Context(), source, func(client *plex.Client) ([]plex.Library, error) {
		return client.ListMusicLibrarySections(r.Context())
	})
	if err != nil {
		if isPlexConnectionError(err) {
			a.recordPlexAvailability(r.Context(), source, false)
		}
		respondPlexAPIError(w, r, err)
		return
	}
	if !source.Available {
		a.recordPlexAvailability(r.Context(), source, true)
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
	selected, err := retryPlexRead(a, r.Context(), source, func(client *plex.Client) (plex.Library, error) {
		libraries, err := client.ListMusicLibrarySections(r.Context())
		if err != nil {
			return plex.Library{}, err
		}
		for _, library := range libraries {
			if library.ID == request.LibraryID {
				return client.MusicLibraryDetails(r.Context(), library)
			}
		}
		return plex.Library{}, nil
	})
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if selected.ID == "" {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_plex_music_library", "Selected Plex library is not a music library", false)
		return
	}
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	current, err := a.db.GetActivePlexSource()
	if err != nil || current == nil || current.ID != source.ID || current.ConnectedAt != source.ConnectedAt {
		respondPlexAPIMessage(w, r, http.StatusConflict, "plex_source_changed", "The Plex source changed. Refresh Libraries and try again.", true)
		return
	}
	if err := a.db.SetPlexLibrary(source.ID, selected.ID, selected.Title); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_library_save_failed", "Failed to save Plex music library selection", true)
		return
	}
	if current.LibraryID != selected.ID {
		a.cancelPlexSyncLocked()
		a.plexLastAttemptSource = ""
	}
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex music library selection changed"})
	}
	respondV2JSON(w, http.StatusOK, selected)
}

func (a *API) startPlexSync(w http.ResponseWriter, r *http.Request) {
	run, err := a.beginPlexSync(r.Context(), false)
	switch {
	case errors.Is(err, errPlexSourceRequired):
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_server_required", "Connect a Plex server first", false)
	case errors.Is(err, errPlexLibraryRequired):
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_library_required", "Select a Plex music library before synchronizing", false)
	case errors.Is(err, errPlexSyncRunning):
		respondPlexAPIMessage(w, r, http.StatusConflict, "plex_sync_running", err.Error(), true)
	case err != nil:
		logger.API("Failed to start Plex sync: %v", err)
		respondPlexAPIMessage(w, r, http.StatusServiceUnavailable, "plex_sync_start_failed", "ViiB could not start Plex synchronization. Try again shortly.", true)
	default:
		go a.runPlexSync(run)
		respondV2JSON(w, http.StatusOK, map[string]string{"status": "started"})
	}
}

func (a *API) runPlexSync(run *plexSyncRun) {
	defer a.finishPlexSync(run)
	source, ctx := run.source, run.ctx
	result, err := retryPlexRead(a, ctx, source, func(client *plex.Client) (plex.SyncResult, error) {
		libraries, err := client.ListMusicLibrarySections(ctx)
		if err != nil {
			return plex.SyncResult{}, err
		}
		for _, library := range libraries {
			if library.ID == source.LibraryID {
				return client.FetchTracksWithProgress(ctx, library, func(tracks, total int) {
					run.tracks.Store(int64(tracks))
					run.total.Store(int64(total))
				})
			}
		}
		return plex.SyncResult{}, errors.New("selected Plex music library no longer exists")
	})
	if err != nil {
		a.failPlexSync(run, err, !isPlexConnectionError(err), "")
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
	added, updated, removed, err := a.completePlexSync(run, catalog)
	if err != nil {
		a.failPlexSync(run, err, true, "Plex responded, but ViiB could not save the music library. Try synchronizing again.")
		return
	}
	if err := a.db.UpdateGenreStats(); err != nil {
		logger.API("Failed to rebuild genre stats after Plex sync: %v", err)
	}
	message := fmt.Sprintf("Plex sync complete: %d added, %d updated, %d removed", added, updated, removed)
	if a.scanner != nil {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: message})
	}
	logger.API("%s", message)
}

func isPlexAuthError(err error) bool {
	return errors.Is(err, plex.ErrInvalidToken) || errors.Is(err, plex.ErrAuthenticationRequired) || errors.Is(err, crypto.ErrReauthRequired)
}

func isPlexConnectionError(err error) bool {
	return isPlexAuthError(err) || errors.Is(err, plex.ErrDNSFailure) || errors.Is(err, plex.ErrConnectionFailed) ||
		errors.Is(err, plex.ErrConnectionTimeout) || errors.Is(err, plex.ErrTLSFailure)
}

func (a *API) recordPlexAvailability(ctx context.Context, source *db.PlexSource, available bool) {
	if source.Available == available || ctx.Err() != nil {
		return
	}
	if err := a.db.SetPlexAvailability(ctx, *source, available); err != nil && ctx.Err() == nil {
		logger.API("Failed to update Plex availability for source %s: %v", source.ID, err)
	}
}

func (a *API) failPlexSync(run *plexSyncRun, err error, available bool, message string) {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	if a.plexSync != run || errors.Is(err, context.Canceled) || errors.Is(run.ctx.Err(), context.Canceled) {
		return
	}
	source := run.source
	status := "error"
	if isPlexAuthError(err) {
		status = "auth_required"
	}
	if message == "" {
		_, _, message, _ = classifyPlexError(err)
	}
	health := &available
	if errors.Is(run.ctx.Err(), context.DeadlineExceeded) {
		status, message, health = "error", "Plex synchronization timed out. Try synchronizing again.", nil
	}
	if stateErr := a.db.FailPlexSync(source.ID, run.id, status, message, health); stateErr != nil {
		logger.API("Failed to save Plex sync failure for source %s: %v", source.ID, stateErr)
	}
	logger.API("Plex sync failed for source %s: %v", source.ID, plex.RedactError(err))
}

func (a *API) getPlexSyncStatus(w http.ResponseWriter, r *http.Request) {
	source, err := a.plexSyncSource()
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
	a.proxyPlexSourceAsset(w, r, source, plexMediaInput{key: artwork.ArtworkKey})
}

func (a *API) proxyPlexAsset(w http.ResponseWriter, r *http.Request, track *db.PlexTrackSource, key string, audio bool) {
	if key == "" {
		respondPlexStreamMessage(w, http.StatusNotFound, "Plex media asset is unavailable")
		return
	}
	source, err := a.db.GetPlexSource(track.SourceID)
	if err != nil || source == nil {
		respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex source is not configured")
		return
	}
	a.proxyPlexSourceAsset(w, r, source, plexMediaInput{
		key: key, metadataKey: track.MetadataKey, ratingKey: track.RatingKey, container: track.Container,
		audio: audio, byteRange: r.Header.Get("Range"),
	})
}

type plexMediaInput struct {
	key, metadataKey, ratingKey string
	container                   string
	audio                       bool
	byteRange                   string
}

func (a *API) openPlexMedia(ctx context.Context, source *db.PlexSource, input plexMediaInput) (*http.Response, error) {
	return retryPlexRead(a, ctx, source, func(client *plex.Client) (*http.Response, error) {
		request := func(key string) (*http.Response, error) {
			req, err := client.MediaRequest(ctx, key)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Accept", "*/*")
			if input.audio {
				client.ApplyPlaybackIdentity(req)
				if input.byteRange != "" {
					req.Header.Set("Range", input.byteRange)
				}
			}
			return client.DoMedia(req)
		}
		key := input.key
		response, err := request(key)
		if err != nil || !input.audio {
			return response, err
		}
		if (response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone) && input.ratingKey != "" {
			refreshedKey, err := client.GetTrackMediaKey(ctx, input.ratingKey)
			if isPlexConnectionError(err) {
				response.Body.Close()
				return nil, err
			}
			if err == nil && refreshedKey != "" && refreshedKey != key {
				response.Body.Close()
				key = refreshedKey
				response, err = request(key)
				if err != nil {
					return nil, err
				}
			}
		}
		// Some Plex versions require a playback decision before serving a part.
		if response.StatusCode == http.StatusServiceUnavailable && input.metadataKey != "" {
			response.Body.Close()
			if err := client.PrepareDirectPlay(ctx, input.metadataKey); err != nil {
				return nil, err
			}
			return request(key)
		}
		return response, nil
	})
}

func (a *API) proxyPlexSourceAsset(w http.ResponseWriter, r *http.Request, source *db.PlexSource, input plexMediaInput) {
	resp, err := a.openPlexMedia(r.Context(), source, input)
	if err != nil {
		if input.audio && isPlexConnectionError(err) {
			a.recordPlexAvailability(r.Context(), source, false)
		}
		if isPlexAuthError(err) {
			respondPlexStreamMessage(w, http.StatusUnauthorized, "Plex authentication expired; reconnect in Settings")
			return
		}
		if isPlexConnectionError(err) {
			respondPlexStreamMessage(w, http.StatusServiceUnavailable, "Plex Media Server is unreachable")
			return
		}
		respondPlexStreamError(w, err)
		return
	}
	defer resp.Body.Close()
	// Preserve RFC 7233 range semantics. In particular, a seek beyond the end of
	// the media must remain 416 with PMS's Content-Range (for example bytes */N)
	// rather than being rewritten as a generic upstream 502.
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		copyPlexProxyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		respondPlexStreamMessage(w, http.StatusNotFound, "Plex no longer has this media asset")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respondPlexStreamMessage(w, http.StatusBadGateway, fmt.Sprintf("Plex media request returned HTTP %d", resp.StatusCode))
		return
	}
	copyPlexProxyHeaders(w.Header(), resp.Header)
	if input.audio {
		if w.Header().Get("Accept-Ranges") == "" {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		// WebKit needs an audio MIME type to select its Ogg decoder for Plex
		// streams. PMS can serve these parts as generic binary data.
		if strings.EqualFold(input.container, "ogg") {
			contentType, _, _ := strings.Cut(w.Header().Get("Content-Type"), ";")
			switch strings.ToLower(strings.TrimSpace(contentType)) {
			case "", "application/octet-stream", "application/ogg":
				w.Header().Set("Content-Type", "audio/ogg")
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(w, resp.Body)
	if copyErr != nil {
		if input.audio && r.Context().Err() == nil {
			logger.API("Plex audio transfer interrupted for source %s: %v", source.ID, plex.RedactError(copyErr))
		}
		return
	}
	if input.audio && !source.Available {
		a.recordPlexAvailability(r.Context(), source, true)
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
