package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const apiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"

type Client struct {
	apiKey string
}

func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey}
}

type generateContentRequest struct {
	Contents []content `json:"contents"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generateContentResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}

// EnrichGenres takes a list of songs and returns a map of song ID to genres using Gemini.
func (c *Client) EnrichGenres(songs []db.Song) (map[string][]string, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString(`You are a music expert with deep knowledge of artists, genres, and subgenres.

Your task is to identify the most accurate genres for each song, taking into account:

1. Song-specific characteristics (sound, instrumentation, production era, stylistic elements).
2. The band's or artist's overall established genres, which must be reflected in the song's genre list unless clearly contradicted by the actual sound.

OUTPUT REQUIREMENTS:
- Return ONLY a valid JSON object.
- Keys: Song IDs exactly as provided.
- Values: Arrays of genre strings.
- Genres should be ordered from most specific (e.g., "Dream Pop") to broader parent genres (e.g., "Indie Rock").
- Do not include any explanations, comments, markdown, or code blocks — JSON only.

GENRE SELECTION RULES:
- Use real, widely recognized genres.
- Prefer specific subgenres when strongly supported.
- Include broader genres only as parents or fallback categories.
- If uncertain, choose the most widely accepted critical consensus for the song or artist.`)

	for _, song := range songs {
		promptBuilder.WriteString(fmt.Sprintf("ID: %s | Artist: %s | Title: %s | Album: %s\n", song.ID, song.Artist, song.Title, song.Album))
	}

	reqBody := generateContentRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: promptBuilder.String()},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", apiEndpoint, c.apiKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))
	}

	var geminiResp generateContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	// Clean up potential markdown code blocks if Gemini ignores the instruction
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var result map[string][]string
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response from Gemini: %w. Response was: %s", err, responseText)
	}

	return result, nil
}
