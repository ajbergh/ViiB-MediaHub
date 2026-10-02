/**
 * Splits an artist string into individual artists.
 * Handles common separators like ", ", " & ", " feat. ", " ft. ", etc.
 */
export const splitArtistNames = (artistString: string): string[] => {
  if (!artistString) return [];

  // Common separators used in artist fields
  const separators = [
    ' feat. ', ' feat ', ' ft. ', ' ft ',
    ' featuring ', ' & ', ' x ', ' and ',
    ', ', ' / ', ' vs. ', ' vs '
  ];

  let artists = [artistString];

  for (const sep of separators) {
    const newArtists: string[] = [];
    for (const artist of artists) {
      // Case-insensitive split
      const parts = artist.split(new RegExp(sep.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i'));
      newArtists.push(...parts);
    }
    artists = newArtists;
  }

  // Clean up and filter empty/whitespace-only entries
  return artists.map(a => a.trim()).filter(a => a.length > 0);
};
