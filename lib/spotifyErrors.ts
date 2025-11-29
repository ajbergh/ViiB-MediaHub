/**
 * ViiB MediaHub - Spotify Error Classes
 * 
 * Custom error types for better Spotify API error handling.
 * Provides specific error classes for different failure modes:
 * 
 * - SpotifyError: Base class for all Spotify errors
 * - SpotifyAuthError: Authentication/authorization failures (401, 403)
 * - SpotifyRateLimitError: Rate limit exceeded (429) with retry timing
 * - SpotifyApiError: General API errors with status codes
 * - SpotifyNetworkError: Network connectivity issues
 * 
 * These errors enable precise catch blocks for different recovery strategies.
 * 
 * @module spotifyErrors
 */

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
