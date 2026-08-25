import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AlertTriangle, ArrowLeft, Check, ExternalLink, FileMusic, Library, Loader2, Music, RefreshCw, Search, Server } from 'lucide-react';
import { plexService, type PlexLibrary, type PlexServer, type PlexSource } from '../services/plex';
import { useStore } from '../store';
import { Button } from './ui/Button';
import { TextInput } from './ui/TextInput';

type SetupStep = 'discovery' | 'libraries' | 'syncing' | 'complete';
type SetupStatus = 'idle' | 'discovering' | 'connecting' | 'authenticating' | 'syncing';

interface PlexFirstLaunchSetupProps {
  onBack: () => void;
  onComplete: () => void;
}

function serverAddress(server: PlexServer): string {
  return server.url || `${server.scheme || 'http'}://${server.host}:${server.port}`;
}

function StepProgress({ step }: { step: SetupStep }) {
  const current = step === 'discovery' ? 1 : step === 'libraries' ? 2 : 3;
  const labels = ['Find server', 'Choose library', 'Import music'];
  return (
    <div className="mb-7">
      <div className="flex items-center justify-between text-xs font-semibold uppercase tracking-wide text-text-subtle">
        <span>Plex setup</span><span>Step {current} of 3</span>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2" aria-label={`Plex setup step ${current} of 3`}>
        {labels.map((label, index) => (
          <div key={label} className="min-w-0">
            <div className={`h-1.5 rounded-full ${index < current ? 'bg-brand' : 'bg-surface-3'}`} />
            <div className={`mt-2 truncate text-xs ${index < current ? 'text-text-main' : 'text-text-subtle'}`}>{label}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function StatusNotice({ status, message, error }: { status: SetupStatus; message: string; error: string }) {
  if (!message && !error && status === 'idle') return null;
  const busy = status !== 'idle';
  return (
    <div className={`mt-5 flex items-start gap-2 rounded-lg border p-3 text-sm ${error ? 'border-error/40 bg-error/10 text-error' : 'border-surface-border bg-surface-1 text-text-secondary'}`} role="status">
      {busy ? <Loader2 size={16} className="mt-0.5 shrink-0 animate-spin" /> : error ? <AlertTriangle size={16} className="mt-0.5 shrink-0" /> : <Check size={16} className="mt-0.5 shrink-0 text-success" />}
      <span>{error || message}</span>
    </div>
  );
}

/** A staged Plex first-run flow that completes the initial import before exit. */
export const PlexFirstLaunchSetup: React.FC<PlexFirstLaunchSetupProps> = ({ onBack, onComplete }) => {
  const [step, setStep] = useState<SetupStep>('discovery');
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
        setMessage(result.warning || 'No Plex server was detected. Enter its hostname or IP address below.');
      } else if (result.servers.length === 1) {
        setMessage('We found a Plex server. Choose it to connect.');
      } else {
        setMessage(`We found ${result.servers.length} Plex servers. Choose the one with your music.`);
      }
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Plex discovery failed.');
      setMessage('You can still connect manually using the server hostname or IP address.');
    }
  }, []);

  const loadLibraries = useCallback(async () => {
    setStep('libraries');
    setStatus('connecting');
    setError('');
    setMessage('Loading music libraries…');
    try {
      const result = await plexService.getLibraries();
      if (disposed.current) return;
      setLibraries(result);
      setNeedsAuth(false);
      setStatus('idle');
      setMessage(result.length > 0 ? 'Choose the Plex music library ViiB should import.' : 'This server has no selectable music or audio libraries.');
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setNeedsAuth(true);
      setError(cause instanceof Error ? cause.message : 'Sign in to Plex to load its music libraries.');
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
            setMessage('The sign-in popup was blocked. The Plex sign-in URL was copied to your clipboard.');
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

  const pollSync = useCallback(async () => {
    if (disposed.current) return;
    try {
      const current = await plexService.getSyncStatus();
      if (!current) {
        setStatus('idle');
        setError('The Plex source was removed while importing.');
        return;
      }
      setSource(current);
      if (current.lastSyncStatus === 'running') {
        syncTimer.current = setTimeout(() => void pollSync(), 1200);
        return;
      }
      setStatus('idle');
      if (current.lastSyncStatus === 'complete') {
        await refreshLibrary();
        if (disposed.current) return;
        setStep('complete');
        setMessage(`Your Plex music is ready${current.libraryTitle ? ` from ${current.libraryTitle}` : ''}.`);
        addLog('success', `[Plex] First-run synchronization completed for ${current.libraryTitle || 'music library'}`);
      } else if (current.lastSyncStatus === 'auth_required') {
        setStep('libraries');
        setNeedsAuth(true);
        setError(current.lastSyncError || 'Plex authentication is required to finish the import.');
      } else {
        setError(current.lastSyncError || 'Plex import failed. Try again before finishing setup.');
      }
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to check Plex import status.');
    }
  }, [addLog, refreshLibrary]);

  const startInitialSync = useCallback(async () => {
    setStep('syncing');
    setStatus('syncing');
    setError('');
    setMessage('Importing your Plex catalog. Keep ViiB open until this finishes.');
    try {
      await plexService.sync();
      syncTimer.current = setTimeout(() => void pollSync(), 750);
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to start the Plex import.');
    }
  }, [pollSync]);

  const selectLibrary = async (library: PlexLibrary) => {
    setStatus('connecting');
    setError('');
    setMessage(`Selecting ${library.title}…`);
    try {
      await plexService.selectLibrary(library.id);
      const config = await plexService.getConfig();
      if (disposed.current) return;
      setSource(config.source);
      setSelectedLibraryId(library.id);
      await startInitialSync();
    } catch (cause) {
      if (disposed.current) return;
      setStatus('idle');
      setError(cause instanceof Error ? cause.message : 'Unable to select Plex music library.');
    }
  };

  const busy = status !== 'idle';
  const title = step === 'discovery' ? 'Find your Plex server' : step === 'libraries' ? 'Choose your music library' : step === 'syncing' ? 'Importing your music' : 'Your Plex library is ready';
  const subtitle = step === 'discovery'
    ? 'ViiB is looking on your computer and local network. You can also add a server manually.'
    : step === 'libraries'
      ? 'Select one Plex music library for ViiB to make available right away.'
      : step === 'syncing'
        ? 'We are importing the catalog now, so you will not arrive at an empty library.'
        : 'Your music is available in ViiB now.';

  return (
    <div>
      <StepProgress step={step} />
      <div className="flex items-start gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-brand/10 text-brand">
          {step === 'discovery' ? <Server size={22} /> : step === 'libraries' ? <Library size={22} /> : step === 'syncing' ? <Loader2 size={22} className="animate-spin" /> : <Check size={22} />}
        </div>
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-text-main">{title}</h2>
          <p className="mt-1 text-sm leading-6 text-text-secondary">{subtitle}</p>
        </div>
      </div>

      <StatusNotice status={status} message={message} error={error} />

      {step === 'discovery' && (
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" disabled={busy} onClick={() => void discover(false)} leftIcon={<Search size={16} />}>Search again</Button>
          </div>
          {servers.length > 0 && (
            <div className="grid gap-2 sm:grid-cols-2">
              {servers.map(server => (
                <button
                  type="button"
                  key={server.machineIdentifier || serverAddress(server)}
                  disabled={busy}
                  onClick={() => void connect(serverAddress(server))}
                  className="rounded-lg border border-surface-border bg-surface-1 p-3 text-left transition hover:border-brand hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50"
                >
                  <div className="font-semibold text-text-main">{server.name || server.host}</div>
                  <div className="mt-1 truncate font-mono text-xs text-text-subtle">{serverAddress(server)}</div>
                  <div className="mt-1 text-xs text-text-secondary">{server.version ? `Plex ${server.version}` : 'Plex Media Server'}</div>
                </button>
              ))}
            </div>
          )}
          <div className="rounded-xl border border-surface-border bg-surface-1 p-4">
            <label className="mb-2 block text-xs font-bold uppercase tracking-wide text-text-secondary">Add a server manually</label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <TextInput value={manualUrl} onChange={event => setManualUrl(event.target.value)} placeholder="192.168.1.20 or plex-server.local" className="flex-1" inputClassName="font-mono" />
              <Button variant="primary" accent="brand" disabled={busy || !manualUrl.trim()} onClick={() => void connect(manualUrl)} leftIcon={<Server size={16} />}>Connect</Button>
            </div>
          </div>
        </div>
      )}

      {step === 'libraries' && (
        <div className="mt-5 space-y-4">
          {source && (
            <div className="rounded-xl border border-surface-border bg-surface-1 p-4">
              <div className="text-xs font-bold uppercase tracking-wide text-text-subtle">Connected server</div>
              <div className="mt-1 font-semibold text-text-main">{source.name || source.machineIdentifier}</div>
              <div className="mt-1 truncate font-mono text-xs text-text-secondary">{source.baseUrl}</div>
            </div>
          )}
          {(needsAuth || (!authenticated && libraries.length === 0 && error)) && (
            <Button variant="primary" accent="brand" disabled={busy} onClick={() => void signIn()} leftIcon={<ExternalLink size={16} />}>Sign in to Plex</Button>
          )}
          {!needsAuth && libraries.length > 0 && (
            <div className="grid gap-2 sm:grid-cols-2">
              {libraries.map(library => {
                const selected = selectedLibraryId === library.id;
                return (
                  <button
                    type="button"
                    key={library.id}
                    disabled={busy}
                    onClick={() => void selectLibrary(library)}
                    className={`rounded-xl border p-4 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50 ${selected ? 'border-brand bg-brand/10' : 'border-surface-border bg-surface-1 hover:border-brand/60 hover:bg-surface-2'}`}
                  >
                    <div className="flex items-center gap-2"><Music size={16} className={selected ? 'text-brand' : 'text-text-subtle'} /><span className="font-semibold text-text-main">{library.title}</span></div>
                    <div className="mt-1 text-xs text-text-subtle">Music / audio{selected ? ' · selected' : ''}</div>
                  </button>
                );
              })}
            </div>
          )}
          <Button variant="secondary" disabled={busy} onClick={() => void loadLibraries()} leftIcon={<RefreshCw size={16} />}>Refresh libraries</Button>
        </div>
      )}

      {step === 'syncing' && (
        <div className="mt-6 rounded-xl border border-brand/25 bg-brand/5 p-5">
          <div className="flex items-center gap-3 text-text-main"><FileMusic size={20} className="text-brand" /><span className="font-medium">{source?.libraryTitle || 'Plex music library'}</span></div>
          <p className="mt-3 text-sm leading-6 text-text-secondary">ViiB imports your music metadata and artwork references without changing anything in Plex. This window will continue as soon as the import completes.</p>
          {error && <Button variant="primary" accent="brand" className="mt-4" onClick={() => void startInitialSync()} leftIcon={<RefreshCw size={16} />}>Try import again</Button>}
        </div>
      )}

      {step === 'complete' && (
        <div className="mt-6 rounded-xl border border-success/30 bg-success/10 p-5">
          <div className="flex items-center gap-3"><Check size={22} className="text-success" /><div><div className="font-semibold text-text-main">Initial import complete</div><div className="mt-1 text-sm text-text-secondary">Open ViiB to start listening.</div></div></div>
        </div>
      )}

      <div className="mt-7 flex items-center justify-between border-t border-surface-border pt-5">
        {step === 'discovery' ? (
          <Button variant="ghost" onClick={onBack} className="px-0" leftIcon={<ArrowLeft size={16} />}>Change source</Button>
        ) : (
          <span className="text-xs text-text-subtle">Plex stays read-only. You can change it later in Settings.</span>
        )}
        {step === 'complete' && <Button variant="primary" accent="brand" onClick={onComplete} rightIcon={<Check size={16} />}>Open my library</Button>}
      </div>
    </div>
  );
};

export default PlexFirstLaunchSetup;
