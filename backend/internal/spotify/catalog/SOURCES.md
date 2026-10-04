# Fixed Web Player catalog protocol sources

**Documentation reviewed:** 2026-10-04. Implementation checkpoint: `8187104`. The pinned revisions below are retained; this documentation review did not fetch new hashes or rerun provider calls.

These sources identify protocol contracts used by the adapter. They do not prove universal account/market availability or completed OAuth parity. Current application coverage and unresolved Recent/profile fields are recorded in the [parity audit](../../../../docs/SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md).

Token/profile/client context: [stupid-social f0f8c219e43d394c84516a3bcc7af7c9fd41f713](https://github.com/stephancill/stupid-social/blob/f0f8c219e43d394c84516a3bcc7af7c9fd41f713/scripts/spotify-web-client.py), Apache-2.0; retained license: [UPSTREAM_LICENSE](UPSTREAM_LICENSE).

Search operation hash, variables and field names: [Spotui f9d05b6450e730469d15f30f9dd4bc790db794db SpotifyHashProvider.kt](https://github.com/Spotui/Spotui/blob/f9d05b6450e730469d15f30f9dd4bc790db794db/spotify/src/main/kotlin/com/metrolist/spotify/SpotifyHashProvider.kt) and [Spotify.kt](https://github.com/Spotui/Spotui/blob/f9d05b6450e730469d15f30f9dd4bc790db794db/spotify/src/main/kotlin/com/metrolist/spotify/Spotify.kt). Only protocol facts are used; its implementation code, automatic remote hash updates, retries and diagnostics are not incorporated.

Operation hashes are reviewed constants. The application never downloads executable protocol code or trusts a mutable hash registry during normal requests. Live checks and coverage limits are recorded in the [validation record](../../../../docs/SPOTIFY_WEBPLAYER_VALIDATION.md).

Album detail and track pages use the same pinned Spotui revision: getAlbum, albumUnion, tracksV2, uri/locale/offset/limit. Live field/type checks confirmed copyright.items and trackNumber/discNumber; track payloads in this operation omit __typename. Only the fixed album track position accepts that omission; identity, duration, artist and parent album checks remain required.

Artist profile and top tracks use queryArtistOverview from the same pinned Spotui revision. Live checks established artistUnion.id, profile, stats.followers, visuals, and discography.topTracks.items[].track. Some top-track albums omit names; those are resolved through the fixed getAlbum adapter and reused per album within the request. No artist identity or album title is guessed. Country-specific market behavior still requires verification.

Playlist detail/content use fetchPlaylist with uri/offset/limit and enableWatchFeedEntrypoint=false from the same pinned Spotui revision. Live field/type verification established playlistV2.followers, revisionId, content.items[].itemV2.data, addedAt.isoString and trackDuration.totalMilliseconds. Public/private state is not inferred from unrelated capabilities.

Saved album/playlist pages use libraryV3 from the same pinned Spotui revision, with filters Albums/Playlists, offset/limit, flattened folders, and an empty optional-feature list. Root identities are checked before the already verified album/playlist adapters resolve artwork, owners and track counts. Live checks passed two limit-1 pages and normal limit-20 pages for both kinds. Page positions and duplicate occurrences are preserved; incomplete pages fail conversion.

Individual track metadata uses fixed getTrack, uri and trackUnion from [Islom4ik/SpotdlRip 5120b890ee0a8d8ed4599fe32ed10bca7cd795c3 spotdlrip.py](https://github.com/Islom4ik/SpotdlRip/blob/5120b890ee0a8d8ed4599fe32ed10bca7cd795c3/spotdlrip.py). Only protocol facts are incorporated; no upstream implementation is copied. Its documented request uses Pathfinder v1; consenting-account checks here verified the fixed hash on the existing bounded v2 transport. Full parent artwork/release dates come from the verified getAlbum operation. Arbitrary-ID batches are bounded compositions of this single-track operation, with per-request track/album reuse and preserved input positions.

## History and profile limits

Recent context history is not an individual play-event contract: it does not establish repeated plays, `played_at` timestamps or the application's before/after cursor semantics. The public Recent endpoint returned 429 in the recorded cookie context, and an experimental listening-history root returned 404. No guessed history adapter is enabled.

The profile operation supplies identity/name/artwork in tested samples. Email/product/country/followers were absent from the inspected fixed response at both checked levels. Preserve unavailable fields as null; neither protocol pinning nor successful authentication justifies inventing their values. Other platform runtime and market variants remain separate verification work.
