package semantic

import (
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestBuildDocumentsDeterministicAndRich(t *testing.T) {
	songs := []db.Song{
		{ID: "song-2", Title: "Second Song", Artist: "The Example", Album: "An Album", AlbumArtist: "The Example", Genre: []string{"Alternative Rock", " Art Rock "}, Year: 1998, LastFMTags: `["Melancholic", "atmospheric", "alternative"]`, Mood: "reflective", Energy: "medium", Tempo: "medium", BPM: 103},
		{ID: "song-1", Title: "First Song", Artist: "The Example", Album: "An Album", AlbumArtist: "The Example", Genre: []string{"Art Rock"}, OriginalYear: 1997, Instrumental: true},
	}
	context := DocumentContext{
		Artists: map[string]ArtistContext{CanonicalArtistKey("The Example"): {Tags: []string{"British"}, Bio: "<p>A  celebrated\nartist.</p>", SimilarArtists: []string{"Peer One", "Peer Two"}, ActiveYears: "1990s–present"}},
		Albums:  map[string]AlbumContext{CanonicalAlbumKey("The Example", "An Album"): {Tags: []string{"classic"}}},
	}
	docs := BuildDocuments(songs, context)
	if len(docs) != 4 {
		t.Fatalf("document count = %d, want 4", len(docs))
	}
	if docs[0].EntityType != db.SemanticEntityArtist || docs[1].EntityType != db.SemanticEntityAlbum {
		t.Fatalf("document order = %q, %q; artist then album required", docs[0].EntityType, docs[1].EntityType)
	}
	track := documentFor(t, docs, db.SemanticEntityTrack, "song-2")
	for _, expected := range []string{"Track: Second Song.", "Original year: 1998.", "Genres and styles: alternative rock, art rock.", "Community tags: alternative, atmospheric, melancholic.", "Artist context: art rock, alternative rock; alternative, atmospheric, melancholic, british; similar artists include peer one, peer two.", "Album context: 1997–1998; art rock/alternative rock; common tags include alternative, atmospheric, melancholic, classic."} {
		if !strings.Contains(track.Content, expected) {
			t.Errorf("track document missing %q: %s", expected, track.Content)
		}
	}
	if strings.Contains(track.Content, "song-2") {
		t.Fatalf("track document leaked ViiB song identity: %s", track.Content)
	}
	if strings.Contains(track.Content, "search_document:") {
		t.Fatalf("provider task prefix belongs outside stored content: %s", track.Content)
	}

	reversed := BuildDocuments([]db.Song{songs[1], songs[0]}, context)
	if len(reversed) != len(docs) {
		t.Fatal("reordered input changed document count")
	}
	for index := range docs {
		if docs[index].EntityType != reversed[index].EntityType || docs[index].EntityKey != reversed[index].EntityKey || docs[index].ContentHash != reversed[index].ContentHash {
			t.Fatalf("document %d is not deterministic: %#v != %#v", index, docs[index], reversed[index])
		}
	}
}

func TestTrackDocumentIgnoresVolatileBehaviour(t *testing.T) {
	base := db.Song{ID: "stable", Title: "Stable", Artist: "Artist", Album: "Album", Genre: []string{"Rock"}, Year: 2001}
	baseline := documentFor(t, BuildDocuments([]db.Song{base}, DocumentContext{}), db.SemanticEntityTrack, base.ID).ContentHash
	changes := []func(*db.Song){
		func(song *db.Song) { song.PlayCount = 99 },
		func(song *db.Song) { song.LastPlayed = 123456789 },
		func(song *db.Song) { song.SkipCount = 11 },
		func(song *db.Song) { song.Liked = true },
	}
	for index, change := range changes {
		candidate := base
		change(&candidate)
		actual := documentFor(t, BuildDocuments([]db.Song{candidate}, DocumentContext{}), db.SemanticEntityTrack, base.ID).ContentHash
		if actual != baseline {
			t.Fatalf("volatile change %d changed hash: %s != %s", index, actual, baseline)
		}
	}
}

func TestDocumentBuilderToleratesMalformedTagsAndCapsContent(t *testing.T) {
	longBio := strings.Repeat("very long artist context ", 1000)
	song := db.Song{ID: "id", Title: "Title", Artist: "Artist", Album: "Album", LastFMTags: `{broken json}`}
	docs := BuildDocuments([]db.Song{song}, DocumentContext{Artists: map[string]ArtistContext{CanonicalArtistKey(song.Artist): {Bio: longBio}}})
	track := documentFor(t, docs, db.SemanticEntityTrack, song.ID)
	if strings.Contains(track.Content, "Community tags") {
		t.Fatalf("malformed tags should be ignored: %s", track.Content)
	}
	artist := documentFor(t, docs, db.SemanticEntityArtist, CanonicalArtistKey(song.Artist))
	if len(artist.Content) > maxDocumentBytes {
		t.Fatalf("artist content length = %d, cap = %d", len(artist.Content), maxDocumentBytes)
	}
}

func TestDocumentHashIncludesVersionAndNormalizedText(t *testing.T) {
	if got, want := DocumentHash(" One\n  Two "), DocumentHash("One Two"); got != want {
		t.Fatalf("normalized document hashes differ: %s != %s", got, want)
	}
	if DocumentHash("one") == DocumentHash("two") {
		t.Fatal("different semantic content has the same hash")
	}
}

func documentFor(t *testing.T, docs []Document, entityType, key string) Document {
	t.Helper()
	for _, doc := range docs {
		if doc.EntityType == entityType && doc.EntityKey == key {
			return doc
		}
	}
	t.Fatalf("document %s/%q not found", entityType, key)
	return Document{}
}
