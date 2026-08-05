package api

import "github.com/ajbergh/viib-mediahub/internal/db"

// transformSongForAPI keeps the operations handlers independent from the
// legacy smart-playlist helper while sharing the v2 library DTO policy.
func transformSongForAPI(song *db.Song) {
	transformLibrarySongForAPI(song)
}
