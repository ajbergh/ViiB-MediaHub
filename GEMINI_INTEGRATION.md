# Gemini Integration for Smart Mixes

ViiB MediaHub now supports using Google's Gemini API to automatically populate genre information for songs that are missing it. This enables better "Smart Mixes" and genre-based filtering.

## Setup

1.  **Get a Gemini API Key**:
    -   Go to [Google AI Studio](https://aistudio.google.com/).
    -   Create an API key.

2.  **Trigger Enrichment**:
    -   You can trigger the enrichment process via a simple HTTP POST request.
    -   The backend will process up to 50 songs at a time to respect API limits. You can run this multiple times.

## Usage

### Via cURL

Replace `YOUR_API_KEY` with your actual Gemini API key.

```bash
curl -X POST http://localhost:8080/api/library/enrich-genres \
  -H "Content-Type: application/json" \
  -d '{"apiKey": "YOUR_API_KEY"}'
```

### Response

```json
{
  "status": "ok",
  "message": "Successfully enriched 42 songs",
  "count": 42
}
```

If no songs need enrichment:

```json
{
  "status": "ok",
  "message": "No songs found with missing genres",
  "count": 0
}
```

## Implementation Details

-   **Backend**: `internal/gemini` package handles the API communication.
-   **Database**: Stores genres as a JSON array in the `songs` table.
-   **Privacy**: Song metadata (Artist, Title, Album) is sent to Google's API. No audio data is uploaded.
