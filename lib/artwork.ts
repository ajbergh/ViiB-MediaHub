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
