/** Manages Plex source configuration, synchronization, and reviewed AI metadata writeback. */

import React, { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, ExternalLink, Eye, Loader2, Music, RefreshCw, Search, Server, ShieldCheck, Trash2 } from 'lucide-react';
import { Button } from './ui/Button';
import { TextInput } from './ui/TextInput';
import { useStore } from '../store';
import { PlexRequestError, plexService, type PlexAccountServer, type PlexMetadataWritebackPreview, type PlexServer } from '../services/plex';
import { openExternalURL, reserveExternalBrowserTab, shouldUseSystemBrowser } from '../services/externalNavigation';
import { connectPlexWithConfirmation, initialPlexSettingsState, plexSettingsReducer, plexSourceNeedsAuthentication, type PlexSettingsState } from '../lib/plexSettingsState';

const busyLabel: Record<NonNullable<PlexSettingsState['busy']>, string> = {
  loading: 'Loading Plex configuration…',
  discovering: 'Searching the local network…',
  account_servers: 'Finding servers available to your Plex account…',
  connecting: 'Validating Plex Media Server…',
  authenticating: 'Waiting for Plex sign-in…',
  libraries: 'Loading music libraries…',
  saving: 'Saving music library…',
  saving_schedule: 'Saving Plex sync schedule…',
  syncing: 'Synchronizing Plex music…',
  canceling_sync: 'Canceling Plex sync…',
  disconnecting: 'Removing Plex source…',
};

function formatSyncTime(value?: number): string {
  return value ? new Date(value).toLocaleString() : 'Never';
}

function serverAddress(server: PlexServer): string {
  return server.url || `${server.scheme || 'http'}://${server.host}:${server.port}`;
}

function formatWritebackValue(value: string[] | number): string {
  return Array.isArray(value) ? (value.length > 0 ? value.join(', ') : 'None') : (value > 0 ? String(value) : 'Unknown');
}

export const PlexMusicSourceSettings: React.FC = () => {
  const [state, dispatch] = useReducer(plexSettingsReducer, initialPlexSettingsState);
  const [editingServer, setEditingServer] = useState(false);
  const [writebackPreview, setWritebackPreview] = useState<PlexMetadataWritebackPreview | null>(null);
  const [writebackBusy, setWritebackBusy] = useState(false);
  const [writebackApproved, setWritebackApproved] = useState(false);
  const [writebackMessage, setWritebackMessage] = useState('');
  const [writebackError, setWritebackError] = useState('');
  const [authUrl, setAuthUrl] = useState('');
  const authTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [autoSync, setAutoSync] = useState(true);
  const disposed = useRef(false);
  const refreshLibrary = useStore(store => store.refreshLibrary);
  const addLog = useStore(store => store.addLog);

  const clearTimers = useCallback(() => {
    if (authTimer.current) clearTimeout(authTimer.current);
    authTimer.current = null;
  }, []);

  const loadLibraries = useCallback(async () => {
    dispatch({ type: 'busy', busy: 'libraries' });
    try {
      const libraries = await plexService.getLibraries().finally(async () => {
        if (disposed.current) return;
        const config = await plexService.getConfig().catch(() => null);
        if (config && !disposed.current) dispatch({ type: 'source', source: config.source });
      });
      if (!disposed.current) dispatch({ type: 'libraries', libraries });
    } catch (error) {
      if (!disposed.current) dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to load Plex music libraries' });
    }
  }, []);

  const loadAccountServers = useCallback(async () => {
    dispatch({ type: 'busy', busy: 'account_servers', message: '' });
    try {
      const servers = await plexService.getAccountServers();
      if (disposed.current) return;
      dispatch({ type: 'account_servers', servers });
      if (servers.length === 0) {
        dispatch({ type: 'message', message: 'Your Plex account has no media servers to choose from.' });
      }
    } catch (error) {
      if (!disposed.current) dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to load Plex account servers' });
    }
  }, []);

  const loadConfig = useCallback(async () => {
    dispatch({ type: 'busy', busy: 'loading' });
    try {
      const config = await plexService.getConfig();
      if (disposed.current) return;
      setAutoSync(config.autoSync ?? true);
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      if (!config.source) {
        if (config.authenticated) await loadAccountServers();
        return;
      }
      await loadLibraries();
    } catch (error) {
      if (!disposed.current) dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to load Plex configuration' });
    }
  }, [loadAccountServers, loadLibraries]);

  useEffect(() => {
    disposed.current = false;
    void loadConfig();
    return () => {
      disposed.current = true;
      clearTimers();
    };
  }, [clearTimers, loadConfig]);

  const discover = async () => {
    dispatch({ type: 'busy', busy: 'discovering', message: '' });
    try {
      const result = await plexService.discover();
      dispatch({ type: 'discovered', servers: result.servers, warning: result.warning });
      setEditingServer(true);
      if (result.servers.length === 0 && !result.warning) {
        dispatch({ type: 'message', message: 'No Plex servers were found on the LAN. Add one manually below.' });
      }
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Plex discovery failed' });
    }
  };

  const connect = async (url: string) => {
    if (!url.trim()) return;
    dispatch({ type: 'busy', busy: 'connecting', message: '' });
    try {
      const server = await connectPlexWithConfirmation(replace => plexService.connect(url.trim(), replace));
      if (!server) {
        dispatch({ type: 'busy', busy: null });
        return;
      }
      addLog('success', `[Plex] Connected to ${server.name || server.host}`);
      setEditingServer(false);
      const config = await plexService.getConfig();
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      await loadLibraries();
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to connect to Plex Media Server' });
    }
  };

  const connectAccountServer = async (server: PlexAccountServer) => {
    dispatch({ type: 'busy', busy: 'connecting', message: '' });
    try {
      const connected = await connectPlexWithConfirmation(replace => plexService.connectAccountServer(server.machineIdentifier, replace));
      if (!connected) {
        dispatch({ type: 'busy', busy: null });
        return;
      }
      addLog('success', `[Plex] Connected to ${connected.name || server.name || connected.machineIdentifier}`);
      setEditingServer(false);
      const config = await plexService.getConfig();
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      await loadLibraries();
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to connect to Plex account server' });
    }
  };

  const pollAuthentication = useCallback(async (expiresAt: number) => {
    if (disposed.current) return;
    try {
      const status = await plexService.getAuthStatus();
      if (disposed.current) return;
      if (status.authenticated) {
        setAuthUrl('');
        dispatch({ type: 'authenticated', authenticated: true });
        addLog('success', '[Plex] Authentication completed');
        dispatch({ type: 'message', message: 'Plex sign-in complete.' });
        await loadConfig();
        return;
      }
      if (!status.pending || Date.now() >= expiresAt * 1000) {
        setAuthUrl('');
        dispatch({ type: 'error', error: status.message || 'Plex sign-in expired. Start sign-in again.' });
        return;
      }
      authTimer.current = setTimeout(() => void pollAuthentication(expiresAt), 2000);
    } catch (error) {
      if (disposed.current) return;
      if (Date.now() < expiresAt * 1000 && (!(error instanceof PlexRequestError) || error.status >= 500)) {
        authTimer.current = setTimeout(() => void pollAuthentication(expiresAt), 2000);
        return;
      }
      setAuthUrl('');
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to check Plex sign-in status' });
    }
  }, [addLog, loadConfig]);

  const signIn = async () => {
    dispatch({ type: 'busy', busy: 'authenticating', message: '' });
    // Open a blank window synchronously while this click still has a user
    // gesture. Opening only after the network request is commonly popup-blocked.
    const useSystemBrowser = shouldUseSystemBrowser();
    setAuthUrl('');
    const authWindow = useSystemBrowser ? null : reserveExternalBrowserTab();
    try {
      const start = await plexService.startAuth();
      if (useSystemBrowser) {
        await openExternalURL(start.authUrl);
      } else if (authWindow) {
        authWindow.location.href = start.authUrl;
      } else {
        setAuthUrl(start.authUrl);
      }
      // Keep ViiB open and poll even if the authorization window needed a
      // second attempt; never navigate the desktop app away from its UI.
      authTimer.current = setTimeout(() => void pollAuthentication(start.expiresAt), 1500);
    } catch (error) {
      authWindow?.close();
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to start Plex sign-in' });
    }
  };

  const selectLibrary = async (libraryId: string) => {
    if (state.source?.libraryId === libraryId) return;
    dispatch({ type: 'busy', busy: 'saving', message: '' });
    try {
      const selected = await plexService.selectLibrary(libraryId);
      const config = await plexService.getConfig();
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      dispatch({ type: 'message', message: `${selected.title} selected. Synchronize to add it to the ViiB library.` });
      await refreshLibrary();
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to select Plex music library' });
    }
  };

  const sourceId = state.source?.id;
  const syncStatus = state.source?.lastSyncStatus;
  const lastSyncAt = state.source?.lastSyncAt;
  useEffect(() => {
    if (!sourceId || editingServer) return;
    const delay = syncStatus === 'running' ? 1500 : 5000;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const checkSync = async () => {
      try {
        const source = await plexService.getSyncStatus();
        if (stopped || (source && source.id !== sourceId)) return;
        dispatch({ type: 'source', source });
        if (source?.lastSyncStatus === 'complete' && (syncStatus === 'running' || source.lastSyncAt !== lastSyncAt)) {
          addLog('success', `[Plex] Synchronized ${source.libraryTitle || 'music library'}`);
          await refreshLibrary();
        }
      } catch {
        // The sync runs on the backend. Continue monitoring after a temporary
        // failure to reach the status endpoint.
      } finally {
        if (!stopped) timer = setTimeout(() => void checkSync(), delay);
      }
    };
    timer = setTimeout(() => void checkSync(), delay);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [addLog, editingServer, lastSyncAt, refreshLibrary, sourceId, syncStatus]);

  const sync = async () => {
    dispatch({ type: 'busy', busy: 'syncing', message: '' });
    try {
      await plexService.sync().catch(error => {
        if (!(error instanceof PlexRequestError && error.code === 'plex_sync_running')) throw error;
      });
      if (state.source) dispatch({ type: 'source', source: { ...state.source, lastSyncStatus: 'running', lastSyncError: undefined } });
      dispatch({ type: 'busy', busy: null });
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to start Plex synchronization' });
    }
  };

  const changeAutoSync = async (enabled: boolean) => {
    dispatch({ type: 'busy', busy: 'saving_schedule', message: '' });
    try {
      setAutoSync(await plexService.setAutoSync(enabled));
      dispatch({ type: 'busy', busy: null });
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to save the Plex sync schedule' });
    }
  };

  const cancelSync = async () => {
    dispatch({ type: 'busy', busy: 'canceling_sync', message: '' });
    try {
      await plexService.cancelSync();
      dispatch({ type: 'source', source: await plexService.getSyncStatus() });
      dispatch({ type: 'busy', busy: null });
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to cancel Plex synchronization' });
    }
  };

  const previewAIWriteback = async () => {
    setWritebackBusy(true); setWritebackError(''); setWritebackMessage(''); setWritebackApproved(false);
    try {
      const preview = await plexService.previewAIEnrichmentWriteback();
      setWritebackPreview(preview);
      if (preview.items.length === 0) {
        setWritebackMessage('No pending AI-enriched Plex metadata is available to review. Run Unified Enrichment to create new proposals.');
      }
    } catch (error) {
      setWritebackPreview(null);
      setWritebackError(error instanceof Error ? error.message : 'Unable to preview Plex metadata changes');
    } finally {
      setWritebackBusy(false);
    }
  };

  const syncAIWriteback = async () => {
    if (!writebackPreview || !writebackApproved) return;
    setWritebackBusy(true); setWritebackError(''); setWritebackMessage('');
    try {
      const result = await plexService.syncAIEnrichmentWriteback(writebackPreview.confirmation);
      setWritebackApproved(false);
      setWritebackPreview(null);
      const parts = [`${result.updated} updated`, `${result.verified} already matched`];
      if (result.failed > 0) {
        setWritebackError(`${result.failed} Plex metadata updates failed. ${result.errors?.join(' ') || 'Review the Plex connection and permissions, then preview again.'}`);
      } else {
        setWritebackMessage(`Plex AI metadata writeback complete: ${parts.join(', ')}. Plex fields were locked after verification.`);
        addLog('success', `[Plex] AI metadata writeback: ${parts.join(', ')}`);
      }
    } catch (error) {
      setWritebackError(error instanceof Error ? error.message : 'Unable to sync approved metadata to Plex');
    } finally {
      setWritebackBusy(false);
    }
  };

  const disconnect = async () => {
    const confirmed = window.confirm('Remove this Plex source from ViiB? This removes only ViiB’s cached catalog and credentials. Nothing is deleted or changed on the Plex server.');
    if (!confirmed) return;
    dispatch({ type: 'busy', busy: 'disconnecting', message: '' });
    try {
      clearTimers();
      await plexService.disconnect();
      await refreshLibrary();
      dispatch({ type: 'reset' });
      setEditingServer(false);
      addLog('success', '[Plex] Source removed from ViiB; Plex media was not modified');
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to remove Plex source' });
    }
  };

  const sourceNeedsAuth = plexSourceNeedsAuthentication(state.source, state.error);
  const sourceConnected = state.source?.available && !sourceNeedsAuth;
  const syncRunning = syncStatus === 'running';
  const busy = state.busy !== null || syncRunning;

  return (
    <section className="rounded-xl border border-surface-3 bg-surface-2 p-6">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Server className="text-brand" size={20} aria-hidden="true" />
          <div>
            <h2 className="text-lg font-bold text-text-main">Plex Media Server</h2>
            <p className="text-sm text-text-secondary">Use a Plex music library as part of the normal ViiB catalog and player.</p>
          </div>
        </div>
        {state.source && (
          <span className={`rounded-full px-3 py-1 text-xs font-semibold ${sourceConnected ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'}`}>
            {sourceNeedsAuth ? 'Sign-in required' : sourceConnected ? 'Connected' : 'Unavailable'}
          </span>
        )}
      </div>

      {(state.message || state.error || state.busy) && (
        <div className={`mt-4 flex items-start gap-2 rounded-lg border p-3 text-sm ${state.error ? 'border-error/40 bg-error/10 text-error' : 'border-surface-border bg-surface-1 text-text-secondary'}`} role="status">
          {state.busy ? <Loader2 className="mt-0.5 animate-spin" size={16} aria-hidden="true" /> : state.error ? <AlertTriangle className="mt-0.5" size={16} aria-hidden="true" /> : <ShieldCheck className="mt-0.5 text-success" size={16} aria-hidden="true" />}
          <span>{state.error || (state.busy ? busyLabel[state.busy] : state.message)}</span>
        </div>
      )}


      {authUrl && (
        <a href={authUrl} target="_blank" rel="noopener noreferrer" className="mt-3 inline-block text-sm font-semibold text-brand underline">
          Open Plex sign-in
        </a>
      )}

      {state.source && !editingServer ? (
        <div className="mt-5 space-y-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg bg-surface-1 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Server</div><div className="mt-1 font-semibold text-text-main">{state.source.name || state.source.machineIdentifier}</div></div>
            <div className="rounded-lg bg-surface-1 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Address</div><div className="mt-1 truncate font-mono text-xs text-text-main" title={state.source.baseUrl}>{state.source.baseUrl}</div></div>
            <div className="rounded-lg bg-surface-1 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Music library</div><div className="mt-1 font-semibold text-text-main">{state.source.libraryTitle || 'Not selected'}</div></div>
            <div className="rounded-lg bg-surface-1 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Last successful sync</div><div className="mt-1 text-sm text-text-main">{formatSyncTime(state.source.lastSyncAt)}</div></div>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" disabled={busy} onClick={() => { setEditingServer(true); void discover(); }} leftIcon={<Search size={15} aria-hidden="true" />}>Rediscover / Change Server</Button>
            <Button variant="secondary" size="sm" disabled={busy} onClick={() => { setEditingServer(true); if (state.authenticated) void loadAccountServers(); else void signIn(); }} leftIcon={<ExternalLink size={15} aria-hidden="true" />}>{state.authenticated ? 'Browse Plex Account Servers' : 'Sign in to Plex'}</Button>
            {sourceNeedsAuth && <Button variant="primary" size="sm" accent="brand" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={15} aria-hidden="true" />}>Sign in / Reconnect</Button>}
            <Button variant="secondary" size="sm" disabled={busy} onClick={() => void loadLibraries()} leftIcon={<RefreshCw size={15} aria-hidden="true" />}>Refresh Libraries</Button>
            <Button variant="secondary" size="sm" disabled={busy} onClick={() => void disconnect()} leftIcon={<Trash2 size={15} aria-hidden="true" />} className="hover:text-error">Remove Plex Source</Button>
          </div>

          {syncRunning && (
            <div className="flex items-center gap-2 text-sm text-text-secondary" role="status">
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
              <span>{state.source.syncTracks ? `Read ${state.source.syncTracks.toLocaleString()}${state.source.syncTotal ? ` of ${state.source.syncTotal.toLocaleString()}` : ''} Plex tracks` : 'Synchronizing Plex music…'}</span>
              <Button variant="secondary" size="sm" disabled={state.busy !== null} onClick={() => void cancelSync()}>Cancel sync</Button>
            </div>
          )}
          {!syncRunning && state.source.lastSyncError && (
            <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
              <AlertTriangle size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
              <span>{state.source.lastSyncError} ViiB kept any previously synced tracks.</span>
            </div>
          )}
        </div>
      ) : (
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button variant="primary" size="sm" accent="brand" disabled={busy} onClick={() => void discover()} leftIcon={<Search size={15} aria-hidden="true" />}>Search Local Network</Button>
            {!state.authenticated ? (
              <Button variant="secondary" size="sm" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={15} aria-hidden="true" />}>Sign in to Plex</Button>
            ) : (
              <Button variant="secondary" size="sm" disabled={busy} onClick={() => void loadAccountServers()} leftIcon={<RefreshCw size={15} aria-hidden="true" />}>Refresh Plex Account Servers</Button>
            )}
            {state.source && <Button variant="secondary" size="sm" disabled={busy} onClick={() => setEditingServer(false)}>Cancel Change</Button>}
          </div>

          {!state.authenticated && <p className="text-xs text-text-subtle">Sign in with your Plex account to find your remote servers and servers with libraries shared with you.</p>}

          {state.accountServers.length > 0 && (
            <div>
              <h3 className="mb-2 text-base font-semibold text-text-main">Plex account servers</h3>
              <div className="grid gap-2 md:grid-cols-2">
                {state.accountServers.map(server => (
                  <button key={server.machineIdentifier} type="button" disabled={busy} onClick={() => void connectAccountServer(server)} className="rounded-lg border border-surface-border bg-surface-1 p-3 text-left transition hover:border-brand disabled:opacity-50">
                    <div className="flex items-center justify-between gap-2"><div className="font-semibold text-text-main">{server.name || server.machineIdentifier}</div><span className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold ${server.owned ? 'bg-brand/10 text-brand' : 'bg-success/15 text-success'}`}>{server.owned ? 'Your server' : 'Shared with you'}</span></div>
                    <div className="mt-1 truncate font-mono text-xs text-text-subtle">{server.url}</div>
                    <div className="mt-1 text-xs text-text-secondary">{server.owner ? `Shared by ${server.owner}` : server.version ? `Plex ${server.version}` : 'Plex Media Server'}{server.relay ? ' · Relay connection' : ''}</div>
                  </button>
                ))}
              </div>
            </div>
          )}

          {state.discovered.length > 0 && (
            <div>
              <h3 className="mb-2 text-base font-semibold text-text-main">Discovered Plex servers</h3>
              <div className="grid gap-2 md:grid-cols-2">
                {state.discovered.map(server => (
                  <button key={server.machineIdentifier || serverAddress(server)} type="button" disabled={busy} onClick={() => void connect(serverAddress(server))} className="rounded-lg border border-surface-border bg-surface-1 p-3 text-left transition hover:border-brand disabled:opacity-50">
                    <div className="font-semibold text-text-main">{server.name || server.host}</div>
                    <div className="mt-1 font-mono text-xs text-text-subtle">{serverAddress(server)}</div>
                    <div className="mt-1 text-xs text-text-secondary">{server.version ? `Plex ${server.version}` : 'Plex Media Server'}{server.claimed ? ' · Claimed' : ' · Unclaimed'}</div>
                  </button>
                ))}
              </div>
            </div>
          )}

          <div>
            <label className="mb-2 block text-xs font-bold uppercase text-text-secondary">Manual Plex server</label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <TextInput value={state.manualUrl} onChange={event => dispatch({ type: 'manual_url', url: event.target.value })} placeholder="192.168.1.20 or https://plex.example.com" className="flex-1" inputClassName="font-mono" />
              <Button variant="secondary" size="sm" disabled={busy || !state.manualUrl.trim()} onClick={() => void connect(state.manualUrl)} leftIcon={<Server size={15} aria-hidden="true" />}>Connect</Button>
            </div>
            <p className="mt-2 text-xs text-text-subtle">Bare hosts/IP addresses default to port 32400. Full HTTP/HTTPS URLs are used as entered so reverse proxies on 80/443 work. The endpoint is validated as Plex before it is saved.</p>
          </div>
        </div>
      )}

      {state.source && !editingServer && (
        <div className="mt-6 border-t border-surface-border pt-5">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2"><Music className="text-brand" size={18} aria-hidden="true" /><h3 className="text-base font-semibold text-text-main">Music Library</h3></div>
            {state.source.libraryId && <Button variant="primary" size="sm" accent="brand" disabled={busy || sourceNeedsAuth} onClick={() => void sync()} leftIcon={<RefreshCw size={15} aria-hidden="true" />}>{state.source.lastSyncAt ? 'Resynchronize' : 'Synchronize'}</Button>}
          </div>

          {state.source.libraryId && (
            <label className="mb-3 flex items-center gap-2 text-sm text-text-secondary">
              <input type="checkbox" checked={autoSync} disabled={state.busy !== null} onChange={event => void changeAutoSync(event.target.checked)} />
              Sync automatically every hour while ViiB is open
            </label>
          )}

          {sourceNeedsAuth ? (
            <p className="text-sm text-text-secondary">Sign in to Plex to retrieve music libraries from this claimed server.</p>
          ) : state.libraries.length === 0 ? (
            <p className="text-sm text-text-secondary">No selectable music/audio libraries are available. Movie, TV, photo, and video libraries are intentionally excluded.</p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
              {state.libraries.map(library => {
                const selected = state.selectedLibraryId === library.id;
                return (
                  <button type="button" key={library.id} disabled={busy} onClick={() => void selectLibrary(library.id)} className={`rounded-lg border p-3 text-left transition disabled:opacity-50 ${selected ? 'border-brand bg-brand/10' : 'border-surface-border bg-surface-1 hover:border-brand/60'}`}>
                    <div className="flex items-center gap-2"><Music size={15} className={selected ? 'text-brand' : 'text-text-subtle'} aria-hidden="true" /><span className="font-semibold text-text-main">{library.title}</span></div>
                    <div className="mt-1 text-xs text-text-subtle">Music / audio only{selected ? ' · Selected' : ''}</div>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      {state.source && !editingServer && state.source.libraryId && (
        <div className="mt-6 border-t border-surface-border pt-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-text-main">AI Metadata Writeback</h3>
              <p className="mt-1 max-w-3xl text-sm text-text-secondary">Review AI-enriched genres and original release years before sending them to Plex. ViiB only writes approved changes, locks those fields in Plex, and verifies each update with a fresh PMS read.</p>
            </div>
            <Button variant="secondary" size="sm" disabled={busy || writebackBusy || sourceNeedsAuth} onClick={() => void previewAIWriteback()} leftIcon={<Eye size={15} aria-hidden="true" />}>{writebackBusy ? 'Loading preview…' : 'Preview AI metadata'}</Button>
          </div>

          {(writebackMessage || writebackError) && (
            <div className={`mt-3 rounded-lg border p-3 text-sm ${writebackError ? 'border-error/40 bg-error/10 text-error' : 'border-success/30 bg-success/10 text-text-main'}`} role="status">
              {writebackError || writebackMessage}
            </div>
          )}

          {writebackPreview && writebackPreview.items.length > 0 && (
            <div className="mt-4 rounded-lg border border-surface-border bg-surface-1 p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="text-sm font-semibold text-text-main">Review {writebackPreview.items.length} Plex track{writebackPreview.items.length === 1 ? '' : 's'}</div>
                {writebackPreview.hasMore && <span className="text-xs text-warning">Only the first 100 pending proposals are shown. Sync this set, then preview again.</span>}
              </div>
              <div className="mt-3 max-h-80 space-y-2 overflow-y-auto pr-1">
                {writebackPreview.items.map(item => (
                  <div key={item.songId} className="rounded-md border border-surface-border bg-surface-2 p-3 text-sm">
                    <div className="font-medium text-text-main">{item.title} <span className="font-normal text-text-secondary">· {item.artist}{item.album ? ` · ${item.album}` : ''}</span></div>
                    {item.changes.length > 0 ? (
                      <ul className="mt-2 space-y-1 text-xs text-text-secondary">
                        {item.changes.map(change => <li key={change.field}><span className="font-semibold text-text-main">{change.field === 'year' ? 'Release year' : 'Genres'}:</span> {formatWritebackValue(change.before)} → {formatWritebackValue(change.after)}</li>)}
                      </ul>
                    ) : <div className="mt-1 text-xs text-success">Already matches Plex; ViiB will only mark this proposal as verified.</div>}
                  </div>
                ))}
              </div>
              <label className="mt-4 flex cursor-pointer items-start gap-2 text-sm text-text-secondary">
                <input type="checkbox" className="mt-1" checked={writebackApproved} onChange={event => setWritebackApproved(event.target.checked)} />
                <span>I reviewed these changes and approve writing them to this Plex server. Plex stores them as locked metadata overrides; ViiB does not alter the source audio files.</span>
              </label>
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <Button variant="primary" size="sm" accent="brand" disabled={busy || writebackBusy || !writebackApproved} onClick={() => void syncAIWriteback()} leftIcon={<CheckCircle2 size={15} aria-hidden="true" />}>{writebackBusy ? 'Writing to Plex…' : 'Sync approved metadata to Plex'}</Button>
                <button type="button" className="text-sm text-text-secondary underline hover:text-text-main" disabled={writebackBusy} onClick={() => { setWritebackPreview(null); setWritebackApproved(false); }}>Discard preview</button>
              </div>
            </div>
          )}
        </div>
      )}

      <p className="mt-5 text-xs text-text-subtle">Plex media storage remains read-only: ViiB never deletes, moves, renames, or modifies source audio files or library configuration. When configured, AI Metadata Writeback creates only user-approved, locked metadata overrides in Plex. Plex video libraries are not supported.</p>
    </section>
  );
};

export default PlexMusicSourceSettings;
