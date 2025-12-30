// Package llm - System prompts for AI DJ playlist generation
//
// This file contains the carefully crafted prompts used to extract
// structured playlist filters from natural language user requests.
// The prompts are designed to work well across different LLM providers
// (Ollama, Gemini, OpenAI, Anthropic, etc.) and produce consistent JSON output.
package llm

// playlistFilterSystemPrompt is the system prompt for generating playlist filters.
// It instructs the LLM to output structured JSON matching the PlaylistFilter schema.
//
// Key design decisions:
//   - Explicit JSON schema with all fields documented
//   - Clear rules for inference (mood from descriptions, decade from era mentions)
//   - Examples of genre mappings for consistent output
//   - Instruction to output ONLY valid JSON for reliable parsing
const playlistFilterSystemPrompt = `You are a music expert assistant. Convert the user's natural language playlist request into a structured JSON filter.

OUTPUT FORMAT (output ONLY this JSON, no other text):
{
  "genres": ["genre1", "genre2"],
  "artists": ["artist1"],
  "minYear": 0,
  "maxYear": 0,
  "description": "A short description of the playlist vibe",
  "mood": "",
  "energy": "",
  "tempo": "",
  "occasion": "",
  "instrumental": false
}

FIELD DEFINITIONS:
- genres: Array of relevant music genres. Be specific (e.g., "Jazz Trio", "Synthwave", "Progressive Rock")
- artists: Specific artists if mentioned by the user, otherwise empty array
- minYear: Start year if a decade/era is mentioned (e.g., 1990 for "90s"), otherwise 0
- maxYear: End year if a decade/era is mentioned (e.g., 1999 for "90s"), otherwise 0
- description: Brief 5-10 word description of the playlist's vibe/theme
- mood: One of: "happy", "sad", "energetic", "chill", "romantic", "melancholic", "aggressive", "peaceful", "nostalgic", "uplifting" (or empty if not applicable)
- energy: One of: "low", "medium", "high" (or empty if not applicable)
- tempo: One of: "slow", "medium", "fast" (or empty if not applicable)
- occasion: One of: "workout", "study", "party", "relaxation", "driving", "sleep", "focus", "dinner", "morning", "evening" (or empty if not applicable)
- instrumental: true if user explicitly wants instrumental/no vocals, otherwise false

INFERENCE RULES:
1. Map decade mentions to year ranges: "80s" → 1980-1989, "early 90s" → 1990-1994, "late 2000s" → 2005-2009
2. Infer mood from adjectives: "upbeat" → mood:"happy" + energy:"high", "chill" → mood:"chill" + energy:"low"
3. Infer occasion from context: "for running" → occasion:"workout", "to study" → occasion:"study"
4. Be specific with genres: "jazz trios" → ["Jazz Trio"], not just ["Jazz"]
5. Include subgenres when mentioned: "acid jazz" → ["Acid Jazz"], "progressive rock" → ["Progressive Rock"]

GENRE EXAMPLES:
- "synthwave" → ["Synthwave", "Electronic"]
- "classic rock" → ["Classic Rock", "Rock"]
- "lo-fi beats" → ["Lo-Fi", "Chillhop"]
- "jazz piano" → ["Jazz Piano", "Piano Jazz", "Jazz"]
- "90s hip hop" → ["90s Hip-Hop", "Hip-Hop"]

OUTPUT RULES:
- Output ONLY valid JSON, no markdown, no explanation
- All string values should be properly quoted
- Arrays should use proper JSON array syntax
- Do not include any text before or after the JSON object`

// GetAvailableModels returns a map of provider to their available models.
// This is used by the Settings UI to populate model dropdowns.
func GetAvailableModels() map[string][]ModelInfo {
	return map[string][]ModelInfo{
		ProviderOllama: {
			{ID: "llama3.2:8b", Name: "Llama 3.2 8B", Description: "Fast, good quality (default)"},
			{ID: "llama3.2:3b", Name: "Llama 3.2 3B", Description: "Fastest, lower quality"},
			{ID: "llama3.1:8b", Name: "Llama 3.1 8B", Description: "Previous generation"},
			{ID: "mistral:7b", Name: "Mistral 7B", Description: "Alternative model"},
			{ID: "gemma2:9b", Name: "Gemma 2 9B", Description: "Google's open model"},
			{ID: "qwen3:4b", Name: "Qwen 3 4B", Description: "Alibaba's model"},
			{ID: "deepseek-r1:8b", Name: "DeepSeek R1 8B", Description: "Deepseek's model"},
		},
		ProviderGemini: {
			{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", Description: "Latest preview"},
			{ID: "gemini-2.5-flash-preview-09-2025", Name: "Gemini 2.5 Flash Preview", Description: "2.5 Flash preview"},
		},
		ProviderOpenAI: {
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Description: "Fast, cost-effective (default)"},
			{ID: "gpt-4o", Name: "GPT-4o", Description: "Best quality, higher cost"},
			{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Description: "Previous flagship"},
			{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", Description: "Legacy, cheapest"},
		},
		ProviderAnthropic: {
			{ID: "claude-3-5-haiku-latest", Name: "Claude 3.5 Haiku", Description: "Fast, cost-effective (default)"},
			{ID: "claude-3-5-sonnet-latest", Name: "Claude 3.5 Sonnet", Description: "Best quality"},
			{ID: "claude-3-opus-latest", Name: "Claude 3 Opus", Description: "Most capable"},
		},
		ProviderXAI: {
			{ID: "grok-2", Name: "Grok 2", Description: "Fast, good quality (default)"},
			{ID: "grok-3", Name: "Grok 3", Description: "Latest model"},
			{ID: "grok-3-mini", Name: "Grok 3 Mini", Description: "Smaller, faster"},
		},
	}
}

// ModelInfo describes a model available for a provider
type ModelInfo struct {
	ID          string `json:"id"`          // Model identifier (e.g., "llama3.2:8b")
	Name        string `json:"name"`        // Display name (e.g., "Llama 3.2 8B")
	Description string `json:"description"` // Brief description
}

// ProviderInfo describes an LLM provider
type ProviderInfo struct {
	ID           string `json:"id"`           // Provider identifier (e.g., "ollama")
	Name         string `json:"name"`         // Display name (e.g., "Ollama (Local)")
	RequiresKey  bool   `json:"requiresKey"`  // Whether API key is required
	DefaultModel string `json:"defaultModel"` // Default model for this provider
}

// GetAvailableProviders returns information about all supported providers
func GetAvailableProviders() []ProviderInfo {
	return []ProviderInfo{
		{
			ID:           ProviderOllama,
			Name:         "Ollama (Local)",
			RequiresKey:  false,
			DefaultModel: DefaultOllamaModel,
		},
		{
			ID:           ProviderGemini,
			Name:         "Google Gemini",
			RequiresKey:  true,
			DefaultModel: DefaultGeminiModel,
		},
		{
			ID:           ProviderOpenAI,
			Name:         "OpenAI",
			RequiresKey:  true,
			DefaultModel: DefaultOpenAIModel,
		},
		{
			ID:           ProviderAnthropic,
			Name:         "Anthropic (Claude)",
			RequiresKey:  true,
			DefaultModel: DefaultAnthropicModel,
		},
		{
			ID:           ProviderXAI,
			Name:         "X.AI (Grok)",
			RequiresKey:  true,
			DefaultModel: DefaultXAIModel,
		},
	}
}
