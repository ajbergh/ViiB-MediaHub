/**
 * ViiB MediaHub - Utility Functions
 * 
 * Collection of helper functions used throughout the application:
 * 
 * Time/Formatting:
 * - formatTime: Converts seconds to MM:SS format
 * - generateId: Creates random alphanumeric IDs
 * 
 * Styling:
 * - generateGradient: Creates HSL gradients from seed strings
 * - cssUrl: Escapes URLs for CSS url() usage
 * - coverBackground: Returns cover image or gradient fallback
 * 
 * Audio:
 * - EQ_FREQUENCIES: 10-band equalizer center frequencies (32Hz-16kHz)
 * - EQ_PRESETS: Named equalizer presets (Flat, Rock, Jazz, etc.)
 * 
 * @module utils
 */

import { EqPreset } from "./types";

/**
 * Formats seconds into MM:SS display format.
 * Returns '0:00' for invalid input.
 */
export const formatTime = (seconds: number): string => {
  if (isNaN(seconds)) return '0:00';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
};

export const generateGradient = (seed: string) => {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = seed.charCodeAt(i) + ((hash << 5) - hash);
  }
  const c1 = `hsl(${hash % 360}, 60%, 40%)`;
  const c2 = `hsl(${(hash + 40) % 360}, 60%, 20%)`;
  return `linear-gradient(135deg, ${c1}, ${c2})`;
};

export const generateId = () => Math.random().toString(36).substr(2, 9);

/**
 * Escapes a URL for use in CSS url() function.
 * Wraps the URL in quotes and escapes special characters.
 */
export const cssUrl = (url: string): string => {
  if (!url) return '';
  // Escape backslashes and quotes, then wrap in quotes for CSS
  const escaped = url.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
  return `url("${escaped}")`;
};

/**
 * Detects if the app is running in a Wails environment (native desktop).
 * Wails uses 'wails.localhost' as the hostname for the WebView.
 */
export const isWailsEnvironment = (): boolean => {
  return window.location.hostname === 'wails.localhost';
};

/**
 * Gets the OAuth callback URL, handling the Wails environment specially.
 * In Wails, we need to use the backend server URL for OAuth callbacks
 * since 'wails.localhost' is not a valid redirect URI for Spotify.
 * 
 * For Wails builds, this returns a URL like: http://127.0.0.1:PORT/callback
 * For web builds, this returns the current origin + /callback
 */
export const getOAuthCallbackUrl = async (): Promise<string> => {
  if (isWailsEnvironment()) {
    // Try to get the server URL from Wails bindings
    try {
      // @ts-ignore - window.go is injected by Wails runtime
      if (window.go?.main?.App?.GetServerURL) {
        const serverUrl = await window.go.main.App.GetServerURL();
        return `${serverUrl}/callback`;
      }
    } catch (e) {
      console.error('Failed to get Wails server URL:', e);
    }
    // Fallback: use 127.0.0.1 with the current port (or default 8080)
    // This shouldn't happen in production since GetServerURL should work
    return 'http://127.0.0.1:8080/callback';
  }
  
  // Standard web build - use current origin
  let origin = window.location.origin;
  if (window.location.hostname === 'localhost') {
    // Spotify prefers 127.0.0.1 over localhost
    origin = origin.replace('localhost', '127.0.0.1');
  }
  return `${origin}/callback`;
};

/**
 * Generates a CSS background property with a cover image or gradient fallback.
 */
export const coverBackground = (coverUrl: string | undefined, fallbackSeed: string): string => {
  if (coverUrl) {
    return `${cssUrl(coverUrl)} center/cover no-repeat`;
  }
  return generateGradient(fallbackSeed);
};

// --- EQ Presets ---

export const EQ_FREQUENCIES = [32, 64, 125, 250, 500, 1000, 2000, 4000, 8000, 16000];

export const EQ_PRESETS: EqPreset[] = [
  { id: 'flat', name: 'Flat', gains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] },
  { id: 'acoustic', name: 'Acoustic', gains: [4, 4, 3, 1, 1, 1, 2, 3, 3, 2] },
  { id: 'bass-boost', name: 'Bass Boost', gains: [7, 6, 5, 3, 1, 0, 0, 0, 0, 0] },
  { id: 'bass-reducer', name: 'Bass Reducer', gains: [-7, -6, -5, -3, -1, 0, 0, 0, 0, 0] },
  { id: 'classical', name: 'Classical', gains: [5, 4, 3, 2, -1, -1, 0, 2, 3, 3] },
  { id: 'dance', name: 'Dance', gains: [4, 6, 5, 0, 1, 3, 5, 5, 4, 0] },
  { id: 'deep', name: 'Deep', gains: [5, 4, 2, 1, 0, 1, 0, -2, -4, -5] },
  { id: 'electronic', name: 'Electronic', gains: [5, 4, 1, 0, -2, 2, 1, 2, 5, 6] },
  { id: 'hip-hop', name: 'Hip-Hop', gains: [6, 5, 2, -1, -2, -1, 1, -1, 2, 3] },
  { id: 'jazz', name: 'Jazz', gains: [3, 3, 1, 2, 2, 2, 1, 2, 3, 4] },
  { id: 'latin', name: 'Latin', gains: [4, 3, 0, 0, -1, -1, -1, 0, 3, 5] },
  { id: 'loudness', name: 'Loudness', gains: [6, 4, 0, -2, -5, -1, 0, -4, 5, 2] },
  { id: 'lounge', name: 'Lounge', gains: [-3, -2, -1, 1, 3, 2, 0, -1, 2, 1] },
  { id: 'piano', name: 'Piano', gains: [2, 1, 0, 2, 3, 1, 2, 3, 1, 2] },
  { id: 'pop', name: 'Pop', gains: [-1, 1, 3, 4, 3, 1, -1, -1, 1, 1] },
  { id: 'r-n-b', name: 'R&B', gains: [3, 7, 5, 1, -2, -1, 1, 1, 2, 3] },
  { id: 'rock', name: 'Rock', gains: [5, 4, 3, 1, -1, -1, 1, 3, 4, 5] },
  { id: 'small-speakers', name: 'Small Speakers', gains: [7, 6, 5, 4, 2, 0, -2, -3, -4, -5] },
  { id: 'spoken-word', name: 'Spoken Word', gains: [-3, -1, 0, 1, 4, 5, 5, 4, 1, -2] },
  { id: 'treble-boost', name: 'Treble Booster', gains: [0, 0, 0, 0, 0, 1, 3, 5, 6, 8] },
  { id: 'treble-reducer', name: 'Treble Reducer', gains: [0, 0, 0, 0, 0, -1, -3, -5, -6, -8] },
  { id: 'vocal-boost', name: 'Vocal Booster', gains: [-2, -3, -1, 2, 4, 5, 4, 2, 0, -1] },
];