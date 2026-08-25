const PLEX_API_BASE = '/api/v2/plex';

export interface PlexServer {
  name: string;
  host: string;
  port: number;
  scheme: string;
  url: string;
  machineIdentifier: string;
  version?: string;
  claimed: boolean;
  authRequired: boolean;
}

export interface PlexSource {
  id: string;
  machineIdentifier: string;
  baseUrl: string;
  name: string;
  version?: string;
  libraryId?: string;
  libraryTitle?: string;
  connectedAt: number;
  lastSyncAt?: number;
  lastSyncStatus: 'never' | 'running' | 'complete' | 'error' | 'auth_required' | string;
  lastSyncError?: string;
  available: boolean;
  active: boolean;
}

export interface PlexLibrary {
  id: string;
  title: string;
  type: string;
  contentKey?: string;
  trackKey?: string;
}

export interface PlexConfig {
  source: PlexSource | null;
  authenticated: boolean;
}

export interface PlexAuthStart {
  authUrl: string;
  expiresAt: number;
}

export interface PlexAuthStatus {
  authenticated: boolean;
  pending: boolean;
  expiresAt?: number;
  message?: string;
}

interface V2ErrorPayload {
  error?: string | { message?: string; code?: string };
}

async function plexResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const payload = await response.json().catch(() => ({})) as V2ErrorPayload;
    const message = typeof payload.error === 'string' ? payload.error : payload.error?.message;
    throw new Error(message || `HTTP ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function isMusicLibrary(library: PlexLibrary): boolean {
  return ['artist', 'music', 'audio'].includes((library.type || '').toLowerCase());
}

export const plexService = {
  async discover(timeoutMs = 3500): Promise<{ servers: PlexServer[]; warning?: string }> {
    const response = await fetch(`${PLEX_API_BASE}/discover`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ timeoutMs }),
    });
    const result = await plexResponse<{ servers?: PlexServer[]; warning?: string }>(response);
    return { ...result, servers: result.servers || [] };
  },

  async connect(url: string): Promise<PlexServer> {
    const response = await fetch(`${PLEX_API_BASE}/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    });
    return plexResponse<PlexServer>(response);
  },

  async getConfig(): Promise<PlexConfig> {
    const response = await fetch(`${PLEX_API_BASE}/config`);
    return plexResponse<PlexConfig>(response);
  },

  async disconnect(): Promise<void> {
    const response = await fetch(`${PLEX_API_BASE}/config`, { method: 'DELETE' });
    await plexResponse(response);
  },

  async startAuth(): Promise<PlexAuthStart> {
    const response = await fetch(`${PLEX_API_BASE}/auth/start`, { method: 'POST' });
    return plexResponse<PlexAuthStart>(response);
  },

  async getAuthStatus(): Promise<PlexAuthStatus> {
    const response = await fetch(`${PLEX_API_BASE}/auth/status`);
    return plexResponse<PlexAuthStatus>(response);
  },

  async getLibraries(): Promise<PlexLibrary[]> {
    const response = await fetch(`${PLEX_API_BASE}/libraries`);
    const result = await plexResponse<{ libraries?: PlexLibrary[] }>(response);
    // Backend filtering is authoritative; keep a defensive frontend filter so
    // future PMS/provider response changes cannot surface video pivots here.
    return (result.libraries || []).filter(isMusicLibrary);
  },

  async selectLibrary(libraryId: string): Promise<PlexLibrary> {
    const response = await fetch(`${PLEX_API_BASE}/library`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ libraryId }),
    });
    return plexResponse<PlexLibrary>(response);
  },

  async sync(): Promise<void> {
    const response = await fetch(`${PLEX_API_BASE}/sync`, { method: 'POST' });
    await plexResponse(response);
  },

  async getSyncStatus(): Promise<PlexSource | null> {
    const response = await fetch(`${PLEX_API_BASE}/sync/status`);
    const result = await plexResponse<{ source: PlexSource | null }>(response);
    return result.source;
  },
};

export default plexService;
