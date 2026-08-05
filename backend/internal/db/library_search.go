package db

import "strings"

func (d *DB) SearchLibrary(query string, limit int) (LibrarySearchResult, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	limit = clampPageLimit(limit, 50, 200)
	result := LibrarySearchResult{
		Query: query, Tracks: []Song{}, Albums: []LibrarySearchAlbum{},
		Artists: []LibrarySearchArtist{}, Playlists: []LibrarySearchPlaylist{},
	}
	if query == "" { return result, nil }

	escaped := escapeLikePattern(query)
	prefix := escaped + "%"
	contains := "%" + escaped + "%"

	rows, err := d.conn.Query(songSelect+`
		JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		  AND (ss.title LIKE ? ESCAPE '\' OR ss.artist LIKE ? ESCAPE '\'
		       OR ss.album LIKE ? ESCAPE '\' OR ss.album_artist LIKE ? ESCAPE '\'
		       OR ss.genre LIKE ? ESCAPE '\' OR ss.file_path LIKE ? ESCAPE '\')
		ORDER BY CASE
			WHEN ss.title LIKE ? ESCAPE '\' THEN 0
			WHEN ss.artist LIKE ? ESCAPE '\' THEN 1
			WHEN ss.album LIKE ? ESCAPE '\' THEN 2 ELSE 3 END,
			COALESCE(songs.play_count, 0) DESC, songs.title
		LIMIT ?`, contains, contains, contains, contains, contains, contains,
		prefix, prefix, prefix, limit)
	if err != nil { return result, err }
	result.Tracks, err = scanLibrarySongRows(rows)
	if err != nil { return result, err }

	albumRows, err := d.conn.Query(`
		SELECT songs.album, COALESCE(NULLIF(songs.album_artist, ''), songs.artist) AS resolved_artist,
		       COUNT(*), COALESCE(MAX(songs.cover_path), '')
		FROM songs JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		  AND (ss.album LIKE ? ESCAPE '\' OR ss.album_artist LIKE ? ESCAPE '\' OR ss.artist LIKE ? ESCAPE '\')
		GROUP BY songs.album, resolved_artist
		ORDER BY CASE WHEN lower(songs.album) LIKE ? ESCAPE '\' THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.album
		LIMIT ?`, contains, contains, contains, prefix, limit)
	if err != nil { return result, err }
	for albumRows.Next() {
		var album LibrarySearchAlbum
		if err := albumRows.Scan(&album.Name, &album.Artist, &album.SongCount, &album.CoverPath); err != nil {
			albumRows.Close(); return result, err
		}
		result.Albums = append(result.Albums, album)
	}
	if err := albumRows.Close(); err != nil { return result, err }

	artistRows, err := d.conn.Query(`
		SELECT songs.artist, COUNT(*), COUNT(DISTINCT songs.album)
		FROM songs JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0 AND ss.artist LIKE ? ESCAPE '\'
		GROUP BY songs.artist
		ORDER BY CASE WHEN lower(songs.artist) LIKE ? ESCAPE '\' THEN 0 ELSE 1 END,
		         COUNT(*) DESC, songs.artist
		LIMIT ?`, contains, prefix, limit)
	if err != nil { return result, err }
	for artistRows.Next() {
		var artist LibrarySearchArtist
		if err := artistRows.Scan(&artist.Name, &artist.SongCount, &artist.AlbumCount); err != nil {
			artistRows.Close(); return result, err
		}
		result.Artists = append(result.Artists, artist)
	}
	if err := artistRows.Close(); err != nil { return result, err }

	playlistRows, err := d.conn.Query(`
		SELECT id, name, json_array_length(song_ids)
		FROM playlists WHERE lower(name) LIKE ? ESCAPE '\'
		ORDER BY CASE WHEN lower(name) LIKE ? ESCAPE '\' THEN 0 ELSE 1 END, name
		LIMIT ?`, contains, prefix, limit)
	if err != nil { return result, err }
	for playlistRows.Next() {
		var playlist LibrarySearchPlaylist
		if err := playlistRows.Scan(&playlist.ID, &playlist.Name, &playlist.SongCount); err != nil {
			playlistRows.Close(); return result, err
		}
		result.Playlists = append(result.Playlists, playlist)
	}
	if err := playlistRows.Close(); err != nil { return result, err }
	return result, nil
}
