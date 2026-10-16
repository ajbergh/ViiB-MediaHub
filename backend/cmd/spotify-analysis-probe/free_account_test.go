//go:build spotify_research

package main

import (
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFreeAccountFailureClassificationDoesNotLeakRawErrors(t *testing.T) {
	for _, tc := range []struct{ marker, want string }{
		{"PremiumAccountRequired", "media_premium_required"},
		{"BadCredentials", "audio_credentials_rejected"},
		{"could not get ap url", "audio_endpoint_resolution_failed"},
		{"could not connect to", "audio_connection_failed"},
		{"failed to login to Spotify", "audio_login_failed"},
	} {
		if got := audioErrorCode(errors.New(tc.marker+" private-token account"), "audio_session"); got != tc.want {
			t.Fatalf("classification=%s", got)
		}
	}
}

type freeAccountTransport func(*http.Request) (*http.Response, error)

func (f freeAccountTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFreeAccountProductIsVerifiedAndRedacted(t *testing.T) {
	for _, tc := range []struct {
		body, want string
		status     int
	}{
		{`{"catalogue":"free","product":"free","private_field":"secret"}`, "free", 200},
		{`{"catalogue":"open","product":"free"}`, "free", 200},
		{`{"product":"premium"}`, "premium", 200},
		{`{"catalogue":"premium","product":"free"}`, "unknown", 200},
		{`{}`, "unknown", 200},
		{`{"error":"secret"}`, "unknown", 403},
	} {
		client := &http.Client{Transport: freeAccountTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://spclient.wg.spotify.com/melody/v1/product_state" || r.Header.Get("Authorization") != "Bearer fixture" || r.Header.Get("Cookie") != "" {
				t.Error("incorrect account check request")
			}
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		product, err := probeAccountProduct(t.Context(), "fixture", client)
		if product != tc.want || (tc.status != 200 && err == nil) || (err != nil && strings.Contains(err.Error(), "secret")) {
			t.Fatalf("product=%s err=%v", product, err)
		}
	}
}

func TestVorbisNominalBitrateUsesIdentificationPacket(t *testing.T) {
	raw := make([]byte, 4096)
	copy(raw, "OggS")
	raw[26] = 1
	raw[27] = 30
	packet := raw[28:58]
	packet[0] = 1
	copy(packet[1:], "vorbis")
	packet[11] = 2
	binary.LittleEndian.PutUint32(packet[12:], 44100)
	packet[29] = 1
	for _, rate := range []uint32{96000, 160000, 320000} {
		binary.LittleEndian.PutUint32(packet[20:], rate)
		if got := vorbisNominalKbps(raw); got != int(rate)/1000 {
			t.Fatalf("bitrate=%d", got)
		}
	}
	for _, bad := range [][]byte{nil, raw[:27], raw[:50], []byte("OggS"), make([]byte, 4096)} {
		if vorbisNominalKbps(bad) != 0 {
			t.Fatal("malformed header claimed bitrate")
		}
	}
	binary.LittleEndian.PutUint32(packet[20:], 0xffffffff)
	if vorbisNominalKbps(raw) != 0 {
		t.Fatal("unknown nominal bitrate claimed as measured")
	}
}

func TestFreeAccountReportCannotClaimPremiumOrCompleteDownload(t *testing.T) {
	report := audioReport{TrackID: "5RIh3Tsq9U7CAvivB2iB7q", ProbeKind: "free_account", AccountProduct: "free", RequestedBitrateKbps: 96, AuthenticationVerified: true, Status: "success", Stage: "complete", Initial: audioPass{SessionAuthenticated: true, StreamOpened: true, BytesRead: 4096, OggHeaderVerified: true, VorbisNominalBitrateKbps: 96}}
	if !validAudioReport(report) {
		t.Fatal("valid free report rejected")
	}
	for _, mutate := range []func(*audioReport){
		func(r *audioReport) { r.AccountProduct = "premium" },
		func(r *audioReport) { r.Initial.VorbisNominalBitrateKbps = 320 },
		func(r *audioReport) { r.Initial.VorbisNominalBitrateKbps = 0 },
		func(r *audioReport) { r.AuthenticationVerified = false },
		func(r *audioReport) { r.RenewalVerified = true },
		func(r *audioReport) { r.AccountProduct = "raw secret" },
	} {
		bad := report
		mutate(&bad)
		if validAudioReport(bad) {
			t.Fatalf("false capability claim accepted: %+v", bad)
		}
	}
}
