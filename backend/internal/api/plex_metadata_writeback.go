package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/plex"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
)

const plexMetadataWritebackLimit = 100

type plexMetadataWritebackRequest struct {
	SongIDs      []string `json:"songIds"`
	Confirmation string   `json:"confirmation"`
}

// plexMetadataWritebackField is a visible, reviewable change. Only these
// fields can reach PMS; AI mood/energy/tempo and vectors remain ViiB-local.
type plexMetadataWritebackField struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type plexMetadataWritebackItem struct {
	SongID         string                       `json:"songId"`
	Title          string                       `json:"title"`
	Artist         string                       `json:"artist"`
	Album          string                       `json:"album"`
	ProposedGenres []string                     `json:"proposedGenres,omitempty"`
	ProposedYear   int                          `json:"proposedYear,omitempty"`
	Changes        []plexMetadataWritebackField `json:"changes"`
	Status         string                       `json:"status"`
	RatingKey      string                       `json:"-"`
}

type plexMetadataWritebackPreview struct {
	Confirmation string                      `json:"confirmation"`
	Items        []plexMetadataWritebackItem `json:"items"`
	HasMore      bool                        `json:"hasMore"`
}

type plexMetadataWritebackResult struct {
	Updated  int      `json:"updated"`
	Verified int      `json:"verified"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

func normalizePlexWritebackSongIDs(ids []string) ([]string, error) {
	if len(ids) > plexMetadataWritebackLimit {
		return nil, errors.New("a plex metadata writeback request can contain at most 100 songs")
	}
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 200 {
			return nil, errors.New("plex metadata writeback includes an invalid song id")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func canonicalPlexTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func equalPlexTags(left, right []string) bool {
	left, right = canonicalPlexTags(left), canonicalPlexTags(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func newPlexWritebackItem(candidate db.PlexAIWritebackCandidate, current plex.TrackMetadata) plexMetadataWritebackItem {
	item := plexMetadataWritebackItem{
		SongID: candidate.SongID, Title: candidate.Title, Artist: candidate.Artist, Album: candidate.Album,
		ProposedGenres: candidate.Genres, ProposedYear: candidate.OriginalYear, RatingKey: candidate.RatingKey,
		Changes: make([]plexMetadataWritebackField, 0, 2),
	}
	if len(candidate.Genres) > 0 && !equalPlexTags(current.Genres, candidate.Genres) {
		item.Changes = append(item.Changes, plexMetadataWritebackField{Field: "genres", Before: current.Genres, After: candidate.Genres})
	}
	if candidate.OriginalYear > 0 && current.Year != candidate.OriginalYear {
		item.Changes = append(item.Changes, plexMetadataWritebackField{Field: "year", Before: current.Year, After: candidate.OriginalYear})
	}
	if len(item.Changes) == 0 {
		item.Status = "already_matches"
	} else {
		item.Status = "ready"
	}
	return item
}

func plexWritebackConfirmation(sourceID string, items []plexMetadataWritebackItem) string {
	// The concrete struct and DB ordering make this digest deterministic. It is
	// recomputed immediately before writes, so a changed PMS value invalidates a
	// stale user review instead of silently overwriting it.
	payload := struct {
		SourceID string                      `json:"sourceId"`
		Items    []plexMetadataWritebackItem `json:"items"`
	}{SourceID: sourceID, Items: items}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (a *API) buildPlexMetadataWritebackPreview(r *http.Request, source *db.PlexSource, client *plex.Client, songIDs []string) (plexMetadataWritebackPreview, error) {
	candidates, hasMore, err := a.db.GetPlexAIWritebackCandidates(source.ID, songIDs, plexMetadataWritebackLimit)
	if err != nil {
		return plexMetadataWritebackPreview{}, err
	}
	items := make([]plexMetadataWritebackItem, 0, len(candidates))
	for _, candidate := range candidates {
		current, err := client.GetTrackMetadata(r.Context(), candidate.RatingKey)
		if err != nil {
			return plexMetadataWritebackPreview{}, err
		}
		items = append(items, newPlexWritebackItem(candidate, current))
	}
	return plexMetadataWritebackPreview{
		Confirmation: plexWritebackConfirmation(source.ID, items),
		Items:        items,
		HasMore:      hasMore,
	}, nil
}

func (a *API) activePlexWritebackClient(r *http.Request) (*db.PlexSource, *plex.Client, error) {
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		return nil, nil, err
	}
	if source == nil || source.LibraryID == "" {
		return nil, nil, errors.New("connect and synchronize a Plex music library before writing metadata")
	}
	client, err := a.plexClientForSource(r.Context(), source)
	if err != nil {
		return nil, nil, err
	}
	return source, client, nil
}

// previewPlexMetadataWriteback performs only PMS reads and returns a stable
// confirmation digest; it never changes Plex or ViiB metadata.
func (a *API) previewPlexMetadataWriteback(w http.ResponseWriter, r *http.Request) {
	var request plexMetadataWritebackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_metadata_writeback_request", "Metadata preview request is not valid JSON", false)
		return
	}
	songIDs, err := normalizePlexWritebackSongIDs(request.SongIDs)
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_metadata_writeback_request", err.Error(), false)
		return
	}
	source, client, err := a.activePlexWritebackClient(r)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	preview, err := a.buildPlexMetadataWritebackPreview(r, source, client, songIDs)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	respondV2JSON(w, http.StatusOK, preview)
}

func itemMatchesPlex(item plexMetadataWritebackItem, current plex.TrackMetadata) bool {
	if len(item.ProposedGenres) > 0 && !equalPlexTags(item.ProposedGenres, current.Genres) {
		return false
	}
	return item.ProposedYear == 0 || item.ProposedYear == current.Year
}

// syncPlexMetadataWriteback requires a fresh preview digest, checks the PMS
// management capability, sends the locked edit, then verifies it with a new
// PMS read before recording success in the local audit queue.
func (a *API) syncPlexMetadataWriteback(w http.ResponseWriter, r *http.Request) {
	var request plexMetadataWritebackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_metadata_writeback_request", "Metadata writeback request is not valid JSON", false)
		return
	}
	songIDs, err := normalizePlexWritebackSongIDs(request.SongIDs)
	if err != nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "invalid_metadata_writeback_request", err.Error(), false)
		return
	}
	if strings.TrimSpace(request.Confirmation) == "" {
		respondPlexAPIMessage(w, r, http.StatusConflict, "metadata_preview_required", "Preview and approve the current metadata changes before syncing to Plex", false)
		return
	}
	source, client, err := a.activePlexWritebackClient(r)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	preview, err := a.buildPlexMetadataWritebackPreview(r, source, client, songIDs)
	if err != nil {
		respondPlexAPIError(w, r, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Confirmation), []byte(preview.Confirmation)) != 1 {
		respondPlexAPIMessage(w, r, http.StatusConflict, "metadata_preview_stale", "Plex metadata changed after preview. Review the updated diff before syncing", false)
		return
	}
	if len(preview.Items) == 0 {
		respondV2JSON(w, http.StatusOK, plexMetadataWritebackResult{})
		return
	}
	needsWrite := false
	for _, item := range preview.Items {
		needsWrite = needsWrite || len(item.Changes) > 0
	}
	if needsWrite {
		allowed, capabilityErr := client.CanManageMetadata(r.Context())
		if capabilityErr != nil {
			respondPlexAPIError(w, r, capabilityErr)
			return
		}
		if !allowed {
			respondPlexAPIError(w, r, plex.ErrMetadataWriteNotAllowed)
			return
		}
	}

	now := time.Now().UnixMilli()
	result := plexMetadataWritebackResult{Errors: make([]string, 0)}
	for _, item := range preview.Items {
		if len(item.Changes) == 0 {
			if err := a.db.MarkPlexAIWritebackSynced(item.SongID, now, now); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, item.Title+": failed to record verified metadata")
			} else {
				result.Verified++
			}
			continue
		}
		edit := plex.TrackMetadataEdit{Genres: item.ProposedGenres, Year: item.ProposedYear}
		if err := client.UpdateTrackMetadata(r.Context(), item.RatingKey, edit); err != nil {
			result.Failed++
			_, _, message, _ := classifyPlexError(err)
			_ = a.db.MarkPlexAIWritebackFailed(item.SongID, message)
			result.Errors = append(result.Errors, item.Title+": "+message)
			continue
		}
		current, err := client.GetTrackMetadata(r.Context(), item.RatingKey)
		if err != nil || !itemMatchesPlex(item, current) {
			result.Failed++
			message := "Plex did not preserve the requested metadata"
			if err != nil {
				_, _, message, _ = classifyPlexError(err)
			}
			_ = a.db.MarkPlexAIWritebackFailed(item.SongID, message)
			result.Errors = append(result.Errors, item.Title+": "+message)
			continue
		}
		if err := a.db.MarkPlexAIWritebackSynced(item.SongID, now, now); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, item.Title+": failed to record Plex sync")
			continue
		}
		result.Updated++
	}
	if a.scanner != nil && (result.Updated > 0 || result.Verified > 0) {
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Plex AI metadata writeback completed"})
	}
	respondV2JSON(w, http.StatusOK, result)
}
