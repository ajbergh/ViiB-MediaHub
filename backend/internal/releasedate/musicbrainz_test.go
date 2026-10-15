package releasedate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const goodRecording = `{"id":"12345678-1234-1234-1234-123456789abc","title":"Like a Prayer","length":341000,"score":100,"first-release-date":"1989-03-03","artist-credit":[{"name":"Madonna","artist":{"name":"Madonna"}}]}`

func TestMusicBrainzAcceptsOnlyIdentifiedOriginalRelease(t *testing.T) {
	for _, test := range []struct{ name, recordings, state string }{
		{"identified", goodRecording, "available"},
		{"wrong_artist", strings.ReplaceAll(goodRecording, "Madonna", "Other"), "not_found"},
		{"wrong_duration", strings.ReplaceAll(goodRecording, "341000", "240000"), "not_found"},
		{"live", strings.Replace(goodRecording, `"length"`, `"disambiguation":"live in London","length"`, 1), "not_found"},
		{"ambiguous", goodRecording + "," + strings.ReplaceAll(goodRecording, "123456789abc", "123456789def"), "ambiguous"},
		{"no_date", strings.ReplaceAll(goodRecording, "1989-03-03", ""), "not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.UserAgent(), "ViiB-MediaHub") || r.URL.Query().Get("query") != `recording:"Like a Prayer" AND artist:"Madonna"` {
					t.Error("incorrect MusicBrainz query or user agent")
				}
				w.Write([]byte(`{"recordings":[` + test.recordings + `]}`))
			}))
			defer s.Close()
			client := NewClient()
			client.BaseURL = s.URL
			client.HTTP = s.Client()
			match, err := client.Lookup(t.Context(), "Like a Prayer - 2024 Remaster", "Madonna", 341)
			if err != nil || match.State != test.state {
				t.Fatalf("match=%+v err=%v", match, err)
			}
			if test.state == "available" && match.Date != "1989-03-03" {
				t.Fatal(match)
			}
		})
	}
}

func TestMusicBrainzHonorsCooldownAndCancellation(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(503)
	}))
	defer s.Close()
	client := NewClient()
	client.BaseURL = s.URL
	client.HTTP = s.Client()
	_, err := client.Lookup(t.Context(), "Track", "Artist", 200)
	var cooldown *CooldownError
	if !errors.As(err, &cooldown) || time.Until(cooldown.Until) < 59*time.Minute {
		t.Fatalf("missing cooldown: %v", err)
	}
	_, err = client.Lookup(t.Context(), "Track", "Artist", 200)
	if !errors.As(err, &cooldown) || calls != 1 {
		t.Fatalf("cooldown ignored: calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Lookup(ctx, "Track", "Artist", 200); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
