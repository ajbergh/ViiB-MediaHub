/**
 * ViiB MediaHub - Settings Page
 * 
 * Comprehensive settings interface for application configuration.
 * 
 * Sections:
 * - Backend Status: Connection status indicator
 * - Library: Scan folders management, library reset
 * - Audio: Crossfade, gapless, normalization, visualizer, EQ
 * - Spotify: OAuth credentials, download location, concurrent downloads
 * - Activity Log: Debug log viewer
 * 
 * Folder browser dialogs allow navigation and selection of:
 * - Music scan directories
 * - Spotify download destination
 * 
 * @module Settings
 */

import React, { useState, useRef, useEffect } from 'react';
import { Wifi, Volume2, HardDrive, Trash2, Terminal, XCircle, SlidersHorizontal, Activity, Layers, Sparkles, FolderOpen, Loader2, AlertTriangle, Plus, X, RefreshCw, Server, MonitorOff, BarChart3 } from 'lucide-react';
import { useStore } from '../store';
import { VisualizerMode, Song } from '../types';
import { parseSong } from '../metadata';
import { api } from '../services/api';

export const Settings: React.FC = () => {
  const { 
      audioSettings, setCrossfade, setGapless, setNormalization, 
      setVisualizerMode, setEqEnabled, toggleEqPanel,
      showSmartMixes, setShowSmartMixes,
      spotifyClientId, spotifyClientSecret, setSpotifyCredentials,
      streamingEnabled, streamingQuality, setStreamingEnabled, setStreamingQuality,
      preferLocalPlayback, setPreferLocalPlayback,
      streamingStats, resetStreamingStats,
      logs, clearLogs, addLog, addSongs, resetLibrary,
      isScanning, scanProgress, setScanning, setScanProgress,
      backendAvailable, scanFolders, loadScanFolders, addScanFolder, removeScanFolder, startBackendScan
  } = useStore();

  const [tempClientId, setTempClientId] = useState(spotifyClientId);
  const [tempClientSecret, setTempClientSecret] = useState(spotifyClientSecret);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  // Folder browser state
  const [showFolderBrowser, setShowFolderBrowser] = useState(false);
  const [browserPath, setBrowserPath] = useState('');
  
  // Gemini Enrichment State
  const [isEnriching, setIsEnriching] = useState(false);
  const [enrichStatus, setEnrichStatus] = useState('');
  const [forceEnrichment, setForceEnrichment] = useState(false);
  const [browserEntries, setBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingBrowser, setLoadingBrowser] = useState(false);

  // Download folder browser state
  const [showDownloadFolderBrowser, setShowDownloadFolderBrowser] = useState(false);
  const [downloadBrowserPath, setDownloadBrowserPath] = useState('');
  const [downloadBrowserEntries, setDownloadBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingDownloadBrowser, setLoadingDownloadBrowser] = useState(false);

  // Spotify download location
  const [spotifyDownloadPath, setSpotifyDownloadPath] = useState('');
  const [downloadPathSaved, setDownloadPathSaved] = useState(false);

  // Concurrent downloads setting
  const [concurrentDownloads, setConcurrentDownloads] = useState(3);
  const [concurrentSaved, setConcurrentSaved] = useState(false);

  // Load scan folders on mount if backend available
  useEffect(() => {
      if (backendAvailable) {
          loadScanFolders();
      }
  }, [backendAvailable]);

  // Load Spotify download path and concurrent downloads
  useEffect(() => {
      const loadDownloadSettings = async () => {
          try {
              const path = await api.getSetting('spotify_download_path');
              if (path) {
                  setSpotifyDownloadPath(path);
              }
          } catch (e) {
              console.error('Failed to load download path:', e);
          }
          try {
              const concurrent = await api.getSetting('concurrent_downloads');
              if (concurrent) {
                  const n = parseInt(concurrent, 10);
                  if (n >= 1 && n <= 10) {
                      setConcurrentDownloads(n);
                  }
              }
          } catch (e) {
              console.error('Failed to load concurrent downloads:', e);
          }
      };
      if (backendAvailable) {
          loadDownloadSettings();
      }
  }, [backendAvailable]);

  const handleSaveConcurrentDownloads = async () => {
      try {
          await api.setSetting('concurrent_downloads', concurrentDownloads.toString());
          addLog('success', `Concurrent downloads set to ${concurrentDownloads}`);
          setConcurrentSaved(true);
          setTimeout(() => setConcurrentSaved(false), 3000);
      } catch (e) {
          addLog('error', 'Failed to save concurrent downloads setting', e);
      }
  };

  const handleSaveCredentials = async () => {
      setSpotifyCredentials(tempClientId, tempClientSecret);
      
      // Sync to backend immediately
      try {
          await api.saveSpotifyCredentials({
              clientId: tempClientId,
              clientSecret: tempClientSecret,
              accessToken: useStore.getState().spotifyAccessToken || '',
              refreshToken: useStore.getState().spotifyRefreshToken || '',
              expiry: useStore.getState().spotifyTokenExpiry || 0
          });
          addLog('success', 'Spotify credentials saved and synced to backend');
      } catch (e) {
          addLog('warn', 'Saved locally but failed to sync to backend', e);
      }

      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
  };

  const handleSaveDownloadPath = async () => {
      try {
          await api.setSetting('spotify_download_path', spotifyDownloadPath);
          addLog('success', 'Download location saved');
          setDownloadPathSaved(true);
          setTimeout(() => setDownloadPathSaved(false), 3000);
      } catch (e) {
          addLog('error', 'Failed to save download location', e);
      }
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

  // Folder browser functions
  const openFolderBrowser = async () => {
      setShowFolderBrowser(true);
      setLoadingBrowser(true);
      try {
          const result = await api.browseFolder();
          setBrowserPath(result.currentPath);
          setBrowserEntries(result.entries);
      } catch (e) {
          console.error("Failed to browse folder", e);
      }
      setLoadingBrowser(false);
  };

  const navigateFolder = async (path: string) => {
      setLoadingBrowser(true);
      try {
          const result = await api.browseFolder(path);
          setBrowserPath(result.currentPath);
          setBrowserEntries(result.entries);
      } catch (e) {
          console.error("Failed to navigate to folder", e);
      }
      setLoadingBrowser(false);
  };

  const selectCurrentFolder = async () => {
      if (browserPath) {
          await addScanFolder(browserPath);
          setShowFolderBrowser(false);
      }
  };

  // Download folder browser functions
  const openDownloadFolderBrowser = async () => {
      setShowDownloadFolderBrowser(true);
      setLoadingDownloadBrowser(true);
      try {
          // Start from current download path if set, otherwise home
          const startPath = spotifyDownloadPath || undefined;
          const result = await api.browseFolder(startPath);
          setDownloadBrowserPath(result.currentPath);
          setDownloadBrowserEntries(result.entries);
      } catch (e) {
          console.error("Failed to browse folder", e);
          // Try without a path on error
          try {
              const result = await api.browseFolder();
              setDownloadBrowserPath(result.currentPath);
              setDownloadBrowserEntries(result.entries);
          } catch (e2) {
              console.error("Failed to browse folder", e2);
          }
      }
      setLoadingDownloadBrowser(false);
  };

  const navigateDownloadFolder = async (path: string) => {
      setLoadingDownloadBrowser(true);
      try {
          const result = await api.browseFolder(path);
          setDownloadBrowserPath(result.currentPath);
          setDownloadBrowserEntries(result.entries);
      } catch (e) {
          console.error("Failed to navigate to folder", e);
      }
      setLoadingDownloadBrowser(false);
  };

  const selectDownloadFolder = async () => {
      if (downloadBrowserPath) {
          setSpotifyDownloadPath(downloadBrowserPath);
          setShowDownloadFolderBrowser(false);
          // Auto-save when selected via browser
          try {
              await api.setSetting('spotify_download_path', downloadBrowserPath);
              addLog('success', `Download location set to: ${downloadBrowserPath}`);
              setDownloadPathSaved(true);
              setTimeout(() => setDownloadPathSaved(false), 3000);
          } catch (e) {
              addLog('error', 'Failed to save download location', e);
          }
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
    <div className="p-8 pb-32 max-w-4xl mx-auto animate-fade-in">
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
            {backendAvailable ? (
                <span className="flex items-center gap-1 text-xs bg-green-500/20 text-green-400 px-2 py-1 rounded-full">
                    <Server size={12} /> Backend Connected
                </span>
            ) : (
                <span className="flex items-center gap-1 text-xs bg-yellow-500/20 text-yellow-400 px-2 py-1 rounded-full">
                    <MonitorOff size={12} /> Browser Mode
                </span>
            )}
        </div>
        
        {/* Backend Mode - Folder Management */}
        {backendAvailable && (
            <div className="mb-6">
                <h3 className="text-sm font-bold text-text-main mb-3">Music Folders</h3>
                <p className="text-sm text-text-secondary mb-4">
                    Add folders containing your music files. The backend will scan these folders for audio files.
                </p>
                
                {/* List of scan folders */}
                <div className="space-y-2 mb-4">
                    {scanFolders.length === 0 ? (
                        <div className="text-text-subtle text-sm italic p-4 bg-surface-1 rounded-lg text-center">
                            No folders added yet. Add a folder to start scanning.
                        </div>
                    ) : (
                        scanFolders.map(folder => (
                            <div key={folder.id} className="flex items-center justify-between bg-surface-1 p-3 rounded-lg border border-surface-border">
                                <div className="flex-1 min-w-0">
                                    <div className="font-mono text-sm text-text-main truncate">{folder.path}</div>
                                    <div className="text-xs text-text-subtle">
                                        {folder.songCount} songs • Last scan: {folder.lastScan ? new Date(folder.lastScan).toLocaleDateString() : 'Never'}
                                    </div>
                                </div>
                                <button 
                                    onClick={() => removeScanFolder(folder.id)}
                                    className="p-2 text-text-subtle hover:text-red-500 transition-colors"
                                    title="Remove folder"
                                >
                                    <X size={18} />
                                </button>
                            </div>
                        ))
                    )}
                </div>
                
                {/* Add folder & Scan buttons */}
                <div className="flex items-center gap-3">
                    <button 
                        onClick={openFolderBrowser}
                        className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main font-bold py-2 px-4 rounded-lg transition-all border border-surface-border"
                    >
                        <Plus size={18} /> Add Folder
                    </button>
                    <button 
                        onClick={startBackendScan}
                        disabled={isScanning || scanFolders.length === 0}
                        className="flex items-center gap-2 bg-brand hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-2 px-4 rounded-lg transition-all"
                    >
                        {isScanning ? <Loader2 size={18} className="animate-spin" /> : <RefreshCw size={18} />}
                        {isScanning ? 'Scanning...' : 'Scan All Folders'}
                    </button>
                </div>
                {isScanning && <div className="text-sm text-brand font-mono mt-2">{scanProgress}</div>}
            </div>
        )}

        {/* Browser Mode - File Picker */}
        {!backendAvailable && (
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
        )}
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
                <div className="flex items-center gap-3">
                    {saveSuccess && (
                        <span className="text-green-500 text-sm font-bold animate-in fade-in slide-in-from-right-4">
                            Saved!
                        </span>
                    )}
                    <button 
                        onClick={handleSaveCredentials}
                        className="bg-brand hover:bg-brand-hover text-black font-bold py-2 px-6 rounded-full transition-colors text-sm"
                    >
                        Save Credentials
                    </button>
                </div>
            </div>
        </div>
      </section>

      {/* Storage & Downloads */}
      <section className="bg-surface-2 rounded-xl p-6 mb-6 border border-surface-3">
        <div className="flex items-center gap-3 mb-6 text-brand">
            <HardDrive size={20} />
            <h2 className="text-lg font-bold text-text-main">Storage & Downloads</h2>
        </div>

        {/* Spotify Download Location */}
        {backendAvailable && (
            <div className="mb-6">
                <label className="block text-xs font-bold text-text-secondary uppercase mb-2">Spotify Download Location</label>
                <p className="text-xs text-text-subtle mb-3">
                    Specify where Spotify downloads should be saved. Leave empty to use default location.
                </p>
                <div className="flex items-center gap-3">
                    <input 
                        type="text" 
                        value={spotifyDownloadPath}
                        onChange={(e) => setSpotifyDownloadPath(e.target.value)}
                        placeholder="Default: AppData/ViiB-MediaHub/spotify_downloads"
                        className="flex-1 bg-surface-1 border border-surface-border rounded px-4 py-3 text-text-main focus:border-brand outline-none font-mono text-sm"
                    />
                    <button 
                        onClick={openDownloadFolderBrowser}
                        className="bg-surface-1 border border-surface-border hover:bg-surface-hover text-text-main font-bold py-3 px-4 rounded-lg transition-colors flex items-center gap-2"
                        title="Browse folders"
                    >
                        <FolderOpen size={18} />
                    </button>
                    {downloadPathSaved && (
                        <span className="text-green-500 text-sm font-bold">
                            Saved!
                        </span>
                    )}
                    <button 
                        onClick={handleSaveDownloadPath}
                        className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-6 rounded-lg transition-colors text-sm"
                    >
                        Save
                    </button>
                </div>
            </div>
        )}

        {/* Concurrent Downloads */}
        {backendAvailable && (
            <div className="mb-6">
                <label className="block text-xs font-bold text-text-secondary uppercase mb-2">Concurrent Downloads</label>
                <p className="text-xs text-text-subtle mb-3">
                    Number of tracks to download simultaneously. Higher values may improve speed on fast connections.
                </p>
                <div className="flex items-center gap-4">
                    <div className="flex-1 flex items-center gap-4">
                        <input 
                            type="range"
                            min={1}
                            max={10}
                            value={concurrentDownloads}
                            onChange={(e) => setConcurrentDownloads(parseInt(e.target.value, 10))}
                            className="flex-1 h-2 bg-surface-1 rounded-lg appearance-none cursor-pointer accent-brand"
                        />
                        <span className="text-text-main font-bold w-8 text-center">{concurrentDownloads}</span>
                    </div>
                    {concurrentSaved && (
                        <span className="text-green-500 text-sm font-bold">
                            Saved!
                        </span>
                    )}
                    <button 
                        onClick={handleSaveConcurrentDownloads}
                        className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-6 rounded-lg transition-colors text-sm"
                    >
                        Save
                    </button>
                </div>
                <p className="text-xs text-text-subtle mt-2">
                    Recommended: 3 for most connections, up to 6-10 for high-speed connections (100+ Mbps).
                </p>
            </div>
        )}

        {/* Spotify Streaming Settings */}
        <div className="mb-6">
            <label className="block text-xs font-bold text-text-secondary uppercase mb-2">Spotify Streaming</label>
            <p className="text-xs text-text-subtle mb-3">
                Stream Spotify tracks directly without downloading first. Requires Spotify Premium.
            </p>
            
            {/* Enable/Disable Streaming Toggle */}
            <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg mb-3">
                <div>
                    <h3 className="text-sm text-text-main font-medium">Enable Streaming</h3>
                    <p className="text-xs text-text-subtle">Play Spotify tracks instantly without downloading</p>
                </div>
                <button
                    onClick={() => setStreamingEnabled(!streamingEnabled)}
                    className={`relative w-12 h-6 rounded-full transition-colors ${streamingEnabled ? 'bg-brand' : 'bg-surface-3'}`}
                >
                    <div className={`absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform ${streamingEnabled ? 'translate-x-7' : 'translate-x-1'}`} />
                </button>
            </div>
            
            {/* Streaming Quality Selector */}
            <div className="bg-surface-1 p-4 rounded-lg">
                <div className="flex items-center justify-between mb-2">
                    <div>
                        <h3 className="text-sm text-text-main font-medium">Streaming Quality</h3>
                        <p className="text-xs text-text-subtle">Higher quality uses more bandwidth</p>
                    </div>
                </div>
                <div className="flex gap-2 mt-3">
                    <button
                        onClick={() => setStreamingQuality('high')}
                        className={`flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all ${
                            streamingQuality === 'high' 
                                ? 'bg-brand text-black' 
                                : 'bg-surface-2 text-text-secondary hover:bg-surface-3'
                        }`}
                    >
                        High (320kbps)
                    </button>
                    <button
                        onClick={() => setStreamingQuality('medium')}
                        className={`flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all ${
                            streamingQuality === 'medium' 
                                ? 'bg-brand text-black' 
                                : 'bg-surface-2 text-text-secondary hover:bg-surface-3'
                        }`}
                    >
                        Medium (160kbps)
                    </button>
                    <button
                        onClick={() => setStreamingQuality('low')}
                        className={`flex-1 py-2 px-4 rounded-lg text-sm font-bold transition-all ${
                            streamingQuality === 'low' 
                                ? 'bg-brand text-black' 
                                : 'bg-surface-2 text-text-secondary hover:bg-surface-3'
                        }`}
                    >
                        Low (96kbps)
                    </button>
                </div>
            </div>
            
            {/* Prefer Local Playback Toggle */}
            <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg mt-3">
                <div>
                    <h3 className="text-sm text-text-main font-medium">Prefer Local Files</h3>
                    <p className="text-xs text-text-subtle">When enabled, downloaded tracks play locally instead of streaming</p>
                </div>
                <button
                    onClick={() => setPreferLocalPlayback(!preferLocalPlayback)}
                    className={`relative w-12 h-6 rounded-full transition-colors ${preferLocalPlayback ? 'bg-brand' : 'bg-surface-3'}`}
                >
                    <div className={`absolute top-1 w-4 h-4 bg-white rounded-full shadow transition-transform ${preferLocalPlayback ? 'translate-x-7' : 'translate-x-1'}`} />
                </button>
            </div>
            
            {/* Streaming Statistics */}
            <div className="bg-surface-1 p-4 rounded-lg mt-3">
                <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                        <BarChart3 size={16} className="text-brand" />
                        <h3 className="text-sm text-text-main font-medium">Streaming Statistics</h3>
                    </div>
                    <button
                        onClick={resetStreamingStats}
                        className="text-xs text-text-subtle hover:text-text-secondary transition-colors"
                    >
                        Reset Stats
                    </button>
                </div>
                
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                    {/* Total Streams */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-brand">{streamingStats.totalStreams}</div>
                        <div className="text-xs text-text-subtle">Total Streams</div>
                    </div>
                    
                    {/* Success Rate */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-green-500">
                            {streamingStats.totalStreams > 0 
                                ? Math.round((streamingStats.successfulStreams / streamingStats.totalStreams) * 100)
                                : 100}%
                        </div>
                        <div className="text-xs text-text-subtle">Success Rate</div>
                    </div>
                    
                    {/* Buffering Events */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-yellow-500">{streamingStats.bufferingEvents}</div>
                        <div className="text-xs text-text-subtle">Buffer Events</div>
                    </div>
                    
                    {/* Average Buffer Time */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-text-main">
                            {streamingStats.averageBufferingDuration.toFixed(1)}s
                        </div>
                        <div className="text-xs text-text-subtle">Avg Buffer Time</div>
                    </div>
                </div>
                
                {/* Error Breakdown (if any errors) */}
                {streamingStats.failedStreams > 0 && (
                    <div className="mt-3 pt-3 border-t border-surface-3">
                        <div className="text-xs text-text-subtle mb-2">Errors by Type</div>
                        <div className="flex gap-4 text-xs">
                            {streamingStats.errorsByType.network > 0 && (
                                <span className="text-red-400">Network: {streamingStats.errorsByType.network}</span>
                            )}
                            {streamingStats.errorsByType.auth > 0 && (
                                <span className="text-orange-400">Auth: {streamingStats.errorsByType.auth}</span>
                            )}
                            {streamingStats.errorsByType.unavailable > 0 && (
                                <span className="text-yellow-400">Unavailable: {streamingStats.errorsByType.unavailable}</span>
                            )}
                            {streamingStats.errorsByType.unknown > 0 && (
                                <span className="text-gray-400">Unknown: {streamingStats.errorsByType.unknown}</span>
                            )}
                        </div>
                    </div>
                )}
                
                {/* Last Streamed Track */}
                {streamingStats.lastStreamedTrack && (
                    <div className="mt-3 pt-3 border-t border-surface-3">
                        <div className="text-xs text-text-subtle">Last Streamed</div>
                        <div className="text-sm text-text-main truncate">{streamingStats.lastStreamedTrack}</div>
                    </div>
                )}
            </div>
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
            <select className="w-full bg-surface-1 border border-surface-border rounded px-4 py-3 text-text-main focus:border-brand outline-none" disabled>
                <option>High Quality (320kbps)</option>
                <option>Normal (160kbps)</option>
            </select>
            <p className="text-xs text-text-subtle mt-2">Downloads automatically use 320kbps when available.</p>
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

      {/* Smart Features Section */}
      <div className="bg-surface-2 rounded-xl p-6 border border-surface-border">
        <div className="flex items-center gap-3 mb-6">
          <Sparkles className="text-brand" size={24} />
          <h2 className="text-xl font-bold text-text-main">Library Intelligence</h2>
        </div>

        <div className="space-y-6">
          {/* Gemini Integration */}
          <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
            <h3 className="text-lg font-bold text-text-main mb-2">Generative Genre Enrichment</h3>
            <p className="text-text-subtle text-sm mb-4">
              Use Google's Gemini AI to automatically populate detailed genre information for your songs. 
              This enables smarter "Vibe" mixes and better organization.
            </p>
            
            <div className="flex flex-col gap-4">
              <div>
                <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                  Gemini API Key
                </label>
                <input
                  type="password"
                  placeholder="Enter your Gemini API Key (optional if already saved)"
                  className="w-full bg-surface-3 border border-surface-border rounded-lg px-3 py-2 text-text-main focus:outline-none focus:border-brand transition-colors"
                  id="gemini-api-key"
                  disabled={isEnriching}
                />
                <p className="text-xs text-text-subtle mt-1">
                  Get a key from <a href="https://aistudio.google.com/" target="_blank" rel="noreferrer" className="text-brand hover:underline">Google AI Studio</a>.
                </p>
              </div>

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="force-enrich"
                  checked={forceEnrichment}
                  onChange={(e) => setForceEnrichment(e.target.checked)}
                  disabled={isEnriching}
                  className="rounded border-surface-border bg-surface-3 text-brand focus:ring-brand"
                />
                <label htmlFor="force-enrich" className="text-sm text-text-main cursor-pointer select-none">
                  Force re-check all songs (slower, overwrites existing genres)
                </label>
              </div>

              <button
                onClick={async () => {
                  const input = document.getElementById('gemini-api-key') as HTMLInputElement;
                  const apiKey = input.value;
                  
                  try {
                    setIsEnriching(true);
                    setEnrichStatus('Starting enrichment process...');
                    
                    let offset = 0;
                    let totalEnriched = 0;
                    let keepGoing = true;

                    while (keepGoing) {
                        setEnrichStatus(`Processing batch starting at ${offset}... (Total enriched: ${totalEnriched})`);
                        
                        const res = await api.enrichGenres(apiKey, forceEnrichment, offset);
                        
                        if (res.status === 'error') {
                            throw new Error(res.message);
                        }

                        totalEnriched += res.count;
                        
                        // If we processed less than 50 songs, we're done
                        if (res.count < 50) {
                            keepGoing = false;
                        } else {
                            // Move to next batch
                            offset += 50;
                        }
                    }
                    
                    setEnrichStatus(`Success! Enriched ${totalEnriched} songs total.`);
                  } catch (e: any) {
                    setEnrichStatus(`Error: ${e.message}`);
                  } finally {
                    setIsEnriching(false);
                  }
                }}
                disabled={isEnriching}
                className={`self-start px-4 py-2 font-bold rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center ${
                    isEnriching 
                        ? 'bg-surface-2 text-text-subtle' 
                        : 'bg-brand text-black hover:bg-brand-hover'
                }`}
              >
                {isEnriching ? (
                    <>
                        <span className="animate-spin mr-2">⏳</span> Processing Library...
                    </>
                ) : (
                    <>✨ Enrich Library Genres</>
                )}
              </button>
              {enrichStatus && (
                  <div className={`text-sm mt-2 ${
                      enrichStatus.startsWith('Error') ? 'text-red-400' : 
                      enrichStatus.startsWith('Success') ? 'text-green-400' : 
                      'text-text-subtle'
                  }`}>
                      {enrichStatus}
                  </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Folder Browser Modal */}
      {showFolderBrowser && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
              <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-2xl w-full shadow-2xl max-h-[80vh] flex flex-col">
                  <div className="flex items-center justify-between mb-4">
                      <h2 className="text-xl font-bold text-white">Select Music Folder</h2>
                      <button 
                          onClick={() => setShowFolderBrowser(false)}
                          className="p-2 hover:bg-surface-3 rounded-lg transition-colors"
                      >
                          <X size={20} />
                      </button>
                  </div>
                  
                  {/* Current Path */}
                  <div className="bg-surface-1 border border-surface-border rounded-lg p-3 mb-4 font-mono text-sm text-text-main truncate">
                      {browserPath || 'Loading...'}
                  </div>
                  
                  {/* Folder List */}
                  <div className="flex-1 overflow-y-auto bg-surface-1 border border-surface-border rounded-lg mb-4 min-h-[300px]">
                      {loadingBrowser ? (
                          <div className="flex items-center justify-center h-full">
                              <Loader2 size={24} className="animate-spin text-brand" />
                          </div>
                      ) : (
                          <div className="divide-y divide-surface-border">
                              {browserEntries.map((entry, idx) => (
                                  <button
                                      key={idx}
                                      onClick={() => navigateFolder(entry.path)}
                                      className="w-full flex items-center gap-3 p-3 hover:bg-surface-hover transition-colors text-left"
                                  >
                                      <FolderOpen size={18} className="text-brand flex-shrink-0" />
                                      <span className="text-text-main truncate">{entry.name}</span>
                                  </button>
                              ))}
                              {browserEntries.length === 0 && (
                                  <div className="p-4 text-center text-text-subtle">No subfolders found</div>
                              )}
                          </div>
                      )}
                  </div>
                  
                  {/* Actions */}
                  <div className="flex items-center justify-end gap-3">
                      <button 
                          onClick={() => setShowFolderBrowser(false)}
                          className="px-4 py-2 rounded-lg font-medium text-text-main hover:bg-surface-3 transition-colors"
                      >
                          Cancel
                      </button>
                      <button 
                          onClick={selectCurrentFolder}
                          disabled={!browserPath}
                          className="px-6 py-2 rounded-lg font-bold bg-brand hover:bg-brand-hover disabled:opacity-50 text-black transition-colors flex items-center gap-2"
                      >
                          <Plus size={16} /> Add This Folder
                      </button>
                  </div>
              </div>
          </div>
      )}

      {/* Download Folder Browser Modal */}
      {showDownloadFolderBrowser && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
              <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-2xl w-full shadow-2xl max-h-[80vh] flex flex-col">
                  <div className="flex items-center justify-between mb-4">
                      <h2 className="text-xl font-bold text-white">Select Download Folder</h2>
                      <button 
                          onClick={() => setShowDownloadFolderBrowser(false)}
                          className="p-2 hover:bg-surface-3 rounded-lg transition-colors"
                      >
                          <X size={20} />
                      </button>
                  </div>
                  
                  {/* Current Path */}
                  <div className="bg-surface-1 border border-surface-border rounded-lg p-3 mb-4 font-mono text-sm text-text-main truncate">
                      {downloadBrowserPath || 'Loading...'}
                  </div>
                  
                  {/* Folder List */}
                  <div className="flex-1 overflow-y-auto bg-surface-1 border border-surface-border rounded-lg mb-4 min-h-[300px]">
                      {loadingDownloadBrowser ? (
                          <div className="flex items-center justify-center h-full">
                              <Loader2 size={24} className="animate-spin text-brand" />
                          </div>
                      ) : (
                          <div className="divide-y divide-surface-border">
                              {downloadBrowserEntries.map((entry, idx) => (
                                  <button
                                      key={idx}
                                      onClick={() => navigateDownloadFolder(entry.path)}
                                      className="w-full flex items-center gap-3 p-3 hover:bg-surface-hover transition-colors text-left"
                                  >
                                      <FolderOpen size={18} className="text-brand flex-shrink-0" />
                                      <span className="text-text-main truncate">{entry.name}</span>
                                  </button>
                              ))}
                              {downloadBrowserEntries.length === 0 && (
                                  <div className="p-4 text-center text-text-subtle">No subfolders found</div>
                              )}
                          </div>
                      )}
                  </div>
                  
                  {/* Actions */}
                  <div className="flex items-center justify-end gap-3">
                      <button 
                          onClick={() => setShowDownloadFolderBrowser(false)}
                          className="px-4 py-2 rounded-lg font-medium text-text-main hover:bg-surface-3 transition-colors"
                      >
                          Cancel
                      </button>
                      <button 
                          onClick={selectDownloadFolder}
                          disabled={!downloadBrowserPath}
                          className="px-6 py-2 rounded-lg font-bold bg-brand hover:bg-brand-hover disabled:opacity-50 text-black transition-colors flex items-center gap-2"
                      >
                          <FolderOpen size={16} /> Select This Folder
                      </button>
                  </div>
              </div>
          </div>
      )}

    </div>
  );
};
