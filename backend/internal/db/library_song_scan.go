package db

import (
	"database/sql"
	"encoding/json"
)

const songSelect = `
	SELECT songs.id, songs.title, songs.artist, songs.album, songs.album_artist,
	       songs.track_number, songs.disc_number, songs.genre, songs.year,
	       songs.original_year, songs.year_uncertain, songs.year_analyzed_at,
	       songs.duration, songs.replay_gain_db, songs.replay_peak, songs.file_path,
	       songs.cover_path, songs.added_at, songs.play_count, songs.last_played,
	       songs.skip_count, songs.file_hash, songs.mood, songs.energy, songs.tempo,
	       songs.bpm, songs.instrumental, songs.mood_analyzed_at, songs.liked,
	       songs.liked_at, songs.lastfm_listeners, songs.lastfm_playcount,
	       songs.lastfm_tags, songs.lastfm_url, songs.lastfm_mbid, songs.lastfm_enriched_at
	FROM songs`

type libraryRowScanner interface {
	Scan(dest ...any) error
}

func scanLibrarySong(row libraryRowScanner) (Song, error) {
	var song Song
	var genreJSON, albumArtist, coverPath, fileHash, mood, energy, tempo sql.NullString
	var lastFMTags, lastFMURL, lastFMMBID sql.NullString
	var trackNum, discNum, year, originalYear, yearUncertain, yearAnalyzedAt sql.NullInt64
	var playCount, lastPlayed, skipCount, bpm, instrumental, moodAnalyzedAt sql.NullInt64
	var liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64
	var replayGainDB, replayPeak sql.NullFloat64

	err := row.Scan(
		&song.ID, &song.Title, &song.Artist, &song.Album, &albumArtist,
		&trackNum, &discNum, &genreJSON, &year, &originalYear, &yearUncertain,
		&yearAnalyzedAt, &song.Duration, &replayGainDB, &replayPeak, &song.FilePath,
		&coverPath, &song.AddedAt, &playCount, &lastPlayed, &skipCount, &fileHash,
		&mood, &energy, &tempo, &bpm, &instrumental, &moodAnalyzedAt, &liked,
		&likedAt, &lastFMListeners, &lastFMPlaycount, &lastFMTags, &lastFMURL,
		&lastFMMBID, &lastFMEnrichedAt,
	)
	if err != nil {
		return Song{}, err
	}

	if albumArtist.Valid { song.AlbumArtist = albumArtist.String }
	if trackNum.Valid { song.TrackNumber = int(trackNum.Int64) }
	if discNum.Valid { song.DiscNumber = int(discNum.Int64) }
	if genreJSON.Valid && genreJSON.String != "" { _ = json.Unmarshal([]byte(genreJSON.String), &song.Genre) }
	if year.Valid { song.Year = int(year.Int64) }
	if originalYear.Valid { song.OriginalYear = int(originalYear.Int64) }
	song.YearUncertain = yearUncertain.Valid && yearUncertain.Int64 == 1
	if yearAnalyzedAt.Valid { song.YearAnalyzedAt = yearAnalyzedAt.Int64 }
	if replayGainDB.Valid { song.ReplayGainDB = replayGainDB.Float64 }
	if replayPeak.Valid { song.ReplayPeak = replayPeak.Float64 }
	if coverPath.Valid { song.CoverPath = coverPath.String }
	if playCount.Valid { song.PlayCount = int(playCount.Int64) }
	if lastPlayed.Valid { song.LastPlayed = lastPlayed.Int64 }
	if skipCount.Valid { song.SkipCount = int(skipCount.Int64) }
	if fileHash.Valid { song.FileHash = fileHash.String }
	if mood.Valid { song.Mood = mood.String }
	if energy.Valid { song.Energy = energy.String }
	if tempo.Valid { song.Tempo = tempo.String }
	if bpm.Valid { song.BPM = int(bpm.Int64) }
	song.Instrumental = instrumental.Valid && instrumental.Int64 == 1
	if moodAnalyzedAt.Valid { song.MoodAnalyzedAt = moodAnalyzedAt.Int64 }
	song.Liked = liked.Valid && liked.Int64 == 1
	if likedAt.Valid { song.LikedAt = likedAt.Int64 }
	if lastFMListeners.Valid { song.LastFMListeners = int(lastFMListeners.Int64) }
	if lastFMPlaycount.Valid { song.LastFMPlaycount = int(lastFMPlaycount.Int64) }
	if lastFMTags.Valid { song.LastFMTags = lastFMTags.String }
	if lastFMURL.Valid { song.LastFMURL = lastFMURL.String }
	if lastFMMBID.Valid { song.LastFMMBID = lastFMMBID.String }
	if lastFMEnrichedAt.Valid { song.LastFMEnrichedAt = lastFMEnrichedAt.Int64 }
	return song, nil
}

func scanLibrarySongRows(rows *sql.Rows) ([]Song, error) {
	defer rows.Close()
	songs := make([]Song, 0)
	for rows.Next() {
		song, err := scanLibrarySong(rows)
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	return songs, rows.Err()
}
