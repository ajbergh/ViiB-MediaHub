package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
)

const supportFixtureJob = `job_id="12345678-1234-4123-8123-123456789abc" mode="missing"`

func supportFixtureLine(category, message string) string {
	return "2026/10/20 12:00:00.000000 [" + category + "] " + message + "\n"
}

func TestSupportProjectionRetainsOnlyTypedDiagnosticRecords(t *testing.T) {
	input := supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"deck_analysis","category":"http","status":503}`) +
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"invalid_payload"}`) +
		supportFixtureLine("API", `Spotify catalog failure: code=upstream_http stage=playlist upstream_status=429`) +
		supportFixtureLine("Analysis", `job started `+supportFixtureJob+` tracks=2 algorithm=`+strconv.Quote(track.AlgorithmVersion)) +
		supportFixtureLine("Analysis", `job canceled `+supportFixtureJob+` processed=1 total=2`) +
		supportFixtureLine("Analysis", `job completed `+supportFixtureJob+` total=2 analyzed=1 skipped=0 failed=1`)
	data, stats := projectSupportLog([]byte(input))
	if stats.Included != 6 || stats.Dropped != 0 || stats.SizeLimited {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
		for key := range record {
			switch key {
			case "timestampLocal", "event", "operation", "category", "status", "code", "stage", "jobId", "mode", "counts":
			default:
				t.Fatalf("unreviewed projected key: %s", key)
			}
		}
	}
	if records[0]["status"] != float64(503) || records[2]["code"] != "upstream_http" || records[5]["jobId"] != "12345678-1234-4123-8123-123456789abc" {
		t.Fatal("classification/status/job correlation lost")
	}
	if strings.Contains(string(data), "algorithm") || strings.Contains(string(data), "requestId") {
		t.Fatal("unapproved identity/algorithm fields exported")
	}
}

func TestSupportProjectionDropsPrivateAndUnknownPayloads(t *testing.T) {
	for _, input := range []string{
		supportFixtureLine("FE ERROR", `[DJEngine] Private song title | Data: {"id":"private-track-id","message":"private error","stack":"/home/alice/music"}`),
		supportFixtureLine("API", `requestId=alice-hostname-123 token=private-cookie /private/music/title.mp3`),
		supportFixtureLine("Analysis", `track failed song_id="private-track-id" path="/private/song.mp3" error="private error"`),
		supportFixtureLine("Analysis", `job failed `+supportFixtureJob+` processed=1 total=2 error="private error"`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"http","status":503,"title":"Private song title"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"http","status":503,"requestId":"alice-hostname-123"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"private-id","category":"unknown"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"private error"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"http","status":999}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"http"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"unknown","status":503}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"unknown"} {"title":"private"}`),
		supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: malformed private body`),
		supportFixtureLine("API", `Spotify catalog failure: code=upstream_http stage=track upstream_status=503 private-title`),
		supportFixtureLine("API", `Spotify catalog failure: code=private-error stage=track upstream_status=503`),
		supportFixtureLine("API", `Spotify catalog failure: code=unknown stage=private-stage upstream_status=0`),
		supportFixtureLine("API", `Spotify catalog failure: code=unknown stage=track upstream_status=99`),
		supportFixtureLine("Analysis", `job completed job_id="private-track-id" mode="missing" total=1 analyzed=1 skipped=0 failed=0`),
		supportFixtureLine("Analysis", `job started `+supportFixtureJob+` tracks=2 algorithm="private-string"`),
		supportFixtureLine("Analysis", `job canceled `+supportFixtureJob+` processed=1000001 total=1000001`),
		"2026/99/20 12:00:00.000000 [API] Spotify catalog failure: code=unknown stage=track upstream_status=0\n",
	} {
		data, stats := projectSupportLog([]byte(input))
		if len(data) != 0 || stats.Dropped != 1 || stats.Included != 0 {
			t.Fatalf("private/unknown payload passed projection: %q %+v", data, stats)
		}
	}
}

func TestSupportProjectionBoundsLinesRecordsAndOutput(t *testing.T) {
	data, stats := projectSupportLog([]byte(strings.Repeat("private", maxSupportLineBytes)))
	if len(data) != 0 || stats.Dropped != 1 || !stats.SizeLimited {
		t.Fatal("giant line not bounded")
	}
	short := supportFixtureLine("API", "Spotify catalog failure: code=unknown stage=track upstream_status=0")
	data, stats = projectSupportLog([]byte(strings.Repeat(short, maxSupportRecords+5)))
	if stats.Included != maxSupportRecords || stats.Dropped != 5 || !stats.SizeLimited || len(data) > maxSupportProjectionBytes {
		t.Fatalf("record bound: bytes=%d stats=%+v", len(data), stats)
	}
	long := supportFixtureLine("Analysis", `job completed `+supportFixtureJob+` total=1000000 analyzed=1000000 skipped=1000000 failed=1000000`)
	data, stats = projectSupportLog([]byte(strings.Repeat(long, maxSupportRecords)))
	if len(data) > maxSupportProjectionBytes || stats.Included >= maxSupportRecords || stats.Dropped == 0 || !stats.SizeLimited {
		t.Fatalf("output bound: bytes=%d stats=%+v", len(data), stats)
	}
}

func TestSupportLogTailBoundRejectsPartialLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "viib.log")
	tail := supportFixtureLine("API", "Spotify catalog failure: code=unknown stage=track upstream_status=0")
	if err := os.WriteFile(path, []byte(strings.Repeat("private", maxSupportLogBytes/7+10)+"\n"+tail), 0600); err != nil {
		t.Fatal(err)
	}
	data, limited, err := readSupportLog(path)
	if err != nil || !limited || len(data) > maxSupportLogBytes || string(data) != tail {
		t.Fatalf("tail bound/partial line: bytes=%d limited=%v err=%v", len(data), limited, err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxSupportLogBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	data, limited, err = readSupportLog(path)
	if err != nil || !limited || len(data) != 0 {
		t.Fatal("unterminated oversized tail was retained")
	}
}

func TestSupportReferenceProjectionRejectsUnreviewedFields(t *testing.T) {
	for _, message := range []string{
		`support reference issued token="private-host-request" status=503`,
		`support reference issued token="12345678-1234-4123-8123-123456789abc" status=503 requestId="private-request"`,
		`support reference issued token="12345678-1234-4123-8123-123456789abc" status=200`,
	} {
		data, stats := projectSupportLog([]byte(supportFixtureLine("API", message)))
		if len(data) != 0 || stats.Dropped != 1 {
			t.Fatalf("unsafe reference record passed: %q", data)
		}
	}
}
