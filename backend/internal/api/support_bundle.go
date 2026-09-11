package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/version"
)

const (
	supportBundleSchema = 1
	maxSupportLogBytes  = 4 << 20
)

var (
	supportSecretPattern = regexp.MustCompile(`(?i)((?:authorization|access_?token|refresh_?token|client_?secret|api_?key|password|shared_?secret|code_?verifier|oauth_?state)["']?\s*[:=]\s*["']?)(?:bearer\s+)?[^"'\s,;}\]]+`)
	supportQueryPattern  = regexp.MustCompile(`(?i)([?&](?:code|state|token|access_token|refresh_token|key|api_key|secret|password)=)[^&\s]+`)
	supportWindowsPath   = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n,;)}\]]+`)
	supportUnixPath      = regexp.MustCompile(`/(?:Users|home|Volumes)/[^\r\n,;)}\]]+`)
)

type supportBundleManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	GeneratedAt   string `json:"generatedAt"`
	AppVersion    string `json:"appVersion"`
	GoVersion     string `json:"goVersion"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
}

type supportBundleDiagnostics struct {
	DebugLogging      bool             `json:"debugLogging"`
	Database          supportFileState `json:"database"`
	Log               supportFileState `json:"log"`
	Integrations      map[string]bool  `json:"integrationsConfigured"`
	IncludedLogBytes  int              `json:"includedLogBytes"`
	LogWasSizeLimited bool             `json:"logWasSizeLimited"`
}

type supportFileState struct {
	Exists bool  `json:"exists"`
	Bytes  int64 `json:"bytes,omitempty"`
}

// CreateSupportBundle creates a bounded, privacy-conscious diagnostic archive.
// It intentionally does not include the media database, media files, artwork,
// scan folders, playlists, or credential values.
func (a *API) CreateSupportBundle() ([]byte, string, error) {
	now := time.Now().UTC()
	manifest := supportBundleManifest{
		SchemaVersion: supportBundleSchema,
		GeneratedAt:   now.Format(time.RFC3339),
		AppVersion:    version.Current,
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
	}

	logPath := filepath.Join(a.dataDir, "viib.log")
	databasePath := filepath.Join(a.dataDir, "library.db")
	logData, logLimited, err := readSupportLog(logPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("read support log: %w", err)
	}
	logData = sanitizeSupportLog(logData)

	diagnostics := supportBundleDiagnostics{
		DebugLogging: logger.IsDebug(),
		Database:     supportFileInfo(databasePath),
		Log:          supportFileInfo(logPath),
		Integrations: map[string]bool{
			"spotify":  configuredSetting(a, "spotify_credentials"),
			"lastfm":   configuredSetting(a, "lastfm_settings"),
			"llm":      configuredSetting(a, "llm_settings"),
			"semantic": configuredSetting(a, "semantic_settings"),
		},
		IncludedLogBytes:  len(logData),
		LogWasSizeLimited: logLimited,
	}

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	if err := writeSupportJSON(archive, "manifest.json", manifest); err != nil {
		return nil, "", err
	}
	if err := writeSupportJSON(archive, "diagnostics.json", diagnostics); err != nil {
		return nil, "", err
	}
	if err := writeSupportEntry(archive, "logs/viib.log", logData); err != nil {
		return nil, "", err
	}
	readme := "ViiB MediaHub support bundle\n\nThis archive contains version/platform diagnostics and a size-limited, sanitized application log.\nIt does not contain the media database, media files, artwork, playlists, scan-folder lists, or credential values.\nReview the files before emailing the archive.\n"
	if err := writeSupportEntry(archive, "README.txt", []byte(readme)); err != nil {
		return nil, "", err
	}
	if err := archive.Close(); err != nil {
		return nil, "", fmt.Errorf("close support archive: %w", err)
	}

	filename := "viib-support-" + now.Format("20060102-150405") + ".zip"
	return output.Bytes(), filename, nil
}

func (a *API) downloadSupportBundle(w http.ResponseWriter, _ *http.Request) {
	data, filename, err := a.CreateSupportBundle()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func configuredSetting(a *API, key string) bool {
	if a.db == nil {
		return false
	}
	value, err := a.db.GetSetting(key)
	return err == nil && strings.TrimSpace(value) != ""
}

func supportFileInfo(path string) supportFileState {
	info, err := os.Stat(path)
	if err != nil {
		return supportFileState{}
	}
	return supportFileState{Exists: true, Bytes: info.Size()}
}

func readSupportLog(path string) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	limited := info.Size() > maxSupportLogBytes
	if limited {
		if _, err := file.Seek(-maxSupportLogBytes, io.SeekEnd); err != nil {
			return nil, false, err
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSupportLogBytes))
	if limited {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	return data, limited, err
}

func sanitizeSupportLog(data []byte) []byte {
	text := string(data)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		text = strings.ReplaceAll(text, home, "<home>")
		text = strings.ReplaceAll(text, filepath.ToSlash(home), "<home>")
	}
	text = supportSecretPattern.ReplaceAllString(text, "$1<redacted>")
	text = supportQueryPattern.ReplaceAllString(text, "$1<redacted>")
	text = supportWindowsPath.ReplaceAllString(text, "<local-path>")
	text = supportUnixPath.ReplaceAllString(text, "<local-path>")
	return []byte(text)
}

func writeSupportJSON(archive *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	data = append(data, '\n')
	return writeSupportEntry(archive, name, data)
}

func writeSupportEntry(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
