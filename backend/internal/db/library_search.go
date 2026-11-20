// library_search.go searches the persisted SQLite metadata index.
package db

import (
	"context"
	"strings"
	"unicode/utf8"
)

func searchPrefixBounds(query string) (string, string) {
	return query, query + string(rune(0x10ffff))
}

func fts5TrigramPhrase(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// SearchLibrary uses indexed prefix matches for titles and albums, and indexed
// trigram FTS5 (with 1-2 char instr fallback) for artist substring matching.
func (d *DB) SearchLibrary(query string, limit int) (LibrarySearchResult, error) {
	return d.SearchLibraryContext(context.Background(), query, limit)
}

// SearchLibraryContext searches the library index while respecting context cancellation.
func (d *DB) SearchLibraryContext(ctx context.Context, query string, limit int) (LibrarySearchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query = strings.TrimSpace(strings.ToLower(query))
	limit = clampPageLimit(limit, 50, 200)
	result := LibrarySearchResult{
		Query: query, Tracks: []Song{}, Albums: []LibrarySearchAlbum{},
		Artists: []LibrarySearchArtist{}, Playlists: []LibrarySearchPlaylist{},
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if query == "" {
		return result, nil
	}
	if err := d.EnsureLibrarySyncSchema(); err != nil {
		return result, err
	}

	lower, upper := searchPrefixBounds(query)
	genreLower, genreUpper := searchPrefixBounds(`["` + query)
	useTrigramFTS := utf8.RuneCountInString(query) >= 3
	ftsPhrase := fts5TrigramPhrase(query)

	var trackQuery string
	var trackArgs []any
	if useTrigramFTS {
		trackQuery = `
		WITH matched_songs AS (
			SELECT song_id FROM song_search WHERE title >= ? AND title < ?
			UNION
			SELECT song_id FROM song_search WHERE album >= ? AND album < ?
			UNION
			SELECT song_id FROM song_search WHERE artist >= ? AND artist < ?
			UNION
			SELECT song_id FROM song_search WHERE album_artist >= ? AND album_artist < ?
			UNION
			SELECT song_id FROM song_search WHERE genre >= ? AND genre < ?
			UNION
			SELECT song_id FROM song_search WHERE file_path >= ? AND file_path < ?
			UNION
			SELECT song_id FROM song_artist_fts
			WHERE song_artist_fts MATCH ?
			  AND (instr(artist, ?) > 0 OR instr(album_artist, ?) > 0)
		)
		` + songSelect + `
		JOIN matched_songs m ON m.song_id = songs.id
		JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		ORDER BY CASE
			WHEN ss.title >= ? AND ss.title < ? THEN 0
			WHEN ss.artist >= ? AND ss.artist < ? THEN 1
			WHEN ss.album >= ? AND ss.album < ? THEN 2 ELSE 3 END,
			COALESCE(songs.play_count, 0) DESC, songs.title
		LIMIT ?`
		trackArgs = []any{
			lower, upper,
			lower, upper,
			lower, upper,
			lower, upper,
			genreLower, genreUpper,
			lower, upper,
			ftsPhrase, query, query,
			lower, upper, lower, upper, lower, upper, limit,
		}
	} else {
		trackQuery = `
		WITH matched_songs AS (
			SELECT song_id FROM song_search WHERE title >= ? AND title < ?
			UNION
			SELECT song_id FROM song_search WHERE album >= ? AND album < ?
			UNION
			SELECT song_id FROM song_search WHERE genre >= ? AND genre < ?
			UNION
			SELECT song_id FROM song_search WHERE file_path >= ? AND file_path < ?
			UNION
			SELECT song_id FROM song_search WHERE instr(artist, ?) > 0
			UNION
			SELECT song_id FROM song_search WHERE instr(album_artist, ?) > 0
		)
		` + songSelect + `
		JOIN matched_songs m ON m.song_id = songs.id
		JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		ORDER BY CASE
			WHEN ss.title >= ? AND ss.title < ? THEN 0
			WHEN ss.artist >= ? AND ss.artist < ? THEN 1
			WHEN ss.album >= ? AND ss.album < ? THEN 2 ELSE 3 END,
			COALESCE(songs.play_count, 0) DESC, songs.title
		LIMIT ?`
		trackArgs = []any{
			lower, upper,
			lower, upper,
			genreLower, genreUpper,
			lower, upper,
			query,
			query,
			lower, upper, lower, upper, lower, upper, limit,
		}
	}

	rows, err := d.conn.QueryContext(ctx, trackQuery, trackArgs...)
	if err != nil {
		return result, err
	}
	result.Tracks, err = scanLibrarySongRows(rows)
	if err != nil {
		return result, err
	}

	var albumQuery string
	var albumArgs []any
	if useTrigramFTS {
		albumQuery = `
		WITH matched_songs AS (
			SELECT song_id FROM song_search WHERE album >= ? AND album < ?
			UNION
			SELECT song_id FROM song_search WHERE artist >= ? AND artist < ?
			UNION
			SELECT song_id FROM song_search WHERE album_artist >= ? AND album_artist < ?
			UNION
			SELECT song_id FROM song_artist_fts
			WHERE song_artist_fts MATCH ?
			  AND (instr(artist, ?) > 0 OR instr(album_artist, ?) > 0)
		)
		SELECT songs.album, COALESCE(NULLIF(songs.album_artist, ''), songs.artist) AS resolved_artist,
		       COUNT(*), COALESCE(MAX(songs.cover_path), '')
		FROM matched_songs m
		JOIN songs ON songs.id = m.song_id
		WHERE COALESCE(songs.ignored, 0) = 0
		GROUP BY songs.album, resolved_artist
		ORDER BY CASE WHEN lower(songs.album) >= ? AND lower(songs.album) < ? THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.album
		LIMIT ?`
		albumArgs = []any{
			lower, upper,
			lower, upper,
			lower, upper,
			ftsPhrase, query, query,
			lower, upper, limit,
		}
	} else {
		albumQuery = `
		WITH matched_songs AS (
			SELECT song_id FROM song_search WHERE album >= ? AND album < ?
			UNION
			SELECT song_id FROM song_search WHERE instr(artist, ?) > 0
			UNION
			SELECT song_id FROM song_search WHERE instr(album_artist, ?) > 0
		)
		SELECT songs.album, COALESCE(NULLIF(songs.album_artist, ''), songs.artist) AS resolved_artist,
		       COUNT(*), COALESCE(MAX(songs.cover_path), '')
		FROM matched_songs m
		JOIN songs ON songs.id = m.song_id
		WHERE COALESCE(songs.ignored, 0) = 0
		GROUP BY songs.album, resolved_artist
		ORDER BY CASE WHEN lower(songs.album) >= ? AND lower(songs.album) < ? THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.album
		LIMIT ?`
		albumArgs = []any{
			lower, upper,
			query,
			query,
			lower, upper, limit,
		}
	}

	albumRows, err := d.conn.QueryContext(ctx, albumQuery, albumArgs...)
	if err != nil {
		return result, err
	}
	for albumRows.Next() {
		var album LibrarySearchAlbum
		if err := albumRows.Scan(&album.Name, &album.Artist, &album.SongCount, &album.CoverPath); err != nil {
			albumRows.Close()
			return result, err
		}
		result.Albums = append(result.Albums, album)
	}
	if err := albumRows.Close(); err != nil {
		return result, err
	}

	var artistQuery string
	var artistArgs []any
	if useTrigramFTS {
		artistQuery = `
		WITH matched_artist_songs AS (
			SELECT song_id FROM song_search WHERE artist >= ? AND artist < ?
			UNION
			SELECT song_id FROM song_search WHERE album_artist >= ? AND album_artist < ?
			UNION
			SELECT song_id FROM song_artist_fts
			WHERE song_artist_fts MATCH ?
			  AND (instr(artist, ?) > 0 OR instr(album_artist, ?) > 0)
		),
		artist_songs AS (
			SELECT songs.id, songs.artist AS name, songs.album
			FROM matched_artist_songs m
			JOIN songs ON songs.id = m.song_id
			JOIN song_search ss ON ss.song_id = songs.id
			WHERE COALESCE(songs.ignored, 0) = 0 AND instr(ss.artist, ?) > 0
			UNION
			SELECT songs.id, songs.album_artist AS name, songs.album
			FROM matched_artist_songs m
			JOIN songs ON songs.id = m.song_id
			JOIN song_search ss ON ss.song_id = songs.id
			WHERE COALESCE(songs.ignored, 0) = 0 AND instr(ss.album_artist, ?) > 0
		)
		SELECT name, COUNT(*), COUNT(DISTINCT album)
		FROM artist_songs
		GROUP BY name
		ORDER BY CASE WHEN lower(name) >= ? AND lower(name) < ? THEN 0 ELSE 1 END,
		         COUNT(*) DESC, name
		LIMIT ?`
		artistArgs = []any{
			lower, upper,
			lower, upper,
			ftsPhrase, query, query,
			query,
			query,
			lower, upper, limit,
		}
	} else {
		artistQuery = `
		WITH artist_songs AS (
			SELECT songs.id, songs.artist AS name, songs.album
			FROM songs JOIN song_search ss ON ss.song_id = songs.id
			WHERE COALESCE(songs.ignored, 0) = 0 AND instr(ss.artist, ?) > 0
			UNION
			SELECT songs.id, songs.album_artist AS name, songs.album
			FROM songs JOIN song_search ss ON ss.song_id = songs.id
			WHERE COALESCE(songs.ignored, 0) = 0 AND instr(ss.album_artist, ?) > 0
		)
		SELECT name, COUNT(*), COUNT(DISTINCT album)
		FROM artist_songs
		GROUP BY name
		ORDER BY CASE WHEN lower(name) >= ? AND lower(name) < ? THEN 0 ELSE 1 END,
		         COUNT(*) DESC, name
		LIMIT ?`
		artistArgs = []any{query, query, lower, upper, limit}
	}

	artistRows, err := d.conn.QueryContext(ctx, artistQuery, artistArgs...)
	if err != nil {
		return result, err
	}
	for artistRows.Next() {
		var artist LibrarySearchArtist
		if err := artistRows.Scan(&artist.Name, &artist.SongCount, &artist.AlbumCount); err != nil {
			artistRows.Close()
			return result, err
		}
		result.Artists = append(result.Artists, artist)
	}
	if err := artistRows.Close(); err != nil {
		return result, err
	}

	playlistRows, err := d.conn.QueryContext(ctx, `
		SELECT id, name, json_array_length(song_ids)
		FROM playlists WHERE lower(name) >= ? AND lower(name) < ?
		ORDER BY name
		LIMIT ?`, lower, upper, limit)
	if err != nil {
		return result, err
	}
	for playlistRows.Next() {
		var playlist LibrarySearchPlaylist
		if err := playlistRows.Scan(&playlist.ID, &playlist.Name, &playlist.SongCount); err != nil {
			playlistRows.Close()
			return result, err
		}
		result.Playlists = append(result.Playlists, playlist)
	}
	if err := playlistRows.Close(); err != nil {
		return result, err
	}
	return result, nil
}
