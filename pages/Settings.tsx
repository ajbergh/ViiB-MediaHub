
import React, { useState, useRef } from 'react';
import { Wifi, Volume2, HardDrive, Trash2, Terminal, XCircle, SlidersHorizontal, Activity, Layers, Sparkles, FolderOpen, Loader2, AlertTriangle } from 'lucide-react';
import { useStore } from '../store';
import { VisualizerMode, Song } from '../types';
import { parseSong } from '../metadata';

export const Settings: React.FC = () => {
  const { 
      audioSettings, setCrossfade, setGapless, setNormalization, 
      setVisualizerMode, setEqEnabled, toggleEqPanel,
      showSmartMixes, setShowSmartMixes,
      spotifyClientId, spotifyClientSecret, setSpotifyCredentials,
      logs, clearLogs, addSongs, resetLibrary,
      isScanning, scanProgress, setScanning, setScanProgress
  } = useStore();

  const [tempClientId, setTempClientId] = useState(spotifyClientId);
  const [tempClientSecret, setTempClientSecret] = useState(spotifyClientSecret);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  const handleSaveCredentials = () => {
      setSpotifyCredentials(tempClientId, tempClientSecret);
  };

  const confirmResetLibrary = async () => {
      setIsResetting(true);
      try {
          await resetLibrary();
      } catch (e) {
          console.error("Reset failed", e);
          setIsResetting(false);
          setShowResetConfirm(false);
          alert("An error occurred while resetting the library.");
      }
  };

  const getLogColor = (level: string) => {
      switch(level) {
          case 'error': return 'text-red-500';
          case 'warn': return 'text-yellow-500';
          case 'success': return 'text-green-500';
          default: return 'text-text-secondary';
      }
  };

  // Helper to collect all files recursively from a directory handle
  async function* getFilesRecursively(entry: FileSystemDirectoryHandle): AsyncGenerator<{ handle: FileSystemFileHandle, path: string }> {
    const queue: { handle: FileSystemDirectoryHandle, path: string }[] = [{ handle: entry, path: entry.name }];
    
    while (queue.length > 0) {
        const { handle, path } = queue.shift()!;
        
        // Iterate over the directory handle
        // @ts-ignore - The File System Access API types might not be fully available in all environments
        for await (const entry of handle.values()) {
            const newPath = `${path}/${entry.name}`;
            if (entry.kind === 'file') {
                yield { handle: entry as FileSystemFileHandle, path: newPath };
            } else if (entry.kind === 'directory') {
                queue.push({ handle: entry as FileSystemDirectoryHandle, path: newPath });
            }
        }
    }
  }

  const processBatch = async (items: any[], processFn: (item: any) => Promise<Song | null>) => {
        const CONCURRENCY = 3; 
        const BATCH_SIZE = 20;
        let pendingBatch: Song[] = [];
        let totalParsed = 0;

        for (let i = 0; i < items.length; i += CONCURRENCY) {
            setScanProgress(`Scanning ${Math.min(i + 1, items.length)} - ${Math.min(i + CONCURRENCY, items.length)} of ${items.length}...`);
            
            const batchPromises = items.slice(i, i + CONCURRENCY).map(processFn);
            
            const results = await Promise.all(batchPromises);
            results.forEach(song => {
                if (song) {
                    pendingBatch.push(song);
                    totalParsed++;
                }
            });

            if (pendingBatch.length >= BATCH_SIZE) {
                addSongs(pendingBatch);
                pendingBatch = [];
            }
            
            await new Promise(resolve => setTimeout(resolve, 10)); // Yield
        }

        if (pendingBatch.length > 0) {
            addSongs(pendingBatch);
        }
        
        return totalParsed;
  };

  const handleScanClick = async () => {
    // Check for API support and if blocked by iframe policy
    const supportsFSA = 'showDirectoryPicker' in window;
    const isIframe = window.self !== window.top;

    if (!supportsFSA || isIframe) {
        // Fallback to legacy input
        if (fileInputRef.current) {
            fileInputRef.current.click();
        } else {
            alert("File import not supported in this browser.");
        }
        return;
    }

    try {
        // Cast window to any to access the experimental API
        const dirHandle = await (window as any).showDirectoryPicker({ id: 'music-lib', mode: 'read' });
        
        setScanning(true);
        setScanProgress('Reading directory structure...');

        const audioHandles: { handle: FileSystemFileHandle, path: string }[] = [];
        const imageFiles = new Map<string, File[]>();

        // 1. First Pass: Collect Handles
        for await (const { handle, path } of getFilesRecursively(dirHandle)) {
            const lowerName = handle.name.toLowerCase();
            if (lowerName.endsWith('.mp3') || lowerName.endsWith('.ogg')) {
                audioHandles.push({ handle, path });
            } else if (lowerName.endsWith('.jpg') || lowerName.endsWith('.jpeg') || lowerName.endsWith('.png')) {
                const file = await handle.getFile();
                const pathParts = path.split('/');
                pathParts.pop(); // remove filename
                const folderPath = pathParts.join('/');
                
                if (!imageFiles.has(folderPath)) {
                    imageFiles.set(folderPath, []);
                }
                imageFiles.get(folderPath)!.push(file);
            }
        }

        if (audioHandles.length === 0) {
            setScanning(false);
            alert("No supported audio files found.");
            return;
        }

        // 2. Process Files
        const total = await processBatch(audioHandles, async ({ handle, path }) => {
            try {
                const file = await handle.getFile();
                const song = await parseSong(file, imageFiles);
                song.fileHandle = handle;
                song.path = path;
                return song;
            } catch (e) {
                console.error(`Failed to parse ${handle.name}`, e);
                return null;
            }
        });

        setScanning(false);
        setScanProgress('');
        alert(`Import complete! Added ${total} songs to library.`);

    } catch (err: any) {
        setScanning(false);
        if (err.name !== 'AbortError') {
            console.error(err);
            // Fallback for security errors
            if (err.name === 'SecurityError' || err.message?.includes('frame')) {
                alert("Browser restricted file access. Using standard upload instead.");
                fileInputRef.current?.click();
            } else {
                alert("Failed to scan directory. See console.");
            }
        }
    }
  };

  const handleLegacyImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0) return;

      setScanning(true);
      setScanProgress('Analyzing files...');

      const audioFiles: File[] = [];
      const imageFiles = new Map<string, File[]>();

      // Pre-process to sort images and audio
      for (let i = 0; i < files.length; i++) {
          const file = files[i];
          const lowerName = file.name.toLowerCase();
          
          if (lowerName.endsWith('.mp3') || lowerName.endsWith('.ogg')) {
              audioFiles.push(file);
          } else if (lowerName.endsWith('.jpg') || lowerName.endsWith('.jpeg') || lowerName.endsWith('.png')) {
              const path = file.webkitRelativePath || '';
              const parts = path.split('/');
              parts.pop();
              const folderPath = parts.join('/');
              if (!imageFiles.has(folderPath)) imageFiles.set(folderPath, []);
              imageFiles.get(folderPath)!.push(file);
          }
      }

      if (audioFiles.length === 0) {
          setScanning(false);
          alert("No audio files found in selection.");
          return;
      }

      const total = await processBatch(audioFiles, async (file) => {
           try {
               // Legacy import cannot persist fileHandle
               const song = await parseSong(file, imageFiles);
               return song;
           } catch (e) {
               return null;
           }
      });

      setScanning(false);
      setScanProgress('');
      alert(`Import complete! Added ${total} songs.`);
      
      // Reset input
      if (fileInputRef.current) fileInputRef.current.value = '';
  };

  return (
    <div className="p-8 pb-32 max-w-4xl mx-auto">
      <h1 className="text-3xl font-bold mb-10">Settings</h1>

      {/* Hidden Fallback Input */}
      <input 
          type="file" 
          ref={fileInputRef} 
          onChange={handleLegacyImport} 
          className="hidden" 
          // @ts-ignore
          webkitdirectory="" 
          directory="" 
          multiple 
      />

      {/* Library Management */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-6 text-brand">
            <FolderOpen size={20} />
            <h2 className="text-lg font-bold text-text-main">Library Management</h2>
        </div>
        
        <div className="flex flex-col gap-4">
            <p className="text-sm text-text-secondary">
                Import music from your local folders. Supported formats: MP3, OGG.
            </p>
            
            <div className="flex items-center gap-4">
                <button 
                    onClick={handleScanClick}
                    disabled={isScanning}
                    className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border disabled:opacity-50 disabled:cursor-not-allowed text-text-main font-bold py-3 px-6 rounded-full transition-all border border-surface-border hover:border-surface-slider"
                >
                    {isScanning ? <Loader2 size={20} className="animate-spin" /> : <FolderOpen size={20} />}
                    {isScanning ? 'Scanning...' : 'Scan Local Directory'}
                </button>
                {isScanning && <span className="text-sm text-brand font-mono">{scanProgress}</span>}
            </div>
            
            <div className="text-xs text-text-subtle mt-2">
                Uses File System Access API where available. Fallback to standard upload in restricted environments.
            </div>
        </div>
      </section>

      {/* Personalization */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-6 text-brand">
            <Sparkles size={20} />
            <h2 className="text-lg font-bold text-text-main">Personalization</h2>
        </div>
        
        <div className="flex items-center justify-between">
            <div>
                <h3 className="font-medium text-text-main">Show Smart Mixes</h3>
                <p className="text-sm text-text-subtle">Display auto-generated mixes on the Home screen</p>
            </div>
            <div 
                className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${showSmartMixes ? 'bg-brand' : 'bg-surface-border'}`}
                onClick={() => setShowSmartMixes(!showSmartMixes)}
            >
                <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${showSmartMixes ? 'right-1' : 'left-1'}`}></div>
            </div>
        </div>
      </section>

      {/* Audio & Playback */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-6 text-brand">
            <Volume2 size={20} />
            <h2 className="text-lg font-bold text-text-main">Audio & Playback</h2>
        </div>

        <div className="space-y-6">
            {/* Equalizer */}
            <div className="flex items-center justify-between pb-4 border-b border-surface-hover">
                <div className="flex items-center gap-4">
                    <div className="p-2 bg-surface-hover rounded-lg text-text-main">
                        <SlidersHorizontal size={20} />
                    </div>
                    <div>
                        <h3 className="font-medium text-text-main">Equalizer</h3>
                        <p className="text-sm text-text-subtle">Adjust frequency levels or choose a preset</p>
                    </div>
                </div>
                <div className="flex items-center gap-4">
                    <button 
                        onClick={toggleEqPanel}
                        className="text-sm font-bold text-brand hover:underline"
                    >
                        Open Panel
                    </button>
                    <div 
                        className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${audioSettings.eqEnabled ? 'bg-brand' : 'bg-surface-border'}`}
                        onClick={() => setEqEnabled(!audioSettings.eqEnabled)}
                    >
                        <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${audioSettings.eqEnabled ? 'right-1' : 'left-1'}`}></div>
                    </div>
                </div>
            </div>

            {/* Visualizer Mode */}
            <div className="flex items-center justify-between pb-4 border-b border-surface-hover">
                 <div className="flex items-center gap-4">
                    <div className="p-2 bg-surface-hover rounded-lg text-text-main">
                        <Activity size={20} />
                    </div>
                    <div>
                        <h3 className="font-medium text-text-main">Visualizer Style</h3>
                        <p className="text-sm text-text-subtle">Choose appearance for Now Playing screen</p>
                    </div>
                </div>
                <select 
                    value={audioSettings.visualizerMode}
                    onChange={(e) => setVisualizerMode(e.target.value as VisualizerMode)}
                    className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
                >
                    <option value="OFF">Disabled</option>
                    <option value="WAVE">Waveform</option>
                    <option value="SPECTRUM">Spectrum Bar</option>
                    <option value="AURORA">Ambient Aurora</option>
                </select>
            </div>

            {/* Crossfade */}
            <div>
                <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-4">
                        <div className="p-2 bg-surface-hover rounded-lg text-text-main">
                            <Layers size={20} />
                        </div>
                        <div>
                            <h3 className="font-medium text-text-main">Crossfade</h3>
                            <p className="text-sm text-text-subtle">Overlap songs for smooth transitions</p>
                        </div>
                    </div>
                    <span className="font-mono text-sm text-brand">{audioSettings.crossfadeDuration}s</span>
                </div>
                <input 
                    type="range" 
                    min="0" 
                    max="12" 
                    step="1"
                    value={audioSettings.crossfadeDuration}
                    onChange={(e) => setCrossfade(parseInt(e.target.value))}
                    className="w-full h-1 bg-surface-border rounded-lg appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white hover:[&::-webkit-slider-thumb]:bg-brand"
                />
            </div>

            {/* Toggles */}
            <div className="flex items-center justify-between">
                <div>
                    <h3 className="font-medium">Gapless Playback</h3>
                    <p className="text-sm text-text-subtle">Eliminate silence between tracks</p>
                </div>
                <div 
                    className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${audioSettings.gapless ? 'bg-brand' : 'bg-surface-border'}`}
                    onClick={() => setGapless(!audioSettings.gapless)}
                >
                    <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${audioSettings.gapless ? 'right-1' : 'left-1'}`}></div>
                </div>
            </div>

             <div className="flex items-center justify-between">
                <div>
                    <h3 className="font-medium">Loudness Normalization</h3>
                    <p className="text-sm text-text-subtle">Adjust volume to the same level for all tracks</p>
                </div>
                <div 
                    className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${audioSettings.normalization ? 'bg-brand' : 'bg-surface-border'}`}
                    onClick={() => setNormalization(!audioSettings.normalization)}
                >
                    <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${audioSettings.normalization ? 'right-1' : 'left-1'}`}></div>
                </div>
            </div>
        </div>
      </section>

      {/* Spotify Integration */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-4 text-brand">
            <Wifi size={20} />
            <h2 className="text-lg font-bold text-text-main">Spotify Metadata Integration</h2>
        </div>
        
        <div className="space-y-4">
            <p className="text-sm text-text-secondary">
                To fetch high-quality album covers and artist images, MediaHub requires Spotify Developer credentials.
            </p>
            
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                    <label className="block text-xs font-bold text-text-subtle uppercase mb-2">Client ID</label>
                    <input 
                        type="text" 
                        value={tempClientId}
                        onChange={(e) => setTempClientId(e.target.value)}
                        placeholder="Enter Client ID"
                        className="w-full bg-surface-1 border border-surface-border rounded px-4 py-3 text-text-main focus:border-brand outline-none transition-colors"
                    />
                </div>
                <div>
                    <label className="block text-xs font-bold text-text-subtle uppercase mb-2">Client Secret</label>
                    <input 
                        type="password" 
                        value={tempClientSecret}
                        onChange={(e) => setTempClientSecret(e.target.value)}
                        placeholder="Enter Client Secret"
                        className="w-full bg-surface-1 border border-surface-border rounded px-4 py-3 text-text-main focus:border-brand outline-none transition-colors"
                    />
                </div>
            </div>

            <div className="flex items-center justify-between mt-2">
                <p className="text-xs text-text-subtle">
                    Create an app at <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-brand hover:underline">developer.spotify.com</a> to get these keys.
                </p>
                <button 
                    onClick={handleSaveCredentials}
                    className="bg-brand hover:bg-brand-hover text-black font-bold py-2 px-6 rounded-full transition-colors text-sm"
                >
                    Save Credentials
                </button>
            </div>
        </div>
      </section>

      {/* Storage & Downloads */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-6 text-brand">
            <HardDrive size={20} />
            <h2 className="text-lg font-bold text-text-main">Storage & Downloads</h2>
        </div>

        <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg mb-4">
            <div>
                <h3 className="text-sm text-text-secondary mb-1">Database & Cache</h3>
                <p className="text-xs text-text-subtle">Clear local database to resolve issues.</p>
            </div>
            <button 
                onClick={() => setShowResetConfirm(true)}
                className="flex items-center gap-2 bg-surface-2 border border-surface-border text-text-main px-4 py-2 rounded font-bold text-sm hover:bg-red-500/20 hover:text-red-500 hover:border-red-500/50 transition-all"
            >
                <Trash2 size={16} /> Reset Library
            </button>
        </div>

        <div>
            <label className="block text-xs font-bold text-text-secondary uppercase mb-2">Download Quality</label>
            <select className="w-full bg-surface-1 border border-surface-border rounded px-4 py-3 text-text-main focus:border-brand outline-none">
                <option>High Quality (320kbps)</option>
                <option>Normal (128kbps)</option>
            </select>
        </div>
      </section>

      {/* Debug Console */}
      <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
          <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-3 text-brand">
                  <Terminal size={20} />
                  <h2 className="text-lg font-bold text-text-main">Debug Console</h2>
              </div>
              <button 
                onClick={clearLogs}
                className="flex items-center gap-1 text-xs text-text-secondary hover:text-text-main"
              >
                  <XCircle size={14} /> Clear
              </button>
          </div>
          
          <div className="bg-surface-1 border border-surface-border rounded-lg p-4 h-64 overflow-y-auto font-mono text-xs">
              {logs.length === 0 ? (
                  <div className="text-surface-slider text-center italic mt-10">No logs generated yet.</div>
              ) : (
                  logs.map((log) => (
                      <div key={log.id} className="mb-2 last:mb-0 border-b border-surface-3 pb-2 last:border-0 last:pb-0">
                          <div className="flex items-start gap-2">
                              <span className="text-surface-slider whitespace-nowrap">{new Date(log.timestamp).toLocaleTimeString()}</span>
                              <span className={`font-bold uppercase w-16 ${getLogColor(log.level)}`}>[{log.level}]</span>
                              <span className="text-gray-300 break-all">{log.message}</span>
                          </div>
                          {log.details && (
                              <div className="ml-24 mt-1 text-gray-500 break-all">
                                  {JSON.stringify(log.details)}
                              </div>
                          )}
                      </div>
                  ))
              )}
          </div>
          <p className="text-[10px] text-surface-slider mt-2">Logs capture API requests to Spotify and internal errors.</p>
      </section>

      {/* Custom Confirmation Modal */}
      {showResetConfirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
              <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-md w-full shadow-2xl scale-100">
                  <div className="flex items-center gap-4 text-red-500 mb-4">
                      <AlertTriangle size={32} />
                      <h2 className="text-xl font-bold text-white">Reset Library?</h2>
                  </div>
                  <p className="text-text-secondary mb-6 leading-relaxed">
                      Are you sure you want to delete your entire music library? This will remove all songs, playlists, and cached metadata from the database. <br/><br/>
                      <span className="text-red-400 font-bold">This action cannot be undone.</span>
                  </p>
                  
                  <div className="flex items-center justify-end gap-3">
                      <button 
                          onClick={() => setShowResetConfirm(false)}
                          disabled={isResetting}
                          className="px-4 py-2 rounded-lg font-medium text-text-main hover:bg-surface-3 transition-colors"
                      >
                          Cancel
                      </button>
                      <button 
                          onClick={confirmResetLibrary}
                          disabled={isResetting}
                          className="px-6 py-2 rounded-lg font-bold bg-red-600 hover:bg-red-700 text-white transition-colors flex items-center gap-2"
                      >
                          {isResetting && <Loader2 size={16} className="animate-spin" />}
                          {isResetting ? 'Resetting...' : 'Yes, Delete Everything'}
                      </button>
                  </div>
              </div>
          </div>
      )}

    </div>
  );
};
