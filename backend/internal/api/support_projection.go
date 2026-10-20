// Projects known diagnostic grammars into bounded, typed support records.
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
)

const (
	supportProjectionVersion  = 1
	maxSupportLineBytes       = 8 << 10
	maxSupportRecords         = 2000
	maxSupportProjectionBytes = 256 << 10
)

var (
	supportLineGrammar    = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{6}) \[(FE WARN|API|Analysis)\] (.*)$`)
	supportAudioGrammar   = regexp.MustCompile(`^\[AudioRead\] Audio read failed \| Data: (\{.*\})$`)
	supportCatalogGrammar = regexp.MustCompile(`^Spotify catalog failure: code=(unknown|upstream_http|cancelled|deadline|schema_incompatible|invalid_query|authentication_required|provider_changed|temporarily_unavailable) stage=(catalog|client_token|profile|search|album|artist|playlist|library|track) upstream_status=([0-9]{1,3})$`)
	supportJobIdentity    = `job_id="([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})" mode="(all|missing|stale|ids|playlist)"`
	supportJobStarted     = regexp.MustCompile(`^job started ` + supportJobIdentity + ` tracks=([0-9]{1,7}) algorithm=` + regexp.QuoteMeta(strconv.Quote(track.AlgorithmVersion)) + `$`)
	supportJobCanceled    = regexp.MustCompile(`^job canceled ` + supportJobIdentity + ` processed=([0-9]{1,7}) total=([0-9]{1,7})$`)
	supportJobCompleted   = regexp.MustCompile(`^job completed ` + supportJobIdentity + ` total=([0-9]{1,7}) analyzed=([0-9]{1,7}) skipped=([0-9]{1,7}) failed=([0-9]{1,7})$`)
)

type supportProjection struct {
	Version     int  `json:"projectionVersion"`
	Included    int  `json:"includedRecords"`
	Dropped     int  `json:"droppedRecords"`
	SizeLimited bool `json:"projectionWasSizeLimited"`
}

// Unknown lines never pass through. Counts describe nonempty lines in the read
// tail only; neither these records nor their IDs authenticate the source log.
func projectSupportLog(data []byte) ([]byte, supportProjection) {
	stats := supportProjection{Version: supportProjectionVersion}
	var output bytes.Buffer
	exhausted := false
	for len(data) > 0 {
		var line []byte
		line, data, _ = bytes.Cut(data, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			continue
		}
		if len(line) > maxSupportLineBytes || exhausted {
			stats.SizeLimited = true
			stats.Dropped++
			continue
		}
		record := projectSupportLine(string(line))
		if record == nil {
			stats.Dropped++
			continue
		}
		encoded, err := json.Marshal(record)
		if err != nil { // Only fixed keys, strings and integers are constructed.
			stats.Dropped++
			continue
		}
		if stats.Included >= maxSupportRecords || output.Len()+len(encoded)+1 > maxSupportProjectionBytes {
			stats.Dropped++
			stats.SizeLimited = true
			exhausted = true
			continue
		}
		output.Write(encoded)
		output.WriteByte('\n')
		stats.Included++
	}
	return output.Bytes(), stats
}

func projectSupportLine(line string) map[string]any {
	m := supportLineGrammar.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	if _, err := time.Parse("2006/01/02 15:04:05.000000", m[1]); err != nil {
		return nil
	}
	record := map[string]any{"timestampLocal": m[1]}
	switch m[2] {
	case "FE WARN":
		payload := supportAudioGrammar.FindStringSubmatch(m[3])
		if payload == nil {
			return nil
		}
		var audio struct {
			Operation string `json:"operation"`
			Category  string `json:"category"`
			Status    *int   `json:"status"`
		}
		decoder := json.NewDecoder(strings.NewReader(payload[1]))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&audio) != nil || decoder.Decode(new(any)) != io.EOF {
			return nil
		}
		switch audio.Operation {
		case "audio_metadata", "local_bands", "provider_bands", "deck_analysis":
		default:
			return nil
		}
		switch audio.Category {
		case "http":
			if audio.Status == nil || *audio.Status < 100 || *audio.Status > 599 {
				return nil
			}
		case "invalid_payload", "invalid_json", "type_error", "unknown":
			if audio.Status != nil {
				return nil
			}
		default:
			return nil
		}
		record["event"], record["operation"], record["category"] = "audio_read_failed", audio.Operation, audio.Category
		if audio.Status != nil {
			record["status"] = *audio.Status
		}
	case "API":
		catalog := supportCatalogGrammar.FindStringSubmatch(m[3])
		if catalog == nil {
			return nil
		}
		status, _ := strconv.Atoi(catalog[3])
		if status != 0 && (status < 100 || status > 599) {
			return nil
		}
		record["event"], record["code"], record["stage"], record["status"] = "spotify_catalog_failed", catalog[1], catalog[2], status
	case "Analysis":
		var job []string
		var keys []string
		switch {
		case supportJobStarted.MatchString(m[3]):
			job, keys = supportJobStarted.FindStringSubmatch(m[3]), []string{"tracks"}
			record["event"] = "analysis_started"
		case supportJobCanceled.MatchString(m[3]):
			job, keys = supportJobCanceled.FindStringSubmatch(m[3]), []string{"processed", "total"}
			record["event"] = "analysis_canceled"
		case supportJobCompleted.MatchString(m[3]):
			job, keys = supportJobCompleted.FindStringSubmatch(m[3]), []string{"total", "analyzed", "skipped", "failed"}
			record["event"] = "analysis_completed"
		default:
			return nil
		}
		record["jobId"], record["mode"] = job[1], job[2]
		counts := map[string]int{}
		for i, key := range keys {
			value, err := strconv.Atoi(job[i+3])
			if err != nil || value > 1_000_000 {
				return nil
			}
			counts[key] = value
		}
		record["counts"] = counts
	}
	return record
}
