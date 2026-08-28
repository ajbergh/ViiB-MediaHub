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

// A sanitized PMS resource returned by the signed-in Plex account. `owned`
// differentiates a user's own servers from servers whose libraries were shared
// with them; no Plex token is ever exposed to the browser.
export interface PlexAccountServer {
  name: string;
  url: string;
  machineIdentifier: string;
  version?: string;
  owned: boolean;
  owner?: string;
  local: boolean;
  relay: boolean;
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

export interface PlexMetadataWritebackField {
  field: 'genres' | 'year' | string;
  before: string[] | number;
  after: string[] | number;
}

export interface PlexMetadataWritebackItem {
  songId: string;
  title: string;
  artist: string;
  album: string;
  proposedGenres?: string[];
  proposedYear?: number;
  changes: PlexMetadataWritebackField[];
  status: 'ready' | 'already_matches' | string;
}

export interface PlexMetadataWritebackPreview {
  confirmation: string;
  items: PlexMetadataWritebackItem[];
  hasMore: boolean;
}

export interface PlexMetadataWritebackResult {
  updated: number;
  verified: number;
  failed: number;
  errors?: string[];
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

  async getAccountServers(): Promise<PlexAccountServer[]> {
    const response = await fetch(`${PLEX_API_BASE}/servers`);
    const result = await plexResponse<{ servers?: PlexAccountServer[] }>(response);
    return result.servers || [];
  },

  async connectAccountServer(machineIdentifier: string): Promise<PlexServer> {
    const response = await fetch(`${PLEX_API_BASE}/servers/select`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ machineIdentifier }),
    });
    return plexResponse<PlexServer>(response);
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

  async previewAIEnrichmentWriteback(songIds?: string[]): Promise<PlexMetadataWritebackPreview> {
    const response = await fetch(`${PLEX_API_BASE}/metadata-writeback/preview`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ songIds: songIds || [] }),
    });
    return plexResponse<PlexMetadataWritebackPreview>(response);
  },

  async syncAIEnrichmentWriteback(confirmation: string, songIds?: string[]): Promise<PlexMetadataWritebackResult> {
    const response = await fetch(`${PLEX_API_BASE}/metadata-writeback/sync`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirmation, songIds: songIds || [] }),
    });
    return plexResponse<PlexMetadataWritebackResult>(response);
  },
};

export default plexService;
