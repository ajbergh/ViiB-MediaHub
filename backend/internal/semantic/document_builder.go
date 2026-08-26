package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	maxDocumentBytes       = 4 * 1024
	maxCommunityTags       = 12
	maxSimilarArtists      = 8
	maxArtistBioRunes      = 900
	maxTrackTitlesPerAlbum = 20
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// CanonicalArtistKey is a stable, whitespace-normalized identity. Entity type
// is stored separately, so a prefix is intentionally unnecessary.
func CanonicalArtistKey(artist string) string { return canonicalText(artist) }

// CanonicalAlbumKey identifies a normalized album artist and album pair. A
// NUL delimiter makes the pair unambiguous without relying on user text.
func CanonicalAlbumKey(albumArtist, album string) string {
	return canonicalText(albumArtist) + "\x00" + canonicalText(album)
}

// DocumentHash gates embedding work. Behavioural fields are excluded because
// callers build the content solely from stable semantic metadata.
func DocumentHash(content string) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(SemanticDocumentVersion) + "\x00" + normalizeDocumentText(content)))
	return hex.EncodeToString(sum[:])
}

// BuildDocuments deterministically creates artist, album, then track
// documents. This order lets the future background indexer make broad context
// useful before every individual track has been embedded.
func BuildDocuments(songs []db.Song, context DocumentContext) []Document {
	aggregates := buildAggregates(songs, context)
	docs := make([]Document, 0, len(songs)+len(aggregates.albums)+len(aggregates.artists))
	artistKeys := sortedKeys(aggregates.artists)
	for _, key := range artistKeys {
		docs = append(docs, buildArtistDocument(key, aggregates.artists[key], context.Artists[key]))
	}
	albumKeys := sortedKeys(aggregates.albums)
	for _, key := range albumKeys {
		docs = append(docs, buildAlbumDocument(key, aggregates.albums[key], context.Albums[key]))
	}
	sortedSongs := append([]db.Song(nil), songs...)
	sort.Slice(sortedSongs, func(i, j int) bool { return sortedSongKey(sortedSongs[i]) < sortedSongKey(sortedSongs[j]) })
	for _, song := range sortedSongs {
		if strings.TrimSpace(song.ID) == "" {
			continue
		}
		artist := aggregates.artists[CanonicalArtistKey(song.Artist)]
		albumArtist := song.AlbumArtist
		if strings.TrimSpace(albumArtist) == "" {
			albumArtist = song.Artist
		}
		album := aggregates.albums[CanonicalAlbumKey(albumArtist, song.Album)]
		docs = append(docs, buildTrackDocument(song, artist, album))
	}
	return docs
}

// BuildTrackDocument is exported for focused incremental indexing tests and
// call sites that have already assembled reusable aggregate context.
func BuildTrackDocument(song db.Song, artist ArtistContext, album AlbumContext) Document {
	return buildTrackDocument(song, artistAggregate{context: artist}, albumAggregate{context: album})
}

type artistAggregate struct {
	name       string
	genres     map[string]int
	tags       map[string]int
	context    ArtistContext
	yearCounts map[int]int
}

type albumAggregate struct {
	name       string
	artist     string
	genres     map[string]int
	tags       map[string]int
	moods      map[string]int
	energies   map[string]int
	tempos     map[string]int
	yearCounts map[int]int
	titles     map[string]struct{}
	vocals     int
	instrument int
	context    AlbumContext
}

type aggregates struct {
	artists map[string]artistAggregate
	albums  map[string]albumAggregate
}

func buildAggregates(songs []db.Song, context DocumentContext) aggregates {
	result := aggregates{artists: make(map[string]artistAggregate), albums: make(map[string]albumAggregate)}
	for _, song := range songs {
		artistName := cleanText(song.Artist)
		if artistName == "" {
			continue
		}
		artistKey := CanonicalArtistKey(artistName)
		artist := result.artists[artistKey]
		if artist.name == "" || artistName < artist.name {
			artist.name = artistName
		}
		if artist.genres == nil {
			artist.genres, artist.tags, artist.yearCounts = map[string]int{}, map[string]int{}, map[int]int{}
			artist.context = context.Artists[artistKey]
		}
		for _, genre := range normalizedValues(song.Genre) {
			artist.genres[genre]++
		}
		for _, tag := range parseTags(song.LastFMTags) {
			artist.tags[tag]++
		}
		if year := semanticYear(song); year > 0 {
			artist.yearCounts[year]++
		}
		result.artists[artistKey] = artist

		albumArtist := cleanText(song.AlbumArtist)
		if albumArtist == "" {
			albumArtist = artistName
		}
		albumName := cleanText(song.Album)
		if albumName == "" {
			continue
		}
		albumKey := CanonicalAlbumKey(albumArtist, albumName)
		album := result.albums[albumKey]
		if album.name == "" || albumName < album.name {
			album.name = albumName
		}
		if album.artist == "" || albumArtist < album.artist {
			album.artist = albumArtist
		}
		if album.genres == nil {
			album.genres, album.tags, album.moods, album.energies, album.tempos = map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
			album.yearCounts, album.titles = map[int]int{}, map[string]struct{}{}
			album.context = context.Albums[albumKey]
		}
		for _, genre := range normalizedValues(song.Genre) {
			album.genres[genre]++
		}
		for _, tag := range parseTags(song.LastFMTags) {
			album.tags[tag]++
		}
		for _, value := range []struct {
			value string
			into  map[string]int
		}{{song.Mood, album.moods}, {song.Energy, album.energies}, {song.Tempo, album.tempos}} {
			if cleaned := canonicalText(value.value); cleaned != "" {
				value.into[cleaned]++
			}
		}
		if year := semanticYear(song); year > 0 {
			album.yearCounts[year]++
		}
		if title := cleanText(song.Title); title != "" {
			album.titles[title] = struct{}{}
		}
		if song.Instrumental {
			album.instrument++
		} else {
			album.vocals++
		}
		result.albums[albumKey] = album
	}
	return result
}

func buildArtistDocument(key string, aggregate artistAggregate, supplied ArtistContext) Document {
	context := mergeArtistContext(aggregate.context, supplied)
	lines := []string{sentence("Artist", aggregate.name)}
	if values := rankedValues(aggregate.genres, maxCommunityTags); len(values) > 0 {
		lines = append(lines, sentence("Genres and styles", strings.Join(values, ", ")))
	}
	tags := mergeValues(rankedValues(aggregate.tags, maxCommunityTags), context.Tags)
	if len(tags) > 0 {
		lines = append(lines, sentence("Community tags", strings.Join(limitValues(tags, maxCommunityTags), ", ")))
	}
	if bio := cleanBio(context.Bio); bio != "" {
		lines = append(lines, sentence("Artist context", bio))
	}
	if similar := limitValues(normalizedValues(context.SimilarArtists), maxSimilarArtists); len(similar) > 0 {
		lines = append(lines, sentence("Similar artists", strings.Join(similar, ", ")))
	}
	if years := cleanText(context.ActiveYears); years != "" {
		lines = append(lines, sentence("Active years", years))
	}
	content := finishDocument(lines)
	return newDocument(db.SemanticEntityArtist, key, aggregate.name, "", aggregate.name, "", content)
}

func buildAlbumDocument(key string, aggregate albumAggregate, supplied AlbumContext) Document {
	context := mergeAlbumContext(aggregate.context, supplied)
	lines := []string{sentence("Album", aggregate.name), sentence("Album artist", aggregate.artist)}
	if years := formatYears(aggregate.yearCounts); years != "" {
		lines = append(lines, sentence("Release years", years))
	}
	if values := rankedValues(aggregate.genres, maxCommunityTags); len(values) > 0 {
		lines = append(lines, sentence("Genres and styles", strings.Join(values, ", ")))
	}
	tags := mergeValues(rankedValues(aggregate.tags, maxCommunityTags), context.Tags)
	if len(tags) > 0 {
		lines = append(lines, sentence("Community tags", strings.Join(limitValues(tags, maxCommunityTags), ", ")))
	}
	for _, distribution := range []struct {
		label  string
		values map[string]int
	}{{"Mood", aggregate.moods}, {"Energy", aggregate.energies}, {"Tempo", aggregate.tempos}} {
		if values := rankedValues(distribution.values, 3); len(values) > 0 {
			lines = append(lines, sentence(distribution.label, strings.Join(values, ", ")))
		}
	}
	if aggregate.instrument > 0 || aggregate.vocals > 0 {
		voice := "vocal"
		if aggregate.instrument > aggregate.vocals {
			voice = "mostly instrumental"
		}
		lines = append(lines, sentence("Vocals", voice))
	}
	if titles := sortedSet(aggregate.titles, maxTrackTitlesPerAlbum); len(titles) > 0 {
		lines = append(lines, sentence("Tracks", strings.Join(titles, ", ")))
	}
	content := finishDocument(lines)
	return newDocument(db.SemanticEntityAlbum, key, aggregate.name, "", aggregate.artist, aggregate.name, content)
}

func buildTrackDocument(song db.Song, artist artistAggregate, album albumAggregate) Document {
	artistName := cleanText(song.Artist)
	albumName := cleanText(song.Album)
	albumArtist := cleanText(song.AlbumArtist)
	if albumArtist == "" {
		albumArtist = artistName
	}
	lines := []string{sentence("Track", cleanText(song.Title)), sentence("Artist", artistName)}
	if albumName != "" {
		lines = append(lines, sentence("Album", albumName))
	}
	if albumArtist != "" {
		lines = append(lines, sentence("Album artist", albumArtist))
	}
	if year := semanticYear(song); year > 0 {
		lines = append(lines, sentence("Original year", strconv.Itoa(year)))
	}
	if values := normalizedValues(song.Genre); len(values) > 0 {
		lines = append(lines, sentence("Genres and styles", strings.Join(values, ", ")))
	}
	if tags := parseTags(song.LastFMTags); len(tags) > 0 {
		lines = append(lines, sentence("Community tags", strings.Join(limitValues(tags, maxCommunityTags), ", ")))
	}
	for _, attribute := range []struct{ label, value string }{{"Mood", song.Mood}, {"Energy", song.Energy}, {"Tempo", song.Tempo}} {
		if value := cleanText(attribute.value); value != "" {
			lines = append(lines, sentence(attribute.label, value))
		}
	}
	if song.BPM > 0 {
		lines = append(lines, sentence("BPM", strconv.Itoa(song.BPM)))
	}
	voice := "vocal"
	if song.Instrumental {
		voice = "instrumental"
	}
	lines = append(lines, sentence("Vocals", voice))
	artistContext := compactArtistContext(artist)
	if artistContext != "" {
		lines = append(lines, sentence("Artist context", artistContext))
	}
	albumContext := compactAlbumContext(album)
	if albumContext != "" {
		lines = append(lines, sentence("Album context", albumContext))
	}
	content := finishDocument(lines)
	return newDocument(db.SemanticEntityTrack, song.ID, cleanText(song.Title), song.ID, artistName, albumName, content)
}

func newDocument(entityType, entityKey, displayName, songID, artist, album, content string) Document {
	content = finishDocument([]string{content})
	return Document{EntityType: entityType, EntityKey: entityKey, DisplayName: displayName, SongID: songID, Artist: artist, Album: album, Content: content, ContentHash: DocumentHash(content), DocumentVersion: SemanticDocumentVersion, Status: "pending"}
}

func compactArtistContext(artist artistAggregate) string {
	parts := make([]string, 0, 3)
	if genres := rankedValues(artist.genres, 5); len(genres) > 0 {
		parts = append(parts, strings.Join(genres, ", "))
	}
	if tags := mergeValues(rankedValues(artist.tags, 5), artist.context.Tags); len(tags) > 0 {
		parts = append(parts, strings.Join(limitValues(tags, 5), ", "))
	}
	if similar := limitValues(normalizedValues(artist.context.SimilarArtists), 4); len(similar) > 0 {
		parts = append(parts, "similar artists include "+strings.Join(similar, ", "))
	}
	return strings.Join(parts, "; ")
}

func compactAlbumContext(album albumAggregate) string {
	parts := make([]string, 0, 3)
	if years := formatYears(album.yearCounts); years != "" {
		parts = append(parts, years)
	}
	if genres := rankedValues(album.genres, 5); len(genres) > 0 {
		parts = append(parts, strings.Join(genres, "/"))
	}
	if tags := mergeValues(rankedValues(album.tags, 5), album.context.Tags); len(tags) > 0 {
		parts = append(parts, "common tags include "+strings.Join(limitValues(tags, 5), ", "))
	}
	return strings.Join(parts, "; ")
}

func mergeArtistContext(base, supplied ArtistContext) ArtistContext {
	return ArtistContext{Tags: mergeValues(base.Tags, supplied.Tags), Bio: firstNonEmpty(supplied.Bio, base.Bio), SimilarArtists: mergeValues(base.SimilarArtists, supplied.SimilarArtists), ActiveYears: firstNonEmpty(supplied.ActiveYears, base.ActiveYears)}
}

func mergeAlbumContext(base, supplied AlbumContext) AlbumContext {
	return AlbumContext{Tags: mergeValues(base.Tags, supplied.Tags)}
}

func semanticYear(song db.Song) int {
	if song.OriginalYear > 0 {
		return song.OriginalYear
	}
	return song.Year
}

func sortedSongKey(song db.Song) string {
	return CanonicalArtistKey(song.Artist) + "\x00" + CanonicalAlbumKey(song.AlbumArtist, song.Album) + "\x00" + canonicalText(song.Title) + "\x00" + song.ID
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sentence(label, value string) string {
	value = cleanText(value)
	if value == "" {
		return ""
	}
	return label + ": " + value + "."
}

func finishDocument(lines []string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			filtered = append(filtered, line)
		}
	}
	return truncateUTF8(normalizeDocumentText(strings.Join(filtered, "\n")), maxDocumentBytes)
}

func normalizeDocumentText(value string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(value, "\u00a0", " ")), " "))
}

func cleanText(value string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}

func canonicalText(value string) string { return strings.ToLower(cleanText(value)) }

func normalizedValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = canonicalText(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func parseTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var stringsOnly []string
	if err := json.Unmarshal([]byte(raw), &stringsOnly); err == nil {
		return normalizedValues(stringsOnly)
	}
	var mixed []any
	if err := json.Unmarshal([]byte(raw), &mixed); err != nil {
		return nil
	}
	values := make([]string, 0, len(mixed))
	for _, item := range mixed {
		switch value := item.(type) {
		case string:
			values = append(values, value)
		case map[string]any:
			if name, ok := value["name"].(string); ok {
				values = append(values, name)
			}
		}
	}
	return normalizedValues(values)
}

func rankedValues(counts map[string]int, limit int) []string {
	type pair struct {
		value string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for value, count := range counts {
		if value = canonicalText(value); value != "" && count > 0 {
			pairs = append(pairs, pair{value, count})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].count > pairs[j].count
	})
	values := make([]string, 0, min(limit, len(pairs)))
	for _, pair := range pairs {
		values = append(values, pair.value)
		if len(values) == limit {
			break
		}
	}
	return values
}

func mergeValues(groups ...[]string) []string {
	values := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, value := range group {
			value = canonicalText(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				values = append(values, value)
			}
		}
	}
	return values
}

func limitValues(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func sortedSet(values map[string]struct{}, limit int) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value = cleanText(value); value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return limitValues(result, limit)
}

func formatYears(years map[int]int) string {
	values := make([]int, 0, len(years))
	for year := range years {
		if year > 0 {
			values = append(values, year)
		}
	}
	sort.Ints(values)
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return strconv.Itoa(values[0])
	}
	return fmt.Sprintf("%d–%d", values[0], values[len(values)-1])
}

func cleanBio(value string) string {
	value = html.UnescapeString(htmlTagPattern.ReplaceAllString(value, " "))
	value = cleanText(value)
	if utf8RuneCount(value) > maxArtistBioRunes {
		value = truncateRunes(value, maxArtistBioRunes)
	}
	return value
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	cut := maximum
	for cut > 0 && (value[cut]&0xC0) == 0x80 {
		cut--
	}
	return strings.TrimSpace(value[:cut])
}

func utf8RuneCount(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}

func truncateRunes(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	count := 0
	for index := range value {
		if count == maximum {
			return strings.TrimSpace(value[:index])
		}
		count++
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = cleanText(value); value != "" {
			return value
		}
	}
	return ""
}

// Kept here so the compiler protects the intent that canonicalization handles
// whitespace only, not arbitrary transliteration or lossy punctuation removal.
var _ = unicode.IsSpace
