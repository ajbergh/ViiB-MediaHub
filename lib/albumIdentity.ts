export const albumIdentity = (album: string, artist: string): string =>
  `${album.trim()}::${artist.trim()}`;

export const albumRoute = (album: string, artist: string): string =>
  `/album/${encodeURIComponent(album)}/${encodeURIComponent(artist)}`;
