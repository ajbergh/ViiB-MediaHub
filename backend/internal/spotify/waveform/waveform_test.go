package waveform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"google.golang.org/protobuf/encoding/protowire"
)

const fixtureID = "5r9W9MJLvHk83fcZSPQ8SE"

func number(tag protowire.Number, n uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, tag, protowire.VarintType), n)
}
func message(tag protowire.Number, raw []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, tag, protowire.BytesType), raw)
}
func joined(parts ...[]byte) []byte {
	var raw []byte
	for _, part := range parts {
		raw = append(raw, part...)
	}
	return raw
}
func fixtureDomain(packed bool) []byte {
	raw := joined(number(1, 44100), number(2, 20))
	for tag := protowire.Number(3); tag <= 5; tag++ {
		values := []uint64{0, 1, 0xffffffffffffffff}
		if packed {
			var band []byte
			for _, n := range values {
				band = protowire.AppendVarint(band, n)
			}
			raw = append(raw, message(tag, band)...)
		} else {
			for _, n := range values {
				raw = append(raw, number(tag, n)...)
			}
		}
	}
	return raw
}
func fixtureResponse(status int, id, typ string, domain []byte) []byte {
	header := joined(number(1, uint64(status)), message(2, []byte("fixture-etag")))
	any := joined(message(1, []byte(typ)), message(2, domain))
	entity := joined(message(1, header), message(2, []byte("spotify:track:"+id)), message(3, any))
	extension := joined(message(1, number(1, 0)), number(2, ExtensionKind), message(3, entity))
	return message(2, extension)
}
func TestDecodeWaveformPackedUnpackedAndUnknownFields(t *testing.T) {
	for _, packed := range []bool{true, false} {
		domain := append(fixtureDomain(packed), number(99, 7)...)
		raw := append(fixtureResponse(200, fixtureID, TypeURL, domain), message(99, []byte("unknown"))...)
		got, err := DecodeResponse(raw, fixtureID)
		if err != nil || len(got.Lows) != 3 || got.Highs[2] != -1 || got.SampleRate != 44100 || got.ETag != "fixture-etag" || got.TrackID != fixtureID {
			t.Fatalf("decode: %+v %v", got, err)
		}
		if !got.Aligned(.06) || got.Aligned(300) {
			t.Fatal("invalid alignment policy")
		}
	}
}
func TestDecodeRejectsMalformedIdentityAndEntityFailures(t *testing.T) {
	for _, raw := range [][]byte{
		fixtureResponse(200, strings.Repeat("A", 22), TypeURL, fixtureDomain(true)),
		fixtureResponse(200, fixtureID, "wrong-type", fixtureDomain(true)),
		fixtureResponse(200, fixtureID, TypeURL, joined(number(1, 44100), number(2, 0))),
		fixtureResponse(200, fixtureID, TypeURL, joined(number(1, 44100), number(2, 20), number(3, 1), number(4, 1))),
		{0x12, 0xff},
	} {
		if _, err := DecodeResponse(raw, fixtureID); err == nil {
			t.Fatal("malformed waveform accepted")
		}
	}
	for _, status := range []int{404, 451, 304} {
		_, err := DecodeResponse(fixtureResponse(status, fixtureID, TypeURL, nil), fixtureID)
		var denied *EntityError
		if !errors.As(err, &denied) || denied.Status != status {
			t.Fatal("entity status lost")
		}
	}
	domain := joined(number(1, 44100), number(2, 20), message(3, make([]byte, MaxSamples+1)))
	if _, err := DecodeDomain(domain); err == nil {
		t.Fatal("sample limit ignored")
	}
	if _, err := DecodeDomain(joined(number(1, 44100), number(2, 20), message(3, []byte{0x80}))); err == nil {
		t.Fatal("truncated packed sample accepted")
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func tokens() *auth.Manager {
	return auth.NewManager(nil, auth.ProviderFuncs{Load: func(context.Context) (auth.Token, error) {
		return auth.NewToken("fixture", auth.WebPlayer, time.Now().Add(time.Hour), 1), nil
	}, Renew: func(context.Context, auth.Token) (auth.Token, error) {
		return auth.NewToken("renewed", auth.WebPlayer, time.Now().Add(time.Hour), 2), nil
	}})
}
func TestTransportRenewalAnd304CacheFencing(t *testing.T) {
	for _, cachePresent := range []bool{false, true} {
		calls := 0
		client := NewClient(tokens(), Options{Enabled: true, AppVersion: "fixture", Client: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "spclient.wg.spotify.com" || r.URL.Path != "/extended-metadata/v0/extended-metadata" || r.Method != "POST" || r.Header.Get("Cookie") != "" || r.Header.Get("Accept") != "application/protobuf" {
				t.Fatal("unsafe dispatch")
			}
			status := 200
			raw := fixtureResponse(200, fixtureID, TypeURL, fixtureDomain(true))
			if calls == 1 {
				status = 401
			} else if calls == 2 {
				status = 304
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		})}})
		var cached *Waveform
		if cachePresent {
			value, _ := DecodeResponse(fixtureResponse(200, fixtureID, TypeURL, fixtureDomain(true)), fixtureID)
			cached = &value
		}
		got, err := client.Fetch(context.Background(), fixtureID, "fixture-etag", cached)
		expected := 3
		if cachePresent {
			expected = 2
		}
		if err != nil || len(got.Lows) != 3 || calls != expected {
			t.Fatalf("304 handling: %+v %v calls=%d", got, err, calls)
		}
	}
}
func TestTransportEntityDeniedAndRateLimitAreIndependent(t *testing.T) {
	for _, status := range []int{404, 451, 429} {
		client := NewClient(tokens(), Options{Enabled: true, AppVersion: "fixture", Client: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Retry-After": []string{"30"}}, Body: io.NopCloser(strings.NewReader(string(fixtureResponse(status, fixtureID, TypeURL, nil))))}, nil
		})}})
		_, err := client.Fetch(context.Background(), fixtureID, "", nil)
		var failure *Error
		if !errors.As(err, &failure) || failure.EntityStatus != status || failure.HTTPStatus != 200 || failure.Code != codeForStatus(status) {
			t.Fatalf("nested status lost: %v", err)
		}
		if status == 429 && failure.RetryAfter != 30*time.Second {
			t.Fatal("retry delay lost")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewClient(tokens(), Options{Enabled: true, AppVersion: "fixture"}).Fetch(canceled, fixtureID, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	_, err = NewClient(nil, Options{}).Fetch(context.Background(), fixtureID, "", nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != spotifyanalysis.Disabled {
		t.Fatal("unexpected enabled client")
	}
}
