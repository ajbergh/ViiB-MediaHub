//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

// Observed in the public desktop Web Player bundle on 2026-10-16.
// Kept diagnostic-only: the production playlist operation has an older projection.
const uiPlaylistHash = "21ea11a6ac75a5f3e01fd365d29c18e0a4a8b7dc09f89bbb5d73910190c6acaf"

func writePlaylistMetadata(ctx context.Context, manager *auth.Manager, contract auth.WebPlayerContract, playlistID, trackID string, out io.Writer) error {
	token, err := manager.Token(ctx, auth.WebAPI)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	clientPayload, _ := json.Marshal(map[string]any{"client_data": map[string]any{"client_version": contract.AppVersion, "client_id": token.WebPlayerClientID(), "js_sdk_data": map[string]string{"device_brand": "", "device_id": "", "device_model": "", "device_type": "", "os": "", "os_version": ""}}})
	clientRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://clienttoken.spotify.com/v1/clienttoken", bytes.NewReader(clientPayload))
	if err != nil {
		return err
	}
	clientRequest.Header.Set("Content-Type", "application/json")
	clientRequest.Header.Set("Accept", "application/json")
	clientRequest.Header.Set("App-Platform", "WebPlayer")
	clientRequest.Header.Set("Spotify-App-Version", contract.AppVersion)
	clientRequest.Header.Set("Origin", "https://open.spotify.com")
	clientRequest.Header.Set("Referer", "https://open.spotify.com/")
	clientRequest.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15")
	clientResponse, err := client.Do(clientRequest)
	if err != nil {
		return errors.New("playlist client context request failed")
	}
	var clientContext struct {
		Granted struct {
			Token string `json:"token"`
		} `json:"granted_token"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(clientResponse.Body, 65536)).Decode(&clientContext)
	clientResponse.Body.Close()
	if clientResponse.StatusCode != 200 || decodeErr != nil || clientContext.Granted.Token == "" {
		return fmt.Errorf("playlist client context unavailable (HTTP %d)", clientResponse.StatusCode)
	}
	// Bounded traversal; missing tracks and missing attributes remain explicit errors.
	for offset := 0; offset < 1000; offset += 100 {
		payload := map[string]any{"operationName": "fetchPlaylist", "variables": map[string]any{"uri": "spotify:playlist:" + playlistID, "offset": offset, "limit": 100, "enableWatchFeedEntrypoint": false}, "extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": uiPlaylistHash}}}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api-partner.spotify.com/pathfinder/v2/query", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token.Bearer())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("App-Platform", "WebPlayer")
		req.Header.Set("Spotify-App-Version", contract.AppVersion)
		req.Header.Set("Client-Token", clientContext.Granted.Token)
		req.Header.Set("Origin", "https://open.spotify.com")
		req.Header.Set("Referer", "https://open.spotify.com/")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15")
		response, err := client.Do(req)
		if err != nil {
			return errors.New("playlist request failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
		response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("playlist request HTTP %d", response.StatusCode)
		}
		if readErr != nil || len(raw) > 8<<20 {
			return errors.New("invalid bounded playlist response")
		}
		result, total, err := findPlaylistTrack(raw, trackID)
		if err != nil {
			return err
		}
		if result != nil {
			encoder := json.NewEncoder(out)
			encoder.SetIndent("", "  ")
			return encoder.Encode(map[string]any{"sourceEndpoint": "pathfinder/fetchPlaylist", "queryHash": uiPlaylistHash, "playlistId": playlistID, "retrievedAt": time.Now().UTC(), "track": result})
		}
		if offset+100 >= total {
			break
		}
	}
	return errors.New("track not present in this account's playlist response (bounded to 1000 rows)")
}

type playlistDiagnosticTrack struct {
	URI        string          `json:"uri"`
	Name       string          `json:"name"`
	Attributes json.RawMessage `json:"playlistAudioAttributes"`
}

func findPlaylistTrack(raw []byte, id string) (*playlistDiagnosticTrack, int, error) {
	var reply struct {
		Errors []json.RawMessage `json:"errors"`
		Data   struct {
			Playlist *struct {
				Content *struct {
					Total int `json:"totalCount"`
					Items []struct {
						Item struct {
							Data playlistDiagnosticTrack `json:"data"`
						} `json:"itemV2"`
					} `json:"items"`
				} `json:"content"`
			} `json:"playlistV2"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &reply) != nil || len(reply.Errors) > 0 || reply.Data.Playlist == nil || reply.Data.Playlist.Content == nil {
		return nil, 0, errors.New("incompatible playlist metadata response")
	}
	page := reply.Data.Playlist.Content
	for _, row := range page.Items {
		if row.Item.Data.URI == "spotify:track:"+id {
			track := row.Item.Data
			return &track, page.Total, nil
		}
	}
	return nil, page.Total, nil
}
