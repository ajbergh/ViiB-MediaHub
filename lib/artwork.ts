/**
 * Plex-backed catalog artwork is authoritative for Plex media. PMS artwork URLs
 * are proxied through ViiB and carry a source-derived version query so browser
 * caches refresh when the selected Plex artwork changes.
 */
export const isAuthoritativePlexArtwork = (url?: string | null): boolean =>
  Boolean(url && /^\/api\/cover\/[^?]+\?v=/.test(url));

export const isPlexSourcePath = (path?: string | null): boolean =>
  Boolean(path && path.startsWith('plex://'));

/**
 * Preserve the existing enrichment preference for local media, but never let
 * Spotify or other enrichment artwork replace Plex's artwork decision. That
 * includes the intentional absence of artwork on the Plex item.
 */
export const resolveAlbumArtwork = (
  catalogArtwork?: string | null,
  enrichmentArtwork?: string | null,
  plexBacked = false,
): string | undefined => {
  if (plexBacked || isAuthoritativePlexArtwork(catalogArtwork)) return catalogArtwork || undefined;
  return enrichmentArtwork || catalogArtwork || undefined;
};

export interface AlbumArtworkIndexEntry {
  name: string;
  artist: string;
  coverUrl?: string;
  plexBacked?: boolean;
}

/**
 * Builds both exact `album::artist` cover keys and the legacy album-title alias.
 * The title-only alias is emitted only when every logical album with that title
 * has the same artwork authority and value. This prevents a Plex album whose
 * authoritative state is "no artwork" from borrowing artwork from a different
 * local album with the same title, while also avoiding suppressing that local
 * album through a globally shared Plex sentinel.
 *
 * An empty string is retained as an intentional Plex no-art sentinel for an
 * unambiguous title. Existing track-level surfaces use a truthy/falsy fallback,
 * so a missing ambiguous alias safely produces a placeholder rather than the
 * wrong album cover.
 */
export const buildAlbumCoverIndex = (albums: AlbumArtworkIndexEntry[]): Record<string, string> => {
  const covers: Record<string, string> = {};
  const titleAliases = new Map<string, {
    value: string | undefined;
    plexBacked: boolean;
    ambiguous: boolean;
  }>();

  albums.forEach((album) => {
    const plexBacked = Boolean(album.plexBacked);
    const value = plexBacked ? (album.coverUrl || '') : album.coverUrl;
    const composite = `${album.name}::${album.artist}`;

    if (value !== undefined) {
      covers[composite] = value;
    }

    const current = titleAliases.get(album.name);
    if (!current) {
      titleAliases.set(album.name, { value, plexBacked, ambiguous: false });
      return;
    }

    if (current.plexBacked !== plexBacked || current.value !== value) {
      current.ambiguous = true;
    }
  });

  titleAliases.forEach((alias, title) => {
    if (!alias.ambiguous && alias.value !== undefined) {
      covers[title] = alias.value;
    }
  });

  return covers;
};
