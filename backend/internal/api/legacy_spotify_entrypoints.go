package api

// These legacy Spotify download entry points are intentionally retained while
// the current download flow remains in transition. Keeping explicit method
// expressions documents that they are compatibility/fallback code rather than
// accidentally dead code, while allowing Staticcheck U1000 to remain enabled.
var (
	_ = (*API).downloadAlbumByID
	_ = (*API).downloadPlaylistByID
	_ = (*API).downloadPlaylistByScraping
)
