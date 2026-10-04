package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testID = "11dFghVXANMlKmJXsNCbNl"

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testHTTP(server *httptest.Server) *http.Client {
	target, _ := url.Parse(server.URL)
	return &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme, u.Host = target.Scheme, target.Host
		clone.URL = &u
		return server.Client().Transport.RoundTrip(clone)
	})}
}
func testTokens(refreshes *atomic.Int32) *auth.Manager {
	return auth.NewManager(nil, auth.ProviderFuncs{
		Load: func(context.Context) (auth.Token, error) {
			return auth.NewToken("old", auth.WebPlayer, time.Now().Add(time.Hour), 1), nil
		},
		Renew: func(context.Context, auth.Token) (auth.Token, error) {
			refreshes.Add(1)
			return auth.NewToken("new", auth.WebPlayer, time.Now().Add(time.Hour), 2), nil
		},
	})
}
func assertCode(t *testing.T, err error, want Code) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}
func TestClientIsOptInAndValidatesIDBeforeAuth(t *testing.T) {
	client := NewClient(nil, Options{})
	_, err := client.Fetch(context.Background(), testID)
	assertCode(t, err, Disabled)
	client = NewClient(nil, Options{Enabled: true, AppVersion: "fixture"})
	for _, id := range []string{"", "spotify:track:" + testID, "https://open.spotify.com/track/" + testID, "../" + testID, strings.Repeat("a", 23)} {
		_, err = client.Fetch(context.Background(), id)
		assertCode(t, err, InvalidTrackID)
	}
}
func TestClientNormalizesNullableScalarsAndAllKeys(t *testing.T) {
	major := []string{"8B", "3B", "10B", "5B", "12B", "7B", "2B", "9B", "4B", "11B", "6B", "1B"}
	minor := []string{"5A", "12A", "7A", "2A", "9A", "4A", "11A", "6A", "1A", "8A", "3A", "10A"}
	for mode := 0; mode <= 1; mode++ {
		for pitch := 0; pitch < 12; pitch++ {
			body := fmt.Sprintf(`{"track":{"key":%d,"mode":%d,"tempo_confidence":0}}`, pitch, mode)
			observation, err := decodeFixture(body)
			if err != nil {
				t.Fatal(err)
			}
			want := minor[pitch]
			if mode == 1 {
				want = major[pitch]
			}
			if observation.Camelot == nil || *observation.Camelot != want || observation.BPM != nil ||
				observation.BPMConfidence == nil || *observation.BPMConfidence != 0 {
				t.Fatalf("incorrect nullable key %d/%d", pitch, mode)
			}
		}
	}
	for _, body := range []string{`{"track":{"tempo":124,"key":-1,"mode":1}}`, `{"track":{"tempo":124}}`} {
		got, err := decodeFixture(body)
		if err != nil || got.Key != nil || got.Camelot != nil {
			t.Fatalf("unknown became known: %v", err)
		}
	}
}
func decodeFixture(body string) (Observation, error) {
	// Exercise the same bounded decoder as HTTP, including unknown field support.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer server.Close()
	var refreshes atomic.Int32
	return NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)}).Fetch(context.Background(), testID)
}
func TestClientRejectsMalformedAndOversizedPayloads(t *testing.T) {
	cases := []string{
		`{}`, `{"track":null}`, `{"track":{}}`, `{"track":{"tempo":0}}`,
		`{"track":{"tempo":124,"key":12}}`, `{"track":{"tempo":124,"mode":2}}`,
		`{"track":{"tempo":124,"key_confidence":1.01}}`, `{"track":{"tempo":"124"}}`,
		`{"track":{"tempo":124,"duration":4},"beats":[{"start":3,"duration":2}]}`,
		`{"track":{"tempo":124},"beats":[{"duration":2}]}`,
		"<html>private-body</html>", strings.Repeat(" ", (8<<20)+1),
	}
	for i, body := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := decodeFixture(body)
			want := ProviderChanged
			if body == `{"track":{}}` {
				want = AnalysisUnavailable
			}
			assertCode(t, err, want)
			var diagnostic *Error
			if !errors.As(err, &diagnostic) || diagnostic.HTTPStatus != http.StatusOK {
				t.Fatal("lost successful response status")
			}
			if strings.Contains(err.Error(), "private-body") {
				t.Fatal("body leaked")
			}
		})
	}
}
func TestClientHTTPPolicyAndOneAuthRetry(t *testing.T) {
	for _, test := range []struct {
		status           int
		want             Code
		calls, refreshes int
	}{
		{401, AuthenticationRequired, 2, 1}, {403, AccessDenied, 1, 0}, {404, NotFound, 1, 0},
		{429, RateLimited, 1, 0}, {500, TemporarilyUnavailable, 1, 0}, {302, ProviderChanged, 1, 0},
	} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			var calls, refreshes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("App-Platform") != "WebPlayer" || r.Header.Get("Spotify-App-Version") != "fixture" ||
					r.Header.Get("Cookie") != "" || r.URL.Path != "/audio-attributes/v1/audio-analysis/"+testID {
					t.Error("invalid request")
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(test.status)
				fmt.Fprint(w, "private-body")
			}))
			defer server.Close()
			_, err := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)}).Fetch(context.Background(), testID)
			assertCode(t, err, test.want)
			if calls.Load() != int32(test.calls) || refreshes.Load() != int32(test.refreshes) {
				t.Fatal("incorrect retry count")
			}
			if test.status == 429 {
				var e *Error
				errors.As(err, &e)
				if e.RetryAfter != 120*time.Second {
					t.Fatal("lost Retry-After")
				}
			}
		})
	}
}
func TestClientRefreshSuccessAndUnknownFields(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer old" {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer new" {
			t.Error("wrong token")
		}
		fmt.Fprint(w, `{"track":{"tempo":124,"key":9,"mode":0,"duration":180},"beats":[{"start":0,"duration":0.5,"confidence":0}],"new_field":{"value":1},"meta":{"analyzer_version":"fixture"}}`)
	}))
	defer server.Close()
	got, err := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)}).Fetch(context.Background(), testID)
	if err != nil || got.Source != "spotify_internal" || got.AnalyzerVersion != "fixture" || got.Camelot == nil || *got.Camelot != "8A" || refreshes.Load() != 1 {
		t.Fatalf("normalization: %+v %v", got, err)
	}
}
func TestClientCancellationAndRedirect(t *testing.T) {
	var refreshes, destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer server.Close()
	client := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)})
	_, err := client.Fetch(context.Background(), testID)
	assertCode(t, err, ProviderChanged)
	if destinationCalls.Load() != 0 {
		t.Fatal("redirect forwarded token")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Fetch(ctx, testID)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestClientCancellationDuringRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	var refreshes atomic.Int32
	client := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.Fetch(ctx, testID); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if refreshes.Load() != 0 {
		t.Fatal("canceled request retried")
	}
}

func TestExplicitFeaturesPreservesProvenanceAndMissingConfidence(t *testing.T) {
	var refreshes atomic.Int32
	var featureCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio-attributes/v1/audio-analysis/"+testID {
			w.WriteHeader(404)
			return
		}
		featureCalls.Add(1)
		if r.URL.Path != "/audio-attributes/v1/audio-features/"+testID || r.URL.Query().Get("format") != "json" {
			t.Error("incorrect feature route")
		}
		fmt.Fprintf(w, `{"id":%q,"type":"audio_features","tempo":128.3,"key":9,"mode":0,"duration_ms":180000,"time_signature":4,"loudness":-6.4,"unexpected_token":"must-not-leak"}`, testID)
	}))
	defer server.Close()
	client := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)})
	_, err := client.Fetch(context.Background(), testID)
	assertCode(t, err, NotFound)
	if featureCalls.Load() != 0 {
		t.Fatal("analysis silently fell back to features")
	}
	observation, err := client.FetchFeatures(context.Background(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Source != "spotify_internal" || observation.SourceEndpoint != "audio_features" || observation.BPM == nil || *observation.BPM != 128.3 || observation.Camelot == nil || *observation.Camelot != "8A" || observation.DurationSeconds == nil || *observation.DurationSeconds != 180 {
		t.Fatalf("invalid feature observation: %+v", observation)
	}
	if observation.BPMConfidence != nil || observation.KeyConfidence != nil || observation.ModeConfidence != nil || observation.AnalyzerVersion != "" {
		t.Fatal("invented analysis facts")
	}
	encoded, _ := json.Marshal(observation)
	if strings.Contains(string(encoded), "must-not-leak") {
		t.Fatal("raw fields leaked")
	}
}
func TestFeaturesRejectsMismatchedRecordingAndMalformedScalars(t *testing.T) {
	for _, body := range []string{
		`{"id":"different","tempo":128}`,
		fmt.Sprintf(`{"id":%q,"type":"episode","tempo":128}`, testID),
		fmt.Sprintf(`{"id":%q,"tempo":128,"duration_ms":-1}`, testID),
		fmt.Sprintf(`{"id":%q,"tempo":128,"key":99}`, testID),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		var refreshes atomic.Int32
		_, err := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", Client: testHTTP(server)}).FetchFeatures(context.Background(), testID)
		server.Close()
		assertCode(t, err, ProviderChanged)
	}
}
