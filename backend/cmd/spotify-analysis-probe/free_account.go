//go:build spotify_research

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// Reads only product labels, never account identifiers or raw provider output.
func probeAccountProduct(ctx context.Context, bearer string, client *http.Client) (string, error) {
	httpClient := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		httpClient = *client
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://spclient.wg.spotify.com/melody/v1/product_state", nil)
	if err != nil {
		return "unknown", errors.New("account check failed")
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Accept", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return "unknown", errors.New("account check failed")
	}
	defer response.Body.Close()
	var state struct {
		Catalogue string `json:"catalogue"`
		Product   string `json:"product"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&state) != nil {
		return "unknown", errors.New("account check failed")
	}
	normalize := func(value string) string {
		switch value {
		case "free", "open":
			return "free"
		case "premium":
			return "premium"
		}
		return "unknown"
	}
	if state.Catalogue == "" {
		return normalize(state.Product), nil
	}
	product := normalize(state.Catalogue)
	if state.Product != "" && normalize(state.Product) != product {
		return "unknown", nil
	}
	return product, nil
}

// Identification-header nominal bitrate is evidence of the selected Vorbis
// encoding, not a measurement of average VBR bandwidth or sustained playback.
func vorbisNominalKbps(raw []byte) int {
	if len(raw) < 27 || string(raw[:4]) != "OggS" || raw[4] != 0 {
		return 0
	}
	segments := int(raw[26])
	start := 27 + segments
	if segments == 0 || start > len(raw) {
		return 0
	}
	length := 0
	for _, size := range raw[27:start] {
		length += int(size)
		if size < 255 {
			if length < 30 || start+length > len(raw) {
				return 0
			}
			packet := raw[start : start+length]
			if packet[0] != 1 || string(packet[1:7]) != "vorbis" || binary.LittleEndian.Uint32(packet[7:11]) != 0 || packet[11] == 0 || binary.LittleEndian.Uint32(packet[12:16]) == 0 || packet[29]&1 == 0 {
				return 0
			}
			nominal := int32(binary.LittleEndian.Uint32(packet[20:24]))
			if nominal <= 0 || nominal > 1000000 {
				return 0
			}
			return int(nominal) / 1000
		}
	}
	return 0
}
