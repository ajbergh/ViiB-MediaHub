import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AlertTriangle, Check, ExternalLink, Loader2, Music, RefreshCw, Search, Server } from 'lucide-react';
import { plexService, type PlexLibrary, type PlexServer, type PlexSource } from '../services/plex';
import { useStore } from '../store';
import { Button } from './ui/Button';
import { TextInput } from './ui/TextInput';

type SetupStatus = 'idle' | 'discovering' | 'connecting' | 'authenticating' | 'libraries' | 'syncing';

function serverAddress(server: PlexServer): string {
  return server.url || `${server.scheme || 'http'}://${server.host}:${server.port}`;
}

export const PlexFirstLaunchSetup: React.FC = () => {
  const [status, setStatus] = useState<SetupStatus>('idle');
  const [servers, setServers] = useState<PlexServer[]>([]);
  const [source, setSource] = useState<PlexSource | null>(null);
  const [authenticated, setAuthenticated] = useState(false);
  const [needsAuth, setNeedsAuth] = useState(false);
  const [libraries, setLibraries] = useState<PlexLibrary[]>([]);
  const [manualUrl, setManualUrl] = useState('');
  const [selectedLibraryId, setSelectedLibraryId] = useState('');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
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
    setStatus('libraries');
    setError('');
    try {
      const result = await plexService.getLibraries();
      if (disposed.current) return;
      setLibraries(result);
      setNeedsAuth(false);
      setStatus('idle');
      if (result.length === 0) {
        setMessage('Connected, but this server has no selectable music/audio libraries.');
      } else {
        setMessage('Choose the Plex music library you want ViiB to use.');
      }
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setNeedsAuth(true);
      setError(cause instanceof Error ? cause.message : 'Sign in to Plex to load music libraries.');
    }
  }, []);

  const discover = useCallback(async (automatic = false) => {
    setStatus('discovering');
    setError('');
    setMessage(automatic ? 'Looking for Plex Media Server on this computer and local network…' : 'Searching this computer and local network for Plex Media Server…');
    try {
      const result = await plexService.discover();
      if (disposed.current) return;
      setServers(result.servers);
      setStatus('idle');
      if (result.servers.length === 0) {
        setMessage(result.warning || 'No Plex server was detected automatically. Enter its hostname or IP address below; manual setup does not depend on network discovery.');
      } else if (result.servers.length === 1) {
        setMessage('Found a Plex server. Select it below to connect.');
      } else {
        setMessage(`Found ${result.servers.length} Plex servers. Select the one containing your music.`);
      }
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Plex discovery failed.');
      setMessage('You can still connect manually using the server hostname or IP address.');
    }
  }, []);

  const loadConfig = useCallback(async () => {
    try {
      const config = await plexService.getConfig();
      if (disposed.current) return;
      setSource(config.source);
      setAuthenticated(config.authenticated);
      setSelectedLibraryId(config.source?.libraryId || '');
      if (config.source) {
        await loadLibraries();
      } else {
        await discover(true);
      }
    } catch (cause) {
      if (disposed.current) return;
      setError(cause instanceof Error ? cause.message : 'Unable to load Plex configuration.');
      await discover(true);
    }
  }, [discover, loadLibraries]);

  useEffect(() => {
    disposed.current = false;
    void loadConfig();
    return () => {
      disposed.current = true;
      clearTimers();
    };
  }, [clearTimers, loadConfig]);

  const connect = async (url: string) => {
    if (!url.trim()) return;
    setStatus('connecting');
    setError('');
    setMessage('Validating Plex Media Server…');
    try {
      const server = await plexService.connect(url.trim());
      const config = await plexService.getConfig();
      if (disposed.current) return;
      setSource(config.source);
      setAuthenticated(config.authenticated);
      setNeedsAuth(server.authRequired && !config.authenticated);
      setSelectedLibraryId(config.source?.libraryId || '');
      setMessage(`Connected to ${server.name || server.host}.`);
      addLog('success', `[Plex] First-run setup connected to ${server.name || server.host}`);
      await loadLibraries();
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to connect to Plex Media Server.');
    }
  };

  const pollAuthentication = useCallback(async (expiresAt: number) => {
    if (disposed.current) return;
    try {
      const result = await plexService.getAuthStatus();
      if (result.authenticated) {
        setAuthenticated(true);
        setNeedsAuth(false);
        setStatus('idle');
        setError('');
        setMessage('Plex sign-in complete. Loading music libraries…');
        addLog('success', '[Plex] First-run authentication completed');
        const config = await plexService.getConfig();
        if (!disposed.current) setSource(config.source);
        await loadLibraries();
        return;
      }
      if (!result.pending || Date.now() >= expiresAt * 1000) {
        setStatus('idle');
        setError(result.message || 'Plex sign-in expired. Choose Sign in to Plex to try again.');
        return;
      }
      authTimer.current = setTimeout(() => void pollAuthentication(expiresAt), 2000);
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to check Plex sign-in status.');
    }
  }, [addLog, loadLibraries]);

  const signIn = async () => {
    setStatus('authenticating');
    setError('');
    setMessage('Opening the official Plex sign-in page…');
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
            setMessage('The sign-in popup was blocked. The official Plex authorization URL was copied to your clipboard.');
          } catch {
            setMessage('The sign-in popup was blocked. Allow popups and choose Sign in to Plex again.');
          }
        }
      }
      authTimer.current = setTimeout(() => void pollAuthentication(start.expiresAt), 1500);
    } catch (cause) {
      authWindow?.close();
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to start Plex sign-in.');
    }
  };

  const selectLibrary = async (library: PlexLibrary) => {
    setStatus('libraries');
    setError('');
    try {
      await plexService.selectLibrary(library.id);
      const config = await plexService.getConfig();
      if (disposed.current) return;
      setSource(config.source);
      setSelectedLibraryId(library.id);
      setStatus('idle');
      setMessage(`${library.title} selected. Synchronize it to import the Plex catalog into ViiB.`);
    } catch (cause) {
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to select Plex music library.');
    }
  };

  const pollSync = useCallback(async () => {
    if (disposed.current) return;
    try {
      const current = await plexService.getSyncStatus();
      if (!current) {
        setStatus('idle');
        setError('The Plex source was removed while synchronizing.');
        return;
      }
      setSource(current);
      if (current.lastSyncStatus === 'running') {
        syncTimer.current = setTimeout(() => void pollSync(), 1500);
        return;
      }
      setStatus('idle');
      if (current.lastSyncStatus === 'complete') {
        await refreshLibrary();
        setMessage(`Plex music synchronization complete${current.libraryTitle ? `: ${current.libraryTitle}` : ''}.`);
        addLog('success', `[Plex] First-run synchronization completed for ${current.libraryTitle || 'music library'}`);
      } else {
        setError(current.lastSyncError || 'Plex synchronization failed. You can retry later in Settings.');
      }
    } catch (cause) {
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to check Plex synchronization status.');
    }
  }, [addLog, refreshLibrary]);

  const sync = async () => {
    setStatus('syncing');
    setError('');
    setMessage('Starting Plex music synchronization…');
    try {
      await plexService.sync();
      syncTimer.current = setTimeout(() => void pollSync(), 1000);
    } catch (cause) {
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to start Plex synchronization.');
    }
  };

  const busy = status !== 'idle';

  return (
    <div className="rounded-xl border border-surface-border bg-surface-1 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="rounded-lg bg-brand/10 p-3"><Server size={22} className="text-brand" /></div>
          <div>
            <h3 className="font-bold text-text-main">Plex Media Server</h3>
            <p className="mt-1 text-sm text-text-secondary">Optional — use a Plex music library instead of, or alongside, local folders.</p>
          </div>
        </div>
        {source && <span className="rounded-full bg-success/15 px-3 py-1 text-xs font-semibold text-success">Connected</span>}
      </div>

      {(status !== 'idle' || message || error) && (
        <div className={`mt-4 flex items-start gap-2 rounded-lg border p-3 text-sm ${error ? 'border-error/40 bg-error/10 text-error' : 'border-surface-border bg-surface-2 text-text-secondary'}`} role="status">
          {busy ? <Loader2 size={16} className="mt-0.5 animate-spin" /> : error ? <AlertTriangle size={16} className="mt-0.5" /> : <Check size={16} className="mt-0.5 text-success" />}
          <span>{error || message}</span>
        </div>
      )}

      {!source ? (
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" disabled={busy} onClick={() => void discover(false)} leftIcon={<Search size={16} />}>Search Again</Button>
          </div>

          {servers.length > 0 && (
            <div className="grid gap-2 sm:grid-cols-2">
              {servers.map(server => (
                <button
                  type="button"
                  key={server.machineIdentifier || serverAddress(server)}
                  disabled={busy}
                  onClick={() => void connect(serverAddress(server))}
                  className="rounded-lg border border-surface-border bg-surface-2 p-3 text-left transition hover:border-brand disabled:opacity-50"
                >
                  <div className="font-semibold text-text-main">{server.name || server.host}</div>
                  <div className="mt-1 truncate font-mono text-xs text-text-subtle">{serverAddress(server)}</div>
                  <div className="mt-1 text-xs text-text-secondary">{server.version ? `Plex ${server.version}` : 'Plex Media Server'}</div>
                </button>
              ))}
            </div>
          )}

          <div>
            <label className="mb-2 block text-xs font-bold uppercase text-text-secondary">Manual server address</label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <TextInput value={manualUrl} onChange={event => setManualUrl(event.target.value)} placeholder="192.168.1.20 or plex-server.local" className="flex-1" inputClassName="font-mono" />
              <Button variant="secondary" disabled={busy || !manualUrl.trim()} onClick={() => void connect(manualUrl)} leftIcon={<Server size={16} />}>Connect</Button>
            </div>
            <p className="mt-2 text-xs text-text-subtle">A bare hostname or IP uses Plex’s standard port 32400. Automatic discovery now tries Plex GDM plus a bounded standard-port check, but manual entry remains the most reliable fallback on networks that filter discovery traffic.</p>
          </div>
        </div>
      ) : (
        <div className="mt-5 space-y-4">
          <div className="rounded-lg bg-surface-2 p-3">
            <div className="text-xs font-bold uppercase text-text-subtle">Connected server</div>
            <div className="mt-1 font-semibold text-text-main">{source.name || source.machineIdentifier}</div>
            <div className="mt-1 truncate font-mono text-xs text-text-secondary">{source.baseUrl}</div>
          </div>

          {(needsAuth || (!authenticated && libraries.length === 0 && error)) && (
            <Button variant="primary" accent="brand" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={16} />}>Sign in to Plex</Button>
          )}

          {!needsAuth && libraries.length > 0 && (
            <div>
              <div className="mb-2 text-xs font-bold uppercase text-text-secondary">Music library</div>
              <div className="grid gap-2 sm:grid-cols-2">
                {libraries.map(library => {
                  const selected = selectedLibraryId === library.id;
                  return (
                    <button
                      type="button"
                      key={library.id}
                      disabled={busy}
                      onClick={() => void selectLibrary(library)}
                      className={`rounded-lg border p-3 text-left transition disabled:opacity-50 ${selected ? 'border-brand bg-brand/10' : 'border-surface-border bg-surface-2 hover:border-brand/60'}`}
                    >
                      <div className="flex items-center gap-2"><Music size={15} className={selected ? 'text-brand' : 'text-text-subtle'} /><span className="font-semibold text-text-main">{library.title}</span></div>
                      <div className="mt-1 text-xs text-text-subtle">Music / audio{selected ? ' · Selected' : ''}</div>
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            {selectedLibraryId && <Button variant="primary" accent="brand" disabled={busy || needsAuth} onClick={() => void sync()} leftIcon={<RefreshCw size={16} />}>{source.lastSyncAt ? 'Resynchronize' : 'Synchronize Plex Music'}</Button>}
            <Button variant="secondary" disabled={busy} onClick={() => void loadLibraries()} leftIcon={<RefreshCw size={16} />}>Refresh Libraries</Button>
          </div>
        </div>
      )}

      <p className="mt-4 text-xs text-text-subtle">Plex access is read-only. ViiB imports the catalog and streams through the Plex server; it does not move, delete, rename, or modify your Plex media.</p>
    </div>
  );
};

export default PlexFirstLaunchSetup;
