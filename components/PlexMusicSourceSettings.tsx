import React, { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { AlertTriangle, ExternalLink, Loader2, Music, RefreshCw, Search, Server, ShieldCheck, Trash2, WifiOff } from 'lucide-react';
import { Button } from './ui/Button';
import { TextInput } from './ui/TextInput';
import { useStore } from '../store';
import { plexService, type PlexAccountServer, type PlexMetadataWritebackPreview, type PlexServer } from '../services/plex';
import { initialPlexSettingsState, plexSettingsReducer, plexSourceNeedsAuthentication } from '../lib/plexSettingsState';

const busyLabel: Record<string, string> = {
  loading: 'Loading Plex configuration…',
  discovering: 'Searching the local network…',
  account_servers: 'Finding servers available to your Plex account…',
  connecting: 'Validating Plex Media Server…',
  authenticating: 'Waiting for Plex sign-in…',
  libraries: 'Loading music libraries…',
  saving: 'Saving music library…',
  syncing: 'Synchronizing Plex music…',
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
  const authTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const syncTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disposed = useRef(false);
  const refreshLibrary = useStore(store => store.refreshLibrary);
  const addLog = useStore(store => store.addLog);

  const clearTimers = useCallback(() => {
    if (authTimer.current) clearTimeout(authTimer.current);
    if (syncTimer.current) clearTimeout(syncTimer.current);
    authTimer.current = null;
    syncTimer.current = null;
  }, []);

  const loadLibraries = useCallback(async () => {
    dispatch({ type: 'busy', busy: 'libraries' });
    try {
      const libraries = await plexService.getLibraries();
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
        dispatch({ type: 'message', message: 'Your Plex account has no reachable media servers or shared servers right now.' });
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
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      if (!config.source) {
        if (config.authenticated) await loadAccountServers();
        return;
      }
      try {
        const libraries = await plexService.getLibraries();
        if (!disposed.current) dispatch({ type: 'libraries', libraries });
      } catch (error) {
        if (!disposed.current) dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Plex authentication may be required' });
      }
    } catch (error) {
      if (!disposed.current) dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to load Plex configuration' });
    }
  }, [loadAccountServers]);

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
      const server = await plexService.connect(url.trim());
      addLog('success', `[Plex] Connected to ${server.name || server.host}`);
      setEditingServer(false);
      const config = await plexService.getConfig();
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      try {
        dispatch({ type: 'libraries', libraries: await plexService.getLibraries() });
      } catch (error) {
        dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Sign in to Plex to view this server’s music libraries' });
      }
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to connect to Plex Media Server' });
    }
  };

  const connectAccountServer = async (server: PlexAccountServer) => {
    dispatch({ type: 'busy', busy: 'connecting', message: '' });
    try {
      const connected = await plexService.connectAccountServer(server.machineIdentifier);
      addLog('success', `[Plex] Connected to ${connected.name || server.name || connected.machineIdentifier}`);
      setEditingServer(false);
      const config = await plexService.getConfig();
      dispatch({ type: 'loaded', source: config.source, authenticated: config.authenticated });
      dispatch({ type: 'libraries', libraries: await plexService.getLibraries() });
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to connect to Plex account server' });
    }
  };

  const pollAuthentication = useCallback(async (expiresAt: number) => {
    if (disposed.current) return;
    try {
      const status = await plexService.getAuthStatus();
      if (status.authenticated) {
        dispatch({ type: 'authenticated', authenticated: true });
        addLog('success', '[Plex] Authentication completed');
        await loadConfig();
        dispatch({ type: 'message', message: 'Plex sign-in complete. Your available servers are ready to choose.' });
        return;
      }
      if (!status.pending || Date.now() >= expiresAt * 1000) {
        dispatch({ type: 'error', error: status.message || 'Plex sign-in expired. Start sign-in again.' });
        return;
      }
      authTimer.current = setTimeout(() => void pollAuthentication(expiresAt), 2000);
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to check Plex sign-in status' });
    }
  }, [addLog, loadConfig]);

  const signIn = async () => {
    dispatch({ type: 'busy', busy: 'authenticating', message: '' });
    // Open a blank window synchronously while this click still has a user
    // gesture. Opening only after the network request is commonly popup-blocked.
    const authWindow = window.open('', '_blank', 'noopener,noreferrer');
    try {
      const start = await plexService.startAuth();
      if (authWindow) {
        authWindow.location.href = start.authUrl;
      } else {
        const secondAttempt = window.open(start.authUrl, '_blank', 'noopener,noreferrer');
        if (!secondAttempt) {
          try {
            await navigator.clipboard.writeText(start.authUrl);
            dispatch({ type: 'message', message: 'The Plex sign-in popup was blocked. The official Plex authorization URL was copied to your clipboard.' });
          } catch {
            dispatch({ type: 'message', message: 'The Plex sign-in popup was blocked. Allow popups for ViiB and choose Sign in / Reconnect again.' });
          }
        }
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
    dispatch({ type: 'selected_library', id: libraryId });
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

  const pollSync = useCallback(async () => {
    if (disposed.current) return;
    try {
      const source = await plexService.getSyncStatus();
      dispatch({ type: 'source', source });
      if (!source) {
        dispatch({ type: 'error', error: 'Plex source was removed while synchronizing.' });
        return;
      }
      if (source.lastSyncStatus === 'running') {
        syncTimer.current = setTimeout(() => void pollSync(), 1500);
        return;
      }
      if (source.lastSyncStatus === 'complete') {
        await refreshLibrary();
        dispatch({ type: 'message', message: 'Plex music synchronization complete.' });
        addLog('success', `[Plex] Synchronized ${source.libraryTitle || 'music library'}`);
        return;
      }
      dispatch({ type: 'error', error: source.lastSyncError || (source.lastSyncStatus === 'auth_required' ? 'Plex authentication is required. Reconnect and try again.' : 'Plex synchronization failed.') });
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to check Plex synchronization status' });
    }
  }, [addLog, refreshLibrary]);

  const sync = async () => {
    dispatch({ type: 'busy', busy: 'syncing', message: '' });
    try {
      await plexService.sync();
      syncTimer.current = setTimeout(() => void pollSync(), 1000);
    } catch (error) {
      dispatch({ type: 'error', error: error instanceof Error ? error.message : 'Unable to start Plex synchronization' });
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
  const busy = state.busy !== null;

  return (
    <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Server className="text-brand" size={21} />
          <div>
            <h2 className="text-xl font-semibold text-text-main">Plex Media Server</h2>
            <p className="text-sm text-text-secondary">Use a Plex music library as part of the normal ViiB catalog and player.</p>
          </div>
        </div>
        {state.source && (
          <span className={`rounded-full px-3 py-1 text-xs font-semibold ${state.source.available ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'}`}>
            {state.source.available ? 'Connected' : 'Offline / unavailable'}
          </span>
        )}
      </div>

      {(state.message || state.error || state.busy) && (
        <div className={`mt-4 flex items-start gap-2 rounded-lg border p-3 text-sm ${state.error ? 'border-error/40 bg-error/10 text-error' : 'border-surface-border bg-surface-2 text-text-secondary'}`} role="status">
          {state.busy ? <Loader2 className="mt-0.5 animate-spin" size={16} /> : state.error ? <AlertTriangle className="mt-0.5" size={16} /> : <ShieldCheck className="mt-0.5 text-success" size={16} />}
          <span>{state.error || (state.busy ? busyLabel[state.busy] : state.message)}</span>
        </div>
      )}

      {state.source && !editingServer ? (
        <div className="mt-5 space-y-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg bg-surface-2 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Server</div><div className="mt-1 font-semibold text-text-main">{state.source.name || state.source.machineIdentifier}</div></div>
            <div className="rounded-lg bg-surface-2 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Address</div><div className="mt-1 truncate font-mono text-xs text-text-main" title={state.source.baseUrl}>{state.source.baseUrl}</div></div>
            <div className="rounded-lg bg-surface-2 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Music library</div><div className="mt-1 font-semibold text-text-main">{state.source.libraryTitle || 'Not selected'}</div></div>
            <div className="rounded-lg bg-surface-2 p-3"><div className="text-xs uppercase tracking-wide text-text-subtle">Last sync</div><div className="mt-1 text-sm text-text-main">{formatSyncTime(state.source.lastSyncAt)}</div></div>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" disabled={busy} onClick={() => { setEditingServer(true); void discover(); }} leftIcon={<Search size={16} />}>Rediscover / Change Server</Button>
            <Button variant="secondary" disabled={busy} onClick={() => { setEditingServer(true); if (state.authenticated) void loadAccountServers(); else void signIn(); }} leftIcon={<ExternalLink size={16} />}>{state.authenticated ? 'Browse Plex Account Servers' : 'Sign in to Plex'}</Button>
            {sourceNeedsAuth && <Button variant="primary" accent="brand" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={16} />}>Sign in / Reconnect</Button>}
            <Button variant="secondary" disabled={busy} onClick={() => void loadLibraries()} leftIcon={<RefreshCw size={16} />}>Refresh Libraries</Button>
            <Button variant="secondary" disabled={busy} onClick={() => void disconnect()} leftIcon={<Trash2 size={16} />} className="hover:text-error">Remove Plex Source</Button>
          </div>

          {state.source.lastSyncStatus === 'error' && state.source.lastSyncError && (
            <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
              <WifiOff size={16} className="mt-0.5" />
              <span>{state.source.lastSyncError}. Cached Plex music remains in ViiB until a later successful sync confirms changes.</span>
            </div>
          )}
        </div>
      ) : (
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button variant="primary" accent="brand" disabled={busy} onClick={() => void discover()} leftIcon={<Search size={16} />}>Search Local Network</Button>
            {!state.authenticated ? (
              <Button variant="secondary" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={16} />}>Sign in to Plex</Button>
            ) : (
              <Button variant="secondary" disabled={busy} onClick={() => void loadAccountServers()} leftIcon={<RefreshCw size={16} />}>Refresh Plex Account Servers</Button>
            )}
            {state.source && <Button variant="secondary" disabled={busy} onClick={() => setEditingServer(false)}>Cancel Change</Button>}
          </div>

          {!state.authenticated && <p className="text-xs text-text-subtle">Sign in with your Plex account to find your remote servers and servers with libraries shared with you.</p>}

          {state.accountServers.length > 0 && (
            <div>
              <h3 className="mb-2 text-sm font-semibold text-text-main">Plex account servers</h3>
              <div className="grid gap-2 md:grid-cols-2">
                {state.accountServers.map(server => (
                  <button key={server.machineIdentifier} type="button" disabled={busy} onClick={() => void connectAccountServer(server)} className="rounded-lg border border-surface-border bg-surface-2 p-3 text-left transition hover:border-brand disabled:opacity-50">
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
              <h3 className="mb-2 text-sm font-semibold text-text-main">Discovered Plex servers</h3>
              <div className="grid gap-2 md:grid-cols-2">
                {state.discovered.map(server => (
                  <button key={server.machineIdentifier || serverAddress(server)} type="button" disabled={busy} onClick={() => void connect(serverAddress(server))} className="rounded-lg border border-surface-border bg-surface-2 p-3 text-left transition hover:border-brand disabled:opacity-50">
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
              <Button variant="secondary" disabled={busy || !state.manualUrl.trim()} onClick={() => void connect(state.manualUrl)} leftIcon={<Server size={16} />}>Connect</Button>
            </div>
            <p className="mt-2 text-xs text-text-subtle">Bare hosts/IP addresses default to port 32400. Full HTTP/HTTPS URLs are used as entered so reverse proxies on 80/443 work. The endpoint is validated as Plex before it is saved.</p>
          </div>
        </div>
      )}

      {state.source && !editingServer && (
        <div className="mt-6 border-t border-surface-border pt-5">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2"><Music className="text-brand" size={18} /><h3 className="font-semibold text-text-main">Music Library</h3></div>
            {state.source.libraryId && <Button variant="primary" accent="brand" disabled={busy || sourceNeedsAuth} onClick={() => void sync()} leftIcon={<RefreshCw size={16} />}>{state.source.lastSyncAt ? 'Resynchronize' : 'Synchronize'}</Button>}
          </div>

          {sourceNeedsAuth ? (
            <p className="text-sm text-text-secondary">Sign in to Plex to retrieve music libraries from this claimed server.</p>
          ) : state.libraries.length === 0 ? (
            <p className="text-sm text-text-secondary">No selectable music/audio libraries are available. Movie, TV, photo, and video libraries are intentionally excluded.</p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
              {state.libraries.map(library => {
                const selected = state.selectedLibraryId === library.id || state.source?.libraryId === library.id;
                return (
                  <button type="button" key={library.id} disabled={busy} onClick={() => void selectLibrary(library.id)} className={`rounded-lg border p-3 text-left transition disabled:opacity-50 ${selected ? 'border-brand bg-brand/10' : 'border-surface-border bg-surface-2 hover:border-brand/60'}`}>
                    <div className="flex items-center gap-2"><Music size={15} className={selected ? 'text-brand' : 'text-text-subtle'} /><span className="font-semibold text-text-main">{library.title}</span></div>
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
              <h3 className="font-semibold text-text-main">AI Metadata Writeback</h3>
              <p className="mt-1 max-w-3xl text-sm text-text-secondary">Review AI-enriched genres and original release years before sending them to Plex. ViiB only writes approved changes, locks those fields in Plex, and verifies each update with a fresh PMS read.</p>
            </div>
            <Button variant="secondary" disabled={busy || writebackBusy || sourceNeedsAuth} onClick={() => void previewAIWriteback()} leftIcon={<Eye size={16} />}>{writebackBusy ? 'Loading preview…' : 'Preview AI metadata'}</Button>
          </div>

          {(writebackMessage || writebackError) && (
            <div className={`mt-3 rounded-lg border p-3 text-sm ${writebackError ? 'border-error/40 bg-error/10 text-error' : 'border-success/30 bg-success/10 text-text-main'}`} role="status">
              {writebackError || writebackMessage}
            </div>
          )}

          {writebackPreview && writebackPreview.items.length > 0 && (
            <div className="mt-4 rounded-lg border border-surface-border bg-surface-2 p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="text-sm font-semibold text-text-main">Review {writebackPreview.items.length} Plex track{writebackPreview.items.length === 1 ? '' : 's'}</div>
                {writebackPreview.hasMore && <span className="text-xs text-warning">Only the first 100 pending proposals are shown. Sync this set, then preview again.</span>}
              </div>
              <div className="mt-3 max-h-80 space-y-2 overflow-y-auto pr-1">
                {writebackPreview.items.map(item => (
                  <div key={item.songId} className="rounded-md border border-surface-border bg-surface-1 p-3 text-sm">
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
                <Button variant="primary" accent="brand" disabled={busy || writebackBusy || !writebackApproved} onClick={() => void syncAIWriteback()} leftIcon={<CheckCircle2 size={16} />}>{writebackBusy ? 'Writing to Plex…' : 'Sync approved metadata to Plex'}</Button>
                <button type="button" className="text-sm text-text-secondary underline hover:text-text-main" disabled={writebackBusy} onClick={() => { setWritebackPreview(null); setWritebackApproved(false); }}>Discard preview</button>
              </div>
            </div>
          )}
        </div>
      )}

      <p className="mt-5 text-xs text-text-subtle">Plex media storage remains read-only: ViiB never deletes, moves, renames, or modifies source audio files or library configuration. The optional AI Metadata Writeback above is the sole exception: it creates user-approved, locked metadata overrides in Plex. Plex video libraries are not supported.</p>
    </section>
  );
};

export default PlexMusicSourceSettings;
