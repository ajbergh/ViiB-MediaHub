// Spotify-specific error classes for better error handling

export class SpotifyError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SpotifyError';
  }
}

export class SpotifyAuthError extends SpotifyError {
  constructor(message: string, public statusCode?: number) {
    super(message);
    this.name = 'SpotifyAuthError';
  }
}

export class SpotifyRateLimitError extends SpotifyError {
  constructor(
    message: string,
    public retryAfter?: number // seconds until retry
  ) {
    super(message);
    this.name = 'SpotifyRateLimitError';
  }
}

export class SpotifyApiError extends SpotifyError {
  constructor(
    message: string,
    public statusCode: number,
    public response?: any
  ) {
    super(message);
    this.name = 'SpotifyApiError';
  }
}

export class SpotifyNetworkError extends SpotifyError {
  constructor(message: string, public originalError?: any) {
    super(message);
    this.name = 'SpotifyNetworkError';
  }
}
