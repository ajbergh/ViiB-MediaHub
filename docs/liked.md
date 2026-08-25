# Liked Songs & Albums

![Liked Songs](../assets/screenshots/liked-songs.png)

![Liked Albums](../assets/screenshots/liked-albums.png)

ViiB MediaHub stores song and album likes as ViiB application state. Local filesystem tracks and synchronized Plex tracks can both be liked through the same UI.

---

## Liked Songs

Liked Songs includes catalog tracks whose ViiB liked state is enabled. The page supports normal playback, shuffle, queue, and track context actions.

For a Plex-backed track, liking or unliking changes ViiB's database only. It does not change the Plex account's rating/favorite state and does not send a metadata update to PMS.

---

## Liked Albums

Liked Albums is also ViiB-local state. Album cards open the normal [Album Detail](albums.md) experience whether the underlying catalog tracks are local or Plex-hosted.

---

## Persistence

Likes survive ViiB restarts because they are stored in the ViiB SQLite database.

They are intentionally independent from external services:

- not synchronized to Plex;
- not synchronized to Spotify;
- not removed merely because a Plex server is temporarily offline.

If a successful authoritative source reconciliation later removes a catalog item that truly no longer exists in the selected Plex library, normal ViiB cleanup/repair behavior applies to stale references.
