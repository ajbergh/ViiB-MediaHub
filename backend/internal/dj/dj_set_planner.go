package dj

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// ============================================================================
// Planner Interface
// ============================================================================

// Planner defines the interface for building DJ set plans.
type Planner interface {
	BuildPlan(ctx context.Context, prompt string, opts PlanOptions, libContext LibraryContext) (*DJSetPlan, error)
}

// LibraryContext provides library information for plan generation.
type LibraryContext struct {
	AvailableGenres  []string // All indexed genres in the library
	SeedGenres       []string // Matched genres from filter
	SeedArtists      []string // Matched artists from filter
	AvgSongLengthSec int      // Average song duration in library
	TotalSongs       int      // Total songs in library
}

// ============================================================================
// LLM Planner Implementation
// ============================================================================

// LLMPlanner uses an LLM provider to generate DJ set plans.
type LLMPlanner struct {
	provider *llm.Provider
	cache    *planCache
}

// NewLLMPlanner creates a new LLM-backed planner.
// Returns nil provider on error (caller should check before use).
func NewLLMPlanner(settings llm.Settings) *LLMPlanner {
	provider, err := llm.NewProvider(settings)
	if err != nil {
		logger.API("DJ Planner: Failed to create LLM provider: %v", err)
		return &LLMPlanner{
			provider: nil,
			cache:    newPlanCache(PlanCacheTTLMinutes * time.Minute),
		}
	}
	return &LLMPlanner{
		provider: provider,
		cache:    newPlanCache(PlanCacheTTLMinutes * time.Minute),
	}
}

// BuildPlan generates a DJ set plan using the LLM.
func (p *LLMPlanner) BuildPlan(ctx context.Context, prompt string, opts PlanOptions, libContext LibraryContext) (*DJSetPlan, error) {
	// Calculate target song count
	avgLen := libContext.AvgSongLengthSec
	if avgLen <= 0 {
		avgLen = DefaultAvgSongLengthSec
	}
	targetSongs := CalculateTargetSongCount(opts.TargetDurationMin, avgLen)

	// Generate cache key
	cacheKey := p.buildCacheKey(prompt, opts, libContext)

	// Check cache first
	if cached := p.cache.get(cacheKey); cached != nil {
		cached.FromCache = true
		return cached, nil
	}

	// Build the user prompt from template
	userPrompt := p.buildUserPrompt(prompt, opts, libContext, targetSongs)

	// Call LLM
	response, err := p.provider.Generate(ctx, llm.DJSetPlanSystemPrompt, userPrompt)
	if err != nil {
		logger.API("DJ Planner LLM error: %v, falling back to default plan", err)
		return p.buildDefaultPlan(prompt, opts, libContext, targetSongs), nil
	}

	// Parse and validate the response
	plan, err := p.parseAndValidatePlan(response, opts, libContext, targetSongs)
	if err != nil {
		logger.API("DJ Planner parse error: %v, falling back to default plan", err)
		return p.buildDefaultPlan(prompt, opts, libContext, targetSongs), nil
	}

	// Store in cache
	p.cache.set(cacheKey, plan)

	return plan, nil
}

// buildCacheKey generates a unique cache key for the plan request.
func (p *LLMPlanner) buildCacheKey(prompt string, opts PlanOptions, libContext LibraryContext) string {
	normalized := NormalizePrompt(prompt)
	genresHash := HashGenres(libContext.SeedGenres)

	key := fmt.Sprintf("%s|%s|%d|%d|%s",
		normalized,
		opts.Persona,
		opts.TargetDurationMin,
		opts.FlowStrictness,
		genresHash,
	)

	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes
}

// buildUserPrompt constructs the user prompt from the template.
func (p *LLMPlanner) buildUserPrompt(prompt string, opts PlanOptions, libContext LibraryContext, targetSongs int) string {
	// Build time context if enabled
	timeContext := ""
	if opts.UseTimeContext {
		hour := time.Now().Hour()
		switch {
		case hour >= 5 && hour < 12:
			timeContext = "Time context: Morning (energizing start)"
		case hour >= 12 && hour < 17:
			timeContext = "Time context: Afternoon (steady energy)"
		case hour >= 17 && hour < 21:
			timeContext = "Time context: Evening (building energy)"
		case hour >= 21 || hour < 5:
			timeContext = "Time context: Night (peak energy or late night chill)"
		}
	}

	// Format genres
	availableGenres := strings.Join(libContext.AvailableGenres, ", ")
	if len(availableGenres) > 500 {
		// Truncate if too long
		availableGenres = availableGenres[:500] + "..."
	}
	seedGenres := strings.Join(libContext.SeedGenres, ", ")
	seedArtists := strings.Join(libContext.SeedArtists, ", ")

	// Replace placeholders
	userPrompt := llm.DJSetPlanUserPromptTemplate
	userPrompt = strings.ReplaceAll(userPrompt, "{{PROMPT}}", prompt)
	userPrompt = strings.ReplaceAll(userPrompt, "{{PERSONA}}", opts.Persona)
	userPrompt = strings.ReplaceAll(userPrompt, "{{DURATION}}", fmt.Sprintf("%d", opts.TargetDurationMin))
	userPrompt = strings.ReplaceAll(userPrompt, "{{TARGET_SONGS}}", fmt.Sprintf("%d", targetSongs))
	userPrompt = strings.ReplaceAll(userPrompt, "{{FLOW}}", fmt.Sprintf("%d", opts.FlowStrictness))
	userPrompt = strings.ReplaceAll(userPrompt, "{{TIME_CONTEXT}}", timeContext)
	userPrompt = strings.ReplaceAll(userPrompt, "{{GENRES}}", availableGenres)
	userPrompt = strings.ReplaceAll(userPrompt, "{{SEED_GENRES}}", seedGenres)
	userPrompt = strings.ReplaceAll(userPrompt, "{{SEED_ARTISTS}}", seedArtists)

	return userPrompt
}

// parseAndValidatePlan parses the LLM response and validates the plan.
func (p *LLMPlanner) parseAndValidatePlan(response string, opts PlanOptions, libContext LibraryContext, targetSongs int) (*DJSetPlan, error) {
	// Extract JSON from response (handle potential markdown wrapping)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	// Parse the JSON
	var rawPlan struct {
		IntentSummary string `json:"intentSummary"`
		Phases        []struct {
			Name         string   `json:"name"`
			TargetEnergy string   `json:"targetEnergy"`
			TargetTempo  string   `json:"targetTempo"`
			TargetMoods  []string `json:"targetMoods"`
			TargetCount  int      `json:"targetCount"`
			MinBPM       int      `json:"minBPM"`
			MaxBPM       int      `json:"maxBPM"`
			Notes        string   `json:"notes"`
		} `json:"phases"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawPlan); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate phase count
	if len(rawPlan.Phases) < MinPhaseCount || len(rawPlan.Phases) > MaxPhaseCount {
		return nil, fmt.Errorf("invalid phase count: %d (expected %d-%d)", len(rawPlan.Phases), MinPhaseCount, MaxPhaseCount)
	}

	// Build the plan with validation
	plan := &DJSetPlan{
		IntentSummary:     rawPlan.IntentSummary,
		TargetDurationMin: opts.TargetDurationMin,
		Persona:           opts.Persona,
		FlowStrictness:    opts.FlowStrictness,
		SeedGenres:        libContext.SeedGenres,
		SeedArtists:       libContext.SeedArtists,
		CreatedAtUnix:     time.Now().Unix(),
		FromCache:         false,
	}

	totalCount := 0
	for _, rp := range rawPlan.Phases {
		phase := DJPhase{
			Name:         validatePhaseName(rp.Name),
			TargetEnergy: validateEnergy(rp.TargetEnergy),
			TargetTempo:  validateTempo(rp.TargetTempo),
			TargetMoods:  rp.TargetMoods,
			TargetCount:  rp.TargetCount,
			MinBPM:       ClampBPM(rp.MinBPM),
			MaxBPM:       ClampBPM(rp.MaxBPM),
			Notes:        rp.Notes,
		}

		// Ensure MinBPM < MaxBPM
		if phase.MinBPM >= phase.MaxBPM {
			phase.MaxBPM = phase.MinBPM + 20
		}

		// Ensure reasonable count
		if phase.TargetCount < 1 {
			phase.TargetCount = 1
		}

		plan.Phases = append(plan.Phases, phase)
		totalCount += phase.TargetCount
	}

	// Adjust counts if they don't sum to target
	if totalCount != targetSongs {
		plan.Phases = adjustPhaseCounts(plan.Phases, targetSongs)
	}

	return plan, nil
}

// buildDefaultPlan creates a deterministic fallback plan when LLM fails.
func (p *LLMPlanner) buildDefaultPlan(prompt string, opts PlanOptions, libContext LibraryContext, targetSongs int) *DJSetPlan {
	// Determine vibe from prompt keywords
	promptLower := strings.ToLower(prompt)
	isEnergetic := strings.Contains(promptLower, "party") || strings.Contains(promptLower, "dance") || strings.Contains(promptLower, "workout")
	isChill := strings.Contains(promptLower, "chill") || strings.Contains(promptLower, "relax") || strings.Contains(promptLower, "calm")

	// Time-based adjustment
	hour := time.Now().Hour()
	isNight := hour >= 21 || hour < 5

	var phases []DJPhase

	if isChill || (isNight && !isEnergetic) {
		// Chill set structure
		phases = []DJPhase{
			{Name: PhaseWarmUp, TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetMoods: []string{"calm"}, MinBPM: 70, MaxBPM: 95, Notes: "Easing into the vibe"},
			{Name: PhaseBuild, TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"peaceful"}, MinBPM: 90, MaxBPM: 115, Notes: "Finding the groove"},
			{Name: PhasePeak, TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"dreamy"}, MinBPM: 100, MaxBPM: 125, Notes: "Smooth sailing"},
			{Name: PhaseCooldown, TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetMoods: []string{"nostalgic"}, MinBPM: 70, MaxBPM: 100, Notes: "Winding down"},
		}
	} else if isEnergetic {
		// High energy set structure
		phases = []DJPhase{
			{Name: PhaseWarmUp, TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"uplifting"}, MinBPM: 100, MaxBPM: 120, Notes: "Getting the energy up"},
			{Name: PhaseBuild, TargetEnergy: EnergyHigh, TargetTempo: TempoFast, TargetMoods: []string{"energetic"}, MinBPM: 115, MaxBPM: 140, Notes: "Building momentum"},
			{Name: PhasePeak, TargetEnergy: EnergyHigh, TargetTempo: TempoFast, TargetMoods: []string{"intense"}, MinBPM: 130, MaxBPM: 160, Notes: "Maximum intensity"},
			{Name: PhaseCooldown, TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"happy"}, MinBPM: 110, MaxBPM: 130, Notes: "Cooling off"},
		}
	} else {
		// Balanced set structure (default)
		phases = []DJPhase{
			{Name: PhaseWarmUp, TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetMoods: []string{"calm", "nostalgic"}, MinBPM: 80, MaxBPM: 105, Notes: "Setting the mood"},
			{Name: PhaseBuild, TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"uplifting"}, MinBPM: 100, MaxBPM: 125, Notes: "Building energy"},
			{Name: PhasePeak, TargetEnergy: EnergyHigh, TargetTempo: TempoFast, TargetMoods: []string{"energetic", "happy"}, MinBPM: 120, MaxBPM: 145, Notes: "Peak energy"},
			{Name: PhaseCooldown, TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetMoods: []string{"peaceful"}, MinBPM: 80, MaxBPM: 110, Notes: "Gentle landing"},
		}
	}

	// Distribute song counts
	phases = adjustPhaseCounts(phases, targetSongs)

	return &DJSetPlan{
		IntentSummary:     fmt.Sprintf("A curated %d-minute set based on: %s", opts.TargetDurationMin, prompt),
		TargetDurationMin: opts.TargetDurationMin,
		Persona:           opts.Persona,
		FlowStrictness:    opts.FlowStrictness,
		Phases:            phases,
		SeedGenres:        libContext.SeedGenres,
		SeedArtists:       libContext.SeedArtists,
		CreatedAtUnix:     time.Now().Unix(),
		FromCache:         false,
	}
}

// ============================================================================
// Plan Cache
// ============================================================================

type planCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	plan      *DJSetPlan
	expiresAt time.Time
}

func newPlanCache(ttl time.Duration) *planCache {
	cache := &planCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
	// Start cleanup goroutine
	go cache.cleanupLoop()
	return cache
}

func (c *planCache) get(key string) *DJSetPlan {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		return nil
	}

	// Return a copy to avoid mutation
	planCopy := *entry.plan
	return &planCopy
}

func (c *planCache) set(key string, plan *DJSetPlan) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		plan:      plan,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *planCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		c.cleanup()
	}
}

func (c *planCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// ============================================================================
// Narration Generator
// ============================================================================

// GenerateNarration generates DJ talk mode narration for a plan.
func (p *LLMPlanner) GenerateNarration(ctx context.Context, plan *DJSetPlan) (*DJNarration, error) {
	if plan == nil || len(plan.Phases) == 0 {
		return nil, fmt.Errorf("plan is required for narration")
	}

	// Build phases description
	var phaseDescs []string
	for _, phase := range plan.Phases {
		phaseDescs = append(phaseDescs, fmt.Sprintf("%s (%s energy, %s tempo)", phase.Name, phase.TargetEnergy, phase.TargetTempo))
	}
	phasesStr := strings.Join(phaseDescs, " → ")

	// Build user prompt
	userPrompt := llm.DJNarrationUserPromptTemplate
	userPrompt = strings.ReplaceAll(userPrompt, "{{INTENT_SUMMARY}}", plan.IntentSummary)
	userPrompt = strings.ReplaceAll(userPrompt, "{{PERSONA}}", plan.Persona)
	userPrompt = strings.ReplaceAll(userPrompt, "{{PHASES}}", phasesStr)
	userPrompt = strings.ReplaceAll(userPrompt, "{{PHASE_COUNT}}", fmt.Sprintf("%d", len(plan.Phases)))
	userPrompt = strings.ReplaceAll(userPrompt, "{{TRANSITION_COUNT}}", fmt.Sprintf("%d", len(plan.Phases)-1))

	// Call LLM
	response, err := p.provider.Generate(ctx, llm.DJNarrationSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("narration generation failed: %w", err)
	}

	// Parse response
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON in narration response")
	}

	var narration DJNarration
	if err := json.Unmarshal([]byte(jsonStr), &narration); err != nil {
		return nil, fmt.Errorf("failed to parse narration: %w", err)
	}

	return &narration, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// NormalizePrompt normalizes a prompt for cache key generation.
func NormalizePrompt(prompt string) string {
	// Lowercase
	prompt = strings.ToLower(prompt)
	// Remove extra whitespace
	prompt = strings.TrimSpace(prompt)
	spaceRegex := regexp.MustCompile(`\s+`)
	prompt = spaceRegex.ReplaceAllString(prompt, " ")
	// Remove punctuation for normalization
	punctRegex := regexp.MustCompile(`[^\w\s]`)
	prompt = punctRegex.ReplaceAllString(prompt, "")
	return prompt
}

// HashGenres creates a short hash of a genre list for cache keys.
func HashGenres(genres []string) string {
	if len(genres) == 0 {
		return "empty"
	}
	sorted := make([]string, len(genres))
	copy(sorted, genres)
	sort.Strings(sorted)
	combined := strings.Join(sorted, "|")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:8])
}

// extractJSON extracts JSON from a potentially markdown-wrapped response.
func extractJSON(response string) string {
	response = strings.TrimSpace(response)

	// Check if response is already valid JSON
	if strings.HasPrefix(response, "{") && strings.HasSuffix(response, "}") {
		return response
	}

	// Try to extract from markdown code block
	jsonRegex := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.+?)\\n?```")
	matches := jsonRegex.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try to find JSON object in response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end > start {
		return response[start : end+1]
	}

	return ""
}

// validatePhaseName ensures a phase name is valid.
func validatePhaseName(name string) string {
	validNames := ValidPhaseNames()
	nameLower := strings.ToLower(strings.TrimSpace(name))

	for _, valid := range validNames {
		if strings.ToLower(valid) == nameLower {
			return valid
		}
	}

	// Default to Build if unknown
	return PhaseBuild
}

// validateEnergy ensures an energy level is valid.
func validateEnergy(energy string) string {
	switch strings.ToLower(strings.TrimSpace(energy)) {
	case "low":
		return EnergyLow
	case "high":
		return EnergyHigh
	default:
		return EnergyMedium
	}
}

// validateTempo ensures a tempo is valid.
func validateTempo(tempo string) string {
	switch strings.ToLower(strings.TrimSpace(tempo)) {
	case "slow":
		return TempoSlow
	case "fast":
		return TempoFast
	default:
		return TempoMedium
	}
}

// adjustPhaseCounts redistributes song counts to match target total.
func adjustPhaseCounts(phases []DJPhase, targetTotal int) []DJPhase {
	if len(phases) == 0 || targetTotal < 1 {
		return phases
	}

	// Use default weights for distribution
	weights := []float64{0.20, 0.25, 0.30, 0.25} // Warm-up, Build, Peak, Cooldown
	if len(phases) == 3 {
		weights = []float64{0.25, 0.45, 0.30}
	} else if len(phases) == 5 {
		weights = []float64{0.15, 0.20, 0.30, 0.20, 0.15}
	}

	// Distribute counts
	remaining := targetTotal
	for i := range phases {
		if i < len(weights) {
			phases[i].TargetCount = int(float64(targetTotal) * weights[i])
		} else {
			phases[i].TargetCount = targetTotal / len(phases)
		}
		if phases[i].TargetCount < 1 {
			phases[i].TargetCount = 1
		}
		remaining -= phases[i].TargetCount
	}

	// Distribute any remaining songs to the peak phase (or last phase)
	peakIdx := len(phases) / 2
	if peakIdx < len(phases) && remaining > 0 {
		phases[peakIdx].TargetCount += remaining
	}

	return phases
}
