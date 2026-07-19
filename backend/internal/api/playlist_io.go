package api

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

func normalizePlaylistPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func parseM3U(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	paths := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		paths = append(paths, line)
	}
	return paths
}

func safePlaylistFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "playlist"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\"", "", "*", "", "?", "", "<", "", ">", "", "|", "")
	return replacer.Replace(name)
}

func (a *API) exportPlaylistM3U(w http.ResponseWriter, r *http.Request) {
	playlist, err := a.db.GetPlaylistByID(chi.URLParam(r, "id"))
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "Playlist not found")
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	var output strings.Builder
	output.WriteString("#EXTM3U\n")
	for _, songID := range playlist.SongIDs {
		song, err := a.db.GetSongByID(songID)
		if err != nil {
			continue
		}
		fmt.Fprintf(&output, "#EXTINF:%d,%s - %s\n%s\n", int(song.Duration), song.Artist, song.Title, song.FilePath)
	}
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.m3u8"`, safePlaylistFilename(playlist.Name)))
	_, _ = w.Write([]byte(output.String()))
}

func (a *API) importPlaylistM3U(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid M3U import")
		return
	}
	paths := parseM3U(request.Content)
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	songByPath := make(map[string]string, len(songs))
	for _, song := range songs {
		songByPath[normalizePlaylistPath(song.FilePath)] = song.ID
	}
	songIDs := make([]string, 0, len(paths))
	unmatched := make([]string, 0)
	seen := make(map[string]struct{})
	for _, path := range paths {
		id, found := songByPath[normalizePlaylistPath(path)]
		if !found {
			unmatched = append(unmatched, path)
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		songIDs = append(songIDs, id)
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "Imported Playlist"
	}
	playlist := &db.Playlist{
		ID:        fmt.Sprintf("pl_%d", time.Now().UnixNano()),
		Name:      name,
		SongIDs:   songIDs,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := a.db.SavePlaylist(playlist); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]any{
		"playlist":  playlist,
		"matched":   len(songIDs),
		"unmatched": unmatched,
	})
}
