package analysisbench

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// SpotifyCorpusOptions identifies the provenance declaration attached to a
// locally held corpus. EvidenceClass and License are deliberately required:
// importing a CSV must not silently make a legal or provenance claim.
type SpotifyCorpusOptions struct {
	EvidenceClass string
	License       string
	LabelSource   string
}

// CorpusImportReport makes incomplete CSV/file matching visible. A manifest
// contains only unequivocally matched media, so an unfinished local corpus can
// be used for regression measurement without attaching a label to the wrong
// file.
type CorpusImportReport struct {
	Root             string   `json:"root"`
	CSVFiles         []string `json:"csvFiles"`
	MediaFiles       int      `json:"mediaFiles"`
	CSVRows          int      `json:"csvRows"`
	MatchedTracks    int      `json:"matchedTracks"`
	Corpus           CorpusCoverage `json:"corpus"`
	ManifestOutput   string   `json:"manifestOutput,omitempty"`
	UnmatchedMedia   []string `json:"unmatchedMedia,omitempty"`
	UnmatchedCSVRows []string `json:"unmatchedCsvRows,omitempty"`
	AmbiguousMedia   []string `json:"ambiguousMedia,omitempty"`
	UnsupportedMedia []string `json:"unsupportedMedia,omitempty"`
	Manifest         CorpusManifest `json:"-"`
}

type spotifyCSVRow struct {
	Title  string
	Artist string
	BPM    float64
	Key    string
	Path   string
	Line   int
}

// ImportSpotifyCorpus builds a local Phase 0 manifest from a directory tree
// whose media folders each contain a CSV with Title, Artist, BPM, and Key
// columns. It supports only the focused .mp3/.ogg scope. File matching is
// intentionally conservative: title match ambiguity remains in the report.
func ImportSpotifyCorpus(root string, options SpotifyCorpusOptions) (CorpusImportReport, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return CorpusImportReport{}, fmt.Errorf("Spotify corpus root is required")
	}
	root = filepath.Clean(root)
	if options.EvidenceClass != EvidenceSyntheticCI && options.EvidenceClass != EvidenceLawfulRealAudio {
		return CorpusImportReport{}, fmt.Errorf("Spotify corpus evidence class must be %q or %q", EvidenceSyntheticCI, EvidenceLawfulRealAudio)
	}
	if strings.TrimSpace(options.License) == "" || strings.TrimSpace(options.LabelSource) == "" {
		return CorpusImportReport{}, fmt.Errorf("Spotify corpus import requires license and label source")
	}

	report := CorpusImportReport{Root: root, Manifest: CorpusManifest{Version: "phase0-spotify-corpus-v1", EvidenceClass: options.EvidenceClass}}
	csvPaths, err := csvFilesUnder(root)
	if err != nil {
		return CorpusImportReport{}, err
	}
	if len(csvPaths) == 0 {
		return CorpusImportReport{}, fmt.Errorf("no CSV files found under %q", root)
	}
	for _, csvPath := range csvPaths {
		rows, err := readSpotifyCSV(csvPath)
		if err != nil {
			return CorpusImportReport{}, err
		}
		report.CSVFiles = append(report.CSVFiles, csvPath)
		report.CSVRows += len(rows)
		media, unsupported, err := mediaFilesAlongside(csvPath)
		if err != nil {
			return CorpusImportReport{}, err
		}
		report.MediaFiles += len(media)
		report.UnsupportedMedia = append(report.UnsupportedMedia, unsupported...)

		matches, unmatchedMedia, unmatchedRows, ambiguous := matchSpotifyCSVRows(media, rows)
		report.UnmatchedMedia = append(report.UnmatchedMedia, unmatchedMedia...)
		report.UnmatchedCSVRows = append(report.UnmatchedCSVRows, unmatchedRows...)
		report.AmbiguousMedia = append(report.AmbiguousMedia, ambiguous...)
		for mediaPath, row := range matches {
			key, err := spotifyCamelotKey(row.Key)
			if err != nil {
				return CorpusImportReport{}, fmt.Errorf("%s line %d: %w", csvPath, row.Line, err)
			}
			bpm := row.BPM
			relativePath, err := filepath.Rel(root, mediaPath)
			if err != nil {
				return CorpusImportReport{}, fmt.Errorf("relative media path: %w", err)
			}
			track := CorpusTrack{
				ID:                spotifyCorpusID(relativePath),
				Path:              mediaPath,
				License:           strings.TrimSpace(options.License),
				LabelSource:       strings.TrimSpace(options.LabelSource),
				Genre:             filepath.Base(filepath.Dir(mediaPath)),
				Split:             spotifyCorpusSplit(relativePath),
				ExpectedBPM:       &bpm,
				AcceptedMetricBPM: []float64{bpm},
				ExpectedKey:       key,
				Notes:             fmt.Sprintf("Imported from %s line %d: %s — %s.", filepath.Base(csvPath), row.Line, row.Artist, row.Title),
			}
			report.Manifest.Tracks = append(report.Manifest.Tracks, track)
		}
	}
	sort.Strings(report.CSVFiles)
	sort.Strings(report.UnmatchedMedia)
	sort.Strings(report.UnmatchedCSVRows)
	sort.Strings(report.AmbiguousMedia)
	sort.Strings(report.UnsupportedMedia)
	sort.Slice(report.Manifest.Tracks, func(i, j int) bool { return report.Manifest.Tracks[i].ID < report.Manifest.Tracks[j].ID })
	report.MatchedTracks = len(report.Manifest.Tracks)
	if report.MatchedTracks == 0 {
		return CorpusImportReport{}, fmt.Errorf("no CSV rows could be matched to .mp3 or .ogg files under %q", root)
	}
	if err := report.Manifest.Validate(); err != nil {
		return CorpusImportReport{}, fmt.Errorf("build Spotify corpus manifest: %w", err)
	}
	report.Corpus = Coverage(report.Manifest, SplitHeldOut)
	return report, nil
}

// WriteSpotifyCorpusManifest writes a successful import once, retaining the
// unmatched/ambiguous report separately in CLI output for reviewer follow-up.
func WriteSpotifyCorpusManifest(path string, manifest CorpusManifest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("Spotify corpus manifest output path is required")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	return WriteCorpusManifest(path, manifest)
}

func csvFilesUnder(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".csv") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk Spotify corpus %q: %w", root, err)
	}
	sort.Strings(paths)
	return paths, nil
}

func readSpotifyCSV(path string) ([]spotifyCSVRow, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Spotify CSV %q: %w", path, err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read Spotify CSV header %q: %w", path, err)
	}
	columns := make(map[string]int, len(header))
	for index, value := range header {
		columns[strings.ToLower(strings.TrimSpace(value))] = index
	}
	for _, required := range []string{"title", "artist", "bpm", "key"} {
		if _, exists := columns[required]; !exists {
			return nil, fmt.Errorf("Spotify CSV %q has no %q column", path, required)
		}
	}
	var rows []spotifyCSVRow
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read Spotify CSV %q line %d: %w", path, line, err)
		}
		get := func(name string) string {
			index := columns[name]
			if index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		bpm, err := strconv.ParseFloat(get("bpm"), 64)
		if err != nil || bpm <= 0 {
			return nil, fmt.Errorf("Spotify CSV %q line %d has invalid BPM %q", path, line, get("bpm"))
		}
		if get("title") == "" || get("artist") == "" || get("key") == "" {
			return nil, fmt.Errorf("Spotify CSV %q line %d requires title, artist, BPM, and key", path, line)
		}
		rows = append(rows, spotifyCSVRow{Title: get("title"), Artist: get("artist"), BPM: bpm, Key: get("key"), Path: path, Line: line})
	}
	return rows, nil
}

func mediaFilesAlongside(csvPath string) ([]string, []string, error) {
	directory := filepath.Dir(csvPath)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, fmt.Errorf("read Spotify corpus directory %q: %w", directory, err)
	}
	var media, unsupported []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".mp3", ".ogg":
			media = append(media, path)
		case ".oga", ".wav", ".flac", ".aac", ".m4a", ".opus", ".aif", ".aiff", ".wma":
			unsupported = append(unsupported, path)
		}
	}
	sort.Strings(media)
	sort.Strings(unsupported)
	return media, unsupported, nil
}

func matchSpotifyCSVRows(media []string, rows []spotifyCSVRow) (map[string]spotifyCSVRow, []string, []string, []string) {
	matches := make(map[string]spotifyCSVRow)
	matchedRows := make(map[int]struct{})
	var unmatchedMedia, ambiguous []string
	for _, mediaPath := range media {
		filename := normalizeSpotifyTitle(strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath)))
		bestScore := 0
		var candidates []int
		for index, row := range rows {
			score := spotifyTitleMatchScore(filename, normalizeSpotifyTitle(row.Title))
			if score == 0 {
				continue
			}
			if score > bestScore {
				bestScore, candidates = score, []int{index}
			} else if score == bestScore {
				candidates = append(candidates, index)
			}
		}
		if len(candidates) == 0 {
			unmatchedMedia = append(unmatchedMedia, mediaPath)
			continue
		}
		if len(candidates) != 1 {
			ambiguous = append(ambiguous, mediaPath)
			continue
		}
		index := candidates[0]
		if _, alreadyMatched := matchedRows[index]; alreadyMatched {
			ambiguous = append(ambiguous, mediaPath)
			continue
		}
		matchedRows[index] = struct{}{}
		matches[mediaPath] = rows[index]
	}
	var unmatchedRows []string
	for index, row := range rows {
		if _, matched := matchedRows[index]; !matched {
			unmatchedRows = append(unmatchedRows, fmt.Sprintf("%s line %d: %s — %s", row.Path, row.Line, row.Artist, row.Title))
		}
	}
	return matches, unmatchedMedia, unmatchedRows, ambiguous
}

func spotifyTitleMatchScore(filename, title string) int {
	if filename == "" || title == "" || len(title) < 5 {
		return 0
	}
	if filename == title {
		return 3
	}
	if strings.HasSuffix(filename, title) {
		return 2
	}
	if strings.Contains(filename, title) {
		return 1
	}
	return 0
}

func normalizeSpotifyTitle(value string) string {
	value = strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsDigit(r) || r == '-' || r == '_' || unicode.IsSpace(r) })
	var normalized strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func spotifyCorpusID(relativePath string) string {
	return strings.TrimSuffix(filepath.ToSlash(relativePath), filepath.Ext(relativePath))
}

func spotifyCorpusSplit(relativePath string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(filepath.ToSlash(relativePath)))
	if hash.Sum32()%3 == 0 {
		return SplitHeldOut
	}
	return SplitTuning
}

func spotifyCamelotKey(value string) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(value))
	mapping := map[string]string{
		"1A": "G# minor", "2A": "D# minor", "3A": "A# minor", "4A": "F minor", "5A": "C minor", "6A": "G minor",
		"7A": "D minor", "8A": "A minor", "9A": "E minor", "10A": "B minor", "11A": "F# minor", "12A": "C# minor",
		"1B": "B major", "2B": "F# major", "3B": "C# major", "4B": "G# major", "5B": "D# major", "6B": "A# major",
		"7B": "F major", "8B": "C major", "9B": "G major", "10B": "D major", "11B": "A major", "12B": "E major",
	}
	canonical, exists := mapping[key]
	if !exists {
		return "", fmt.Errorf("unsupported Spotify Camelot key %q", value)
	}
	return canonical, nil
}
