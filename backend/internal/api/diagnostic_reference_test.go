package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/google/uuid"
)

func TestMaintenanceFailureReferenceReachesSupportArchive(t *testing.T) {
	directory := t.TempDir()
	if err := logger.Init(directory); err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	a := &API{dataDir: directory}
	router := a.V2LibraryOperationRoutes()
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "/repair?title=PrivateFixtureTitle", strings.NewReader(`{"private":"PrivateBody"`))
		request.Header.Set("X-Request-ID", "PrivateHostRequestID")
		incoming := "PrivateIncomingToken"
		if i == 1 {
			incoming = "87654321-1234-4123-8123-123456789abc"
		}
		request.Header.Set(DiagnosticTokenHeader, incoming)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var envelope struct{ Error V2Error }
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusBadRequest {
			t.Fatalf("maintenance response: %d %v", response.Code, err)
		}
		token := envelope.Error.DiagnosticToken
		parsed, err := uuid.Parse(token)
		if err != nil || parsed.Version() != 4 || token != response.Header().Get(DiagnosticTokenHeader) || seen[token] || token == incoming {
			t.Fatalf("reference is not an independent UUID v4: %q %v", token, err)
		}
		seen[token] = true
		if envelope.Error.RequestID != "PrivateHostRequestID" {
			t.Fatal("legacy request ID semantics changed")
		}
	}
	archive, _, err := a.CreateSupportBundle()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range reader.File {
		source, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(source)
		source.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"PrivateFixtureTitle", "PrivateBody", "PrivateHostRequestID", "PrivateIncomingToken", "Repair options are not valid JSON"} {
			if bytes.Contains(content, []byte(forbidden)) {
				t.Fatalf("archive entry %s leaked %s", entry.Name, forbidden)
			}
		}
		if entry.Name != "logs/diagnostics.jsonl" {
			continue
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatal(err)
			}
			token, _ := record["diagnosticToken"].(string)
			if seen[token] {
				if len(record) != 4 || record["event"] != "v2_error" || record["status"] != float64(400) {
					t.Fatalf("unexpected reference record: %v", record)
				}
				found[token] = true
			}
		}
	}
	if len(found) != len(seen) {
		t.Fatalf("response references absent from archive: %v %v", seen, found)
	}
}
