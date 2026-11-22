/** Defines Plex settings state, authentication checks, and reducer transitions. */

import { PlexRequestError, plexService, type PlexAccountServer, type PlexLibrary, type PlexServer, type PlexSource } from '../services/plex';

export async function connectPlexWithConfirmation(connect: (replaceSourceId?: string) => Promise<PlexServer>): Promise<PlexServer | null> {
  try {
    return await connect();
  } catch (error) {
    if (!(error instanceof PlexRequestError && error.code === 'plex_replace_confirmation_required')) throw error;
    const { source } = await plexService.getConfig();
    if (!source) return connect();
    const confirmed = window.confirm(`Replace ${source.name || 'the current Plex server'}? ViiB will remove its synced tracks from your library. Your files on Plex stay in place.`);
    return confirmed ? connect(source.id) : null;
  }
}

export interface PlexSettingsState {
  source: PlexSource | null;
  authenticated: boolean;
  discovered: PlexServer[];
  accountServers: PlexAccountServer[];
  libraries: PlexLibrary[];
  selectedLibraryId: string;
  manualUrl: string;
  busy: 'loading' | 'discovering' | 'account_servers' | 'connecting' | 'authenticating' | 'libraries' | 'saving' | 'saving_schedule' | 'syncing' | 'canceling_sync' | 'disconnecting' | null;
  message: string;
  error: string;
}

export const initialPlexSettingsState: PlexSettingsState = {
  source: null,
  authenticated: false,
  discovered: [],
  accountServers: [],
  libraries: [],
  selectedLibraryId: '',
  manualUrl: '',
  busy: 'loading',
  message: '',
  error: '',
};

export type PlexSettingsAction =
  | { type: 'loaded'; source: PlexSource | null; authenticated: boolean }
  | { type: 'busy'; busy: PlexSettingsState['busy']; message?: string }
  | { type: 'error'; error: string }
  | { type: 'message'; message: string }
  | { type: 'discovered'; servers: PlexServer[]; warning?: string }
  | { type: 'account_servers'; servers: PlexAccountServer[] }
  | { type: 'libraries'; libraries: PlexLibrary[] }
  | { type: 'manual_url'; url: string }
  | { type: 'selected_library'; id: string }
  | { type: 'source'; source: PlexSource | null }
  | { type: 'authenticated'; authenticated: boolean }
  | { type: 'reset' };

// Unclaimed Plex servers can expose libraries without account authentication,
// so token presence alone must never gate the UI. Require sign-in only after the
// source/backend explicitly reports auth failure or the current request error
// says authentication/reconnect is required.
export function plexSourceNeedsAuthentication(source: PlexSource | null, error: string): boolean {
  if (!source) return false;
  if (!source.available && source.lastSyncStatus === 'auth_required') return true;
  return /authentication|sign[ -]?in|reconnect/i.test(error);
}

export function plexSettingsReducer(state: PlexSettingsState, action: PlexSettingsAction): PlexSettingsState {
  switch (action.type) {
    case 'loaded':
      return {
        ...state,
        source: action.source,
        authenticated: action.authenticated,
        selectedLibraryId: action.source?.libraryId || '',
        manualUrl: action.source?.baseUrl || state.manualUrl,
        busy: null,
        error: '',
      };
    case 'busy':
      return { ...state, busy: action.busy, error: '', message: action.message ?? state.message };
    case 'error':
      return { ...state, busy: null, error: action.error, message: '' };
    case 'message':
      return { ...state, busy: null, message: action.message, error: '' };
    case 'discovered':
      return { ...state, busy: null, discovered: action.servers, message: action.warning || '', error: '' };
    case 'account_servers':
      return { ...state, busy: null, accountServers: action.servers, error: '' };
    case 'libraries':
      return {
        ...state,
        busy: null,
        libraries: action.libraries,
        selectedLibraryId: state.source?.libraryId || state.selectedLibraryId,
        error: '',
      };
    case 'manual_url':
      return { ...state, manualUrl: action.url };
    case 'selected_library':
      return { ...state, selectedLibraryId: action.id };
    case 'source':
      return {
        ...state,
        source: action.source,
        selectedLibraryId: action.source?.libraryId || '',
        manualUrl: action.source?.baseUrl || state.manualUrl,
      };
    case 'authenticated':
      return { ...state, authenticated: action.authenticated };
    case 'reset':
      return { ...initialPlexSettingsState, busy: null };
    default:
      return state;
  }
}
