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
 * - Library Intelligence: AI-powered features
 *   - Generative Genre Enrichment: Uses Gemini AI to populate genre metadata
 *   - Mood & Energy Analysis: Analyzes mood, energy, tempo, BPM for AI DJ
 * - Activity Log: Debug log viewer
 * 
 * Folder browser dialogs allow navigation and selection of:
 * - Music scan directories
 * - Spotify download destination
 * 
 * AI Features (requires Gemini API key):
 * - Genre enrichment runs during library scans or can be triggered manually
 * - Mood analysis detects emotional characteristics without audio processing
 * - Results are stored in the songs table for AI DJ playlist generation
 * 
 * @module Settings
 */

import React, { useState, useRef, useEffect } from 'react';
import { Wifi, Volume2, HardDrive, Trash2, Terminal, XCircle, SlidersHorizontal, Activity, Layers, Sparkles, FolderOpen, Loader2, AlertTriangle, Plus, X, RefreshCw, Server, MonitorOff, BarChart3, Zap } from 'lucide-react';
import { useStore } from '../store';
import { VisualizerMode, Song } from '../types';
import { parseSong } from '../metadata';
import { api } from '../services/api';
import { Button } from '../components/ui/Button';
import { Page } from '../components/ui/Page';
import { TextInput } from '../components/ui/TextInput';

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
      backendAvailable, scanFolders, loadScanFolders, addScanFolder, removeScanFolder, startBackendScan, startQuickScan
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
  const [geminiKey, setGeminiKey] = useState('');
  const [keySaved, setKeySaved] = useState(false);
  const [showKeySavedMessage, setShowKeySavedMessage] = useState(false);
  const [isEnriching, setIsEnriching] = useState(false);
  const [enrichStatus, setEnrichStatus] = useState('');
  const [forceEnrichment, setForceEnrichment] = useState(false);
  const [enrichProgress, setEnrichProgress] = useState<{
    processedSongs: number;
    totalSongs: number;
    currentBatch: number;
    totalBatches: number;
  } | null>(null);

  // Mood Analysis State
  const [isMoodAnalyzing, setIsMoodAnalyzing] = useState(false);
  const [moodStatus, setMoodStatus] = useState('');
  const [moodProgress, setMoodProgress] = useState<{
    processedSongs: number;
    totalSongs: number;
    currentBatch: number;
    totalBatches: number;
  } | null>(null);

  // Unified Enrichment State (genres + mood + years in one call)
  const [isUnifiedEnriching, setIsUnifiedEnriching] = useState(false);
  const [unifiedStatus, setUnifiedStatus] = useState('');
  const [forceUnified, setForceUnified] = useState(false);
  const [unifiedProgress, setUnifiedProgress] = useState<{
    processedSongs: number;
    totalSongs: number;
    currentBatch: number;
    totalBatches: number;
  } | null>(null);

  // Year Backfill State
  const [yearBackfillStatus, setYearBackfillStatus] = useState('');
  
  // Remaster Detection State
  const [remasterStatus, setRemasterStatus] = useState('');

  const [browserEntries, setBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingBrowser, setLoadingBrowser] = useState(false);

  // Load Gemini Key
  useEffect(() => {
      const loadGeminiKey = async () => {
          try {
              const key = await api.getSetting('gemini_api_key');
              if (key) {
                  setGeminiKey(key);
                  setKeySaved(true);
              }
          } catch (e) {
              console.error('Failed to load Gemini key:', e);
          }
      };
      loadGeminiKey();
  }, []);

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
          // Start from drives on Windows, home on others
          const startPath = navigator.platform.toLowerCase().includes('win') ? 'drives' : undefined;
          const result = await api.browseFolder(startPath);
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

  // Helper to determine if an entry is a drive letter (e.g., "C:")
  const isDriveLetter = (name: string): boolean => {
      return /^[A-Z]:$/.test(name);
  };

  // Download folder browser functions
  const openDownloadFolderBrowser = async () => {
      setShowDownloadFolderBrowser(true);
      setLoadingDownloadBrowser(true);
      try {
          // Start from drives on Windows if no download path set, otherwise use current path
          let startPath: string | undefined = spotifyDownloadPath;
          if (!startPath && navigator.platform.toLowerCase().includes('win')) {
              startPath = 'drives';
          }
          const result = await api.browseFolder(startPath);
          setDownloadBrowserPath(result.currentPath);
          setDownloadBrowserEntries(result.entries);
      } catch (e) {
          console.error("Failed to browse folder", e);
          // Fallback: try without a path on error
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
          case 'error': return 'text-error';
          case 'warn': return 'text-warning';
          case 'success': return 'text-success';
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
        <Page className="max-w-4xl mx-auto">
            <h1 className="text-display mb-10">Settings</h1>

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
            <h2 className="text-card font-semibold text-text-main">Library Management</h2>
            {backendAvailable ? (
                <span className="flex items-center gap-1 text-xs bg-success/20 text-success px-2 py-1 rounded-full">
                    <Server size={12} /> Backend Connected
                </span>
            ) : (
                <span className="flex items-center gap-1 text-xs bg-warning/20 text-warning px-2 py-1 rounded-full">
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
                                <Button
                                    variant="ghost"
                                    onClick={() => removeScanFolder(folder.id)}
                                    className="p-2 text-text-subtle hover:text-error"
                                    title="Remove folder"
                                    aria-label="Remove folder"
                                >
                                    <X size={18} />
                                </Button>
                            </div>
                        ))
                    )}
                </div>
                
                {/* Add folder & Scan buttons */}
                <div className="flex items-center gap-3 flex-wrap">
                    <Button
                        variant="secondary"
                        onClick={openFolderBrowser}
                        leftIcon={<Plus size={18} />}
                        className="font-bold"
                    >
                        Add Folder
                    </Button>
                    <Button
                        variant="secondary"
                        onClick={startQuickScan}
                        disabled={isScanning || scanFolders.length === 0}
                        leftIcon={isScanning ? <Loader2 size={18} className="animate-spin" /> : <Zap size={18} />}
                        className="font-bold"
                        title="Fast scan using signatures - detects new/changed/deleted files only"
                    >
                        Quick Scan
                    </Button>
                    <Button
                        variant="primary"
                        accent="brand"
                        onClick={startBackendScan}
                        disabled={isScanning || scanFolders.length === 0}
                        leftIcon={isScanning ? <Loader2 size={18} className="animate-spin" /> : <RefreshCw size={18} />}
                        className="font-bold"
                        title="Performs a complete rescan of all configured folders"
                    >
                        {isScanning ? 'Rescanning...' : 'Full Rescan'}
                    </Button>
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
                    <Button
                        variant="secondary"
                        onClick={handleScanClick}
                        disabled={isScanning}
                        leftIcon={isScanning ? <Loader2 size={20} className="animate-spin" /> : <FolderOpen size={20} />}
                        className="rounded-full py-3 px-6 font-bold"
                    >
                        {isScanning ? 'Scanning...' : 'Scan Local Directory'}
                    </Button>
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
                    <option value="SPECTRUM">Spectrum Bars</option>
                    <option value="AURORA">Ambient Aurora</option>
                    <option value="CIRCULAR">Circular Pulse</option>
                    <option value="PARTICLES">Particle Storm</option>
                    <option value="NEBULA">Deep Space Nebula</option>
                    <option value="FLAME_SPECTRUM">Flame Spectrum Crown</option>
                    <option value="STARDUST_HALO">Stardust Pulse Halo</option>
                    <option value="AURORA_RIBBON">Aurora Ribbon</option>
                    <option value="ELECTRIC_ARC">Electric Arc Wireframe</option>
                    <option value="GRASS_OSCILLOSCOPE">Growing Grass Oscilloscope</option>
                    <option value="CRYSTAL_SHARDS">Crystal Shards Burst</option>
                    <option value="WATERCOLOR_BLOOM">Watercolor Bloom</option>
                    <option value="ICE_FRACTURE">Ice Fracture Pulse</option>
                    <option value="FIREFLY_FIELD">Holiday Firefly Field</option>
                    <option value="VINYL_SPIN">Vinyl Spin Overlay</option>
                    <option value="BEAT_ORBS">Beat Explosion Orbs</option>
                    <option value="TUNNEL_WAVEFORM">3D Tunnel Waveform</option>
                    <option value="GLASS_SHARDS">Reflective Glass Shards</option>
                    <option value="WIND_FIELD">Soft Wind Field</option>
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
                    <TextInput
                        type="text"
                        value={tempClientId}
                        onChange={(e) => setTempClientId(e.target.value)}
                        placeholder="Enter Client ID"
                        className="w-full px-4 py-3"
                    />
                </div>
                <div>
                    <label className="block text-xs font-bold text-text-subtle uppercase mb-2">Client Secret</label>
                    <TextInput
                        type="password"
                        value={tempClientSecret}
                        onChange={(e) => setTempClientSecret(e.target.value)}
                        placeholder="Enter Client Secret"
                        className="w-full px-4 py-3"
                    />
                </div>
            </div>

            <div className="flex items-center justify-between mt-2">
                <p className="text-xs text-text-subtle">
                    Create an app at <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-brand hover:underline">developer.spotify.com</a> to get these keys.
                </p>
                <div className="flex items-center gap-3">
                    {saveSuccess && (
                        <span className="text-success text-sm font-bold animate-in fade-in slide-in-from-right-4">
                            Saved!
                        </span>
                    )}
                    <Button
                        variant="primary"
                        accent="brand"
                        onClick={handleSaveCredentials}
                        className="rounded-full px-6 py-2 text-sm font-bold"
                    >
                        Save Credentials
                    </Button>
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
                    <TextInput
                        type="text"
                        value={spotifyDownloadPath}
                        onChange={(e) => setSpotifyDownloadPath(e.target.value)}
                        placeholder="Default: AppData/ViiB-MediaHub/spotify_downloads"
                        className="flex-1 px-4 py-3"
                        inputClassName="font-mono text-sm"
                    />
                    <Button
                        variant="secondary"
                        onClick={openDownloadFolderBrowser}
                        className="py-3 px-4"
                        title="Browse folders"
                        aria-label="Browse folders"
                    >
                        <FolderOpen size={18} />
                    </Button>
                    {downloadPathSaved && (
                        <span className="text-success text-sm font-bold">
                            Saved!
                        </span>
                    )}
                    <Button
                        variant="primary"
                        accent="brand"
                        onClick={handleSaveDownloadPath}
                        className="py-3 px-6 rounded-lg text-sm font-bold"
                    >
                        Save
                    </Button>
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
                        <span className="text-success text-sm font-bold">
                            Saved!
                        </span>
                    )}
                    <Button
                        variant="primary"
                        accent="brand"
                        onClick={handleSaveConcurrentDownloads}
                        className="py-3 px-6 rounded-lg transition-colors text-sm font-bold"
                    >
                        Save
                    </Button>
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
                    <Button
                        onClick={() => setStreamingQuality('high')}
                        variant={streamingQuality === 'high' ? 'primary' : 'secondary'}
                        className="flex-1 py-2 px-4 rounded-lg text-sm font-bold"
                    >
                        High (320kbps)
                    </Button>
                    <Button
                        onClick={() => setStreamingQuality('medium')}
                        variant={streamingQuality === 'medium' ? 'primary' : 'secondary'}
                        className="flex-1 py-2 px-4 rounded-lg text-sm font-bold"
                    >
                        Medium (160kbps)
                    </Button>
                    <Button
                        onClick={() => setStreamingQuality('low')}
                        variant={streamingQuality === 'low' ? 'primary' : 'secondary'}
                        className="flex-1 py-2 px-4 rounded-lg text-sm font-bold"
                    >
                        Low (96kbps)
                    </Button>
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
                    <Button
                        variant="ghost"
                        onClick={resetStreamingStats}
                        className="px-0 py-0 text-xs text-text-subtle hover:text-text-secondary"
                    >
                        Reset Stats
                    </Button>
                </div>
                
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                    {/* Total Streams */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-brand">{streamingStats.totalStreams}</div>
                        <div className="text-xs text-text-subtle">Total Streams</div>
                    </div>
                    
                    {/* Success Rate */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-success">
                            {streamingStats.totalStreams > 0 
                                ? Math.round((streamingStats.successfulStreams / streamingStats.totalStreams) * 100)
                                : 100}%
                        </div>
                        <div className="text-xs text-text-subtle">Success Rate</div>
                    </div>
                    
                    {/* Buffering Events */}
                    <div className="bg-surface-2 p-3 rounded-lg">
                        <div className="text-lg font-bold text-warning">{streamingStats.bufferingEvents}</div>
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
                                <span className="text-error">Network: {streamingStats.errorsByType.network}</span>
                            )}
                            {streamingStats.errorsByType.auth > 0 && (
                                <span className="text-accent-orange">Auth: {streamingStats.errorsByType.auth}</span>
                            )}
                            {streamingStats.errorsByType.unavailable > 0 && (
                                <span className="text-warning">Unavailable: {streamingStats.errorsByType.unavailable}</span>
                            )}
                            {streamingStats.errorsByType.unknown > 0 && (
                                <span className="text-text-subtle">Unknown: {streamingStats.errorsByType.unknown}</span>
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
            <Button
                variant="secondary"
                onClick={() => setShowResetConfirm(true)}
                leftIcon={<Trash2 size={16} />}
                className="px-4 py-2 font-bold text-sm hover:bg-error/20 hover:text-error hover:border-error/50"
            >
                Reset Library
            </Button>
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
              <Button
                variant="ghost"
                onClick={clearLogs}
                leftIcon={<XCircle size={14} />}
                className="px-0 py-0 text-xs text-text-secondary hover:text-text-main"
              >
                  Clear
              </Button>
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
                              <span className="text-text-subtle break-all">{log.message}</span>
                          </div>
                          {log.details && (
                              <div className="ml-24 mt-1 text-text-subtle break-all">
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
                  <div className="flex items-center gap-4 text-error mb-4">
                      <AlertTriangle size={32} />
                      <h2 className="text-xl font-bold text-white">Reset Library?</h2>
                  </div>
                  <p className="text-text-secondary mb-6 leading-relaxed">
                      Are you sure you want to delete your entire music library? This will remove all songs, playlists, and cached metadata from the database. <br/><br/>
                      <span className="text-error font-bold">This action cannot be undone.</span>
                  </p>
                  
                  <div className="flex items-center justify-end gap-3">
                      <Button
                          variant="ghost"
                          onClick={() => setShowResetConfirm(false)}
                          disabled={isResetting}
                      >
                          Cancel
                      </Button>
                      <Button
                          variant="primary"
                          accent="destructive"
                          onClick={confirmResetLibrary}
                          disabled={isResetting}
                          leftIcon={isResetting ? <Loader2 size={16} className="animate-spin" /> : undefined}
                          className="px-6 py-2 font-bold text-white"
                      >
                          {isResetting ? 'Resetting...' : 'Yes, Delete Everything'}
                      </Button>
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
                <div className="flex gap-2">
                    <TextInput
                      type="password"
                      value={geminiKey}
                      onChange={(e) => {
                          setGeminiKey(e.target.value);
                          setKeySaved(false);
                      }}
                      placeholder={keySaved ? "API Key Saved (Hidden)" : "Enter your Gemini API Key"}
                      className="flex-1 bg-surface-3"
                      disabled={isEnriching}
                    />
                    <Button
                        variant="secondary"
                        onClick={async () => {
                            try {
                                await api.setSetting('gemini_api_key', geminiKey);
                                setKeySaved(true);
                                setShowKeySavedMessage(true);
                                setTimeout(() => setShowKeySavedMessage(false), 3000);
                            } catch (e) {
                                console.error('Failed to save key:', e);
                            }
                        }}
                        className="text-sm font-bold"
                    >
                        Save
                    </Button>
                </div>
                {showKeySavedMessage && (
                    <p className="text-success text-xs mt-1 font-bold">API Key saved successfully!</p>
                )}
                {keySaved && !showKeySavedMessage && (
                    <p className="text-brand text-xs mt-1 flex items-center gap-1">
                        <span className="w-2 h-2 rounded-full bg-brand inline-block"></span>
                        Key saved & active. Enrichment will run during library scans.
                    </p>
                )}
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

                            <Button
                onClick={() => {
                  if (!geminiKey && !keySaved) {
                      alert("Please enter or save an API Key first.");
                      return;
                  }

                  setIsEnriching(true);
                  setEnrichStatus('Connecting to enrichment service...');
                  setEnrichProgress(null);

                  const eventSource = api.enrichGenresStream(forceEnrichment, (progress) => {
                    setEnrichStatus(progress.message);
                    
                    if (progress.status === 'started' || progress.status === 'processing' || progress.status === 'batch_complete') {
                      setEnrichProgress({
                        processedSongs: progress.processedSongs,
                        totalSongs: progress.totalSongs,
                        currentBatch: progress.currentBatch,
                        totalBatches: progress.totalBatches,
                      });
                    }
                    
                    if (progress.status === 'complete') {
                      setIsEnriching(false);
                      // Keep progress visible for a moment
                      setTimeout(() => setEnrichProgress(null), 5000);
                    }
                    
                    if (progress.status === 'error') {
                      setIsEnriching(false);
                      setEnrichProgress(null);
                      setEnrichStatus(`Error: ${progress.error || progress.message}`);
                    }
                  });

                  // Store eventSource for cleanup if needed
                  return () => eventSource.close();
                }}
                disabled={isEnriching || (!geminiKey && !keySaved)}
                                variant={isEnriching || (!geminiKey && !keySaved) ? 'secondary' : 'primary'}
                                accent="brand"
                                className="self-start px-4 py-2 font-bold"
              >
                {isEnriching ? (
                    <>
                        <span className="animate-spin mr-2">⏳</span> Processing Library...
                    </>
                ) : (
                    <>✨ Enrich Library Genres</>
                )}
                            </Button>
              
              {/* Progress Bar */}
              {enrichProgress && enrichProgress.totalSongs > 0 && (
                <div className="w-full">
                  <div className="flex justify-between text-xs text-text-subtle mb-1">
                    <span>Batch {enrichProgress.currentBatch} of {enrichProgress.totalBatches}</span>
                    <span>{enrichProgress.processedSongs} / {enrichProgress.totalSongs} songs</span>
                  </div>
                  <div className="w-full bg-surface-3 rounded-full h-2 overflow-hidden">
                    <div 
                      className="bg-brand h-full rounded-full transition-all duration-300 ease-out"
                      style={{ 
                        width: `${Math.round((enrichProgress.processedSongs / enrichProgress.totalSongs) * 100)}%` 
                      }}
                    />
                  </div>
                </div>
              )}
              
              {enrichStatus && (
                  <div className={`text-sm mt-2 ${
                      enrichStatus.startsWith('Error') ? 'text-error' : 
                      enrichStatus.includes('complete') || enrichStatus.startsWith('Success') ? 'text-success' : 
                      'text-text-subtle'
                  }`}>
                      {enrichStatus}
                  </div>
              )}
            </div>
          </div>

          {/* Unified AI Enrichment - RECOMMENDED */}
          <div className="bg-surface-1 rounded-lg p-4 border-2 border-brand">
            <div className="flex items-center gap-2 mb-2">
              <span className="px-2 py-0.5 text-xs font-bold bg-brand text-black rounded">RECOMMENDED</span>
              <h3 className="text-lg font-bold text-text-main">Unified AI Enrichment</h3>
            </div>
            <p className="text-text-subtle text-sm mb-4">
              Enrich your entire library with genres, mood, energy, tempo, BPM, and original year detection 
              in a <strong>single efficient operation</strong>. Uses TOON format for 3x better efficiency 
              and processes 200 songs per batch.
            </p>

            <div className="flex flex-col gap-4">
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="force-unified"
                  checked={forceUnified}
                  onChange={(e) => setForceUnified(e.target.checked)}
                  disabled={isUnifiedEnriching}
                  className="rounded border-surface-border bg-surface-3 text-brand focus:ring-brand"
                />
                <label htmlFor="force-unified" className="text-sm text-text-main cursor-pointer select-none">
                  Force re-analyze all songs (overwrites existing metadata)
                </label>
              </div>

              <Button
                onClick={() => {
                  if (!geminiKey && !keySaved) {
                    alert("Please enter or save an API Key first.");
                    return;
                  }

                  setIsUnifiedEnriching(true);
                  setUnifiedStatus('Connecting to unified enrichment service...');
                  setUnifiedProgress(null);

                  const eventSource = api.enrichAllMetadataStream(forceUnified, (progress) => {
                    setUnifiedStatus(progress.message);
                    
                    if (progress.status === 'started' || progress.status === 'processing' || progress.status === 'batch_complete') {
                      setUnifiedProgress({
                        processedSongs: progress.processedSongs,
                        totalSongs: progress.totalSongs,
                        currentBatch: progress.currentBatch,
                        totalBatches: progress.totalBatches,
                      });
                    }
                    
                    if (progress.status === 'complete') {
                      setIsUnifiedEnriching(false);
                      setTimeout(() => setUnifiedProgress(null), 5000);
                    }
                    
                    if (progress.status === 'error') {
                      setIsUnifiedEnriching(false);
                      setUnifiedProgress(null);
                      setUnifiedStatus(`Error: ${progress.error || progress.message}`);
                    }
                  });

                  return () => eventSource.close();
                }}
                disabled={isUnifiedEnriching || (!geminiKey && !keySaved)}
                variant={isUnifiedEnriching || (!geminiKey && !keySaved) ? 'secondary' : 'primary'}
                accent="brand"
                className="self-start px-6 py-3 font-bold text-base"
              >
                {isUnifiedEnriching ? (
                  <>
                    <span className="animate-spin mr-2">⏳</span> Enriching Library...
                  </>
                ) : (
                  <>🚀 Enrich All Metadata</>
                )}
              </Button>
              
              {/* Progress Bar */}
              {unifiedProgress && unifiedProgress.totalSongs > 0 && (
                <div className="w-full">
                  <div className="flex justify-between text-xs text-text-subtle mb-1">
                    <span>Batch {unifiedProgress.currentBatch} of {unifiedProgress.totalBatches}</span>
                    <span>{unifiedProgress.processedSongs} / {unifiedProgress.totalSongs} songs</span>
                  </div>
                  <div className="w-full bg-surface-3 rounded-full h-2.5 overflow-hidden">
                    <div 
                      className="bg-brand h-full rounded-full transition-all duration-300 ease-out"
                      style={{ 
                        width: `${Math.round((unifiedProgress.processedSongs / unifiedProgress.totalSongs) * 100)}%` 
                      }}
                    />
                  </div>
                </div>
              )}
              
              {unifiedStatus && (
                <div className={`text-sm mt-2 ${
                  unifiedStatus.startsWith('Error') ? 'text-error' : 
                  unifiedStatus.includes('complete') || unifiedStatus.startsWith('Success') ? 'text-success' : 
                  'text-text-subtle'
                }`}>
                  {unifiedStatus}
                </div>
              )}
            </div>
          </div>

          {/* Mood/Energy Analysis */}
          <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
            <h3 className="text-lg font-bold text-text-main mb-2">Mood & Energy Analysis</h3>
            <p className="text-text-subtle text-sm mb-4">
              Analyze your library to detect mood, energy level, tempo, and estimated BPM for each song. 
              This enables the AI DJ to create better playlists matching your vibe.
            </p>

            <div className="flex flex-col gap-3">
                            <Button
                onClick={() => {
                  if (!geminiKey && !keySaved) {
                    alert("Please enter or save an API Key first.");
                    return;
                  }

                  setIsMoodAnalyzing(true);
                  setMoodStatus('Connecting to mood analysis service...');
                  setMoodProgress(null);

                  const eventSource = api.enrichMoodStream((progress) => {
                    setMoodStatus(progress.message);
                    
                    if (progress.status === 'started' || progress.status === 'processing' || progress.status === 'batch_complete') {
                      setMoodProgress({
                        processedSongs: progress.processedSongs,
                        totalSongs: progress.totalSongs,
                        currentBatch: progress.currentBatch,
                        totalBatches: progress.totalBatches,
                      });
                    }
                    
                    if (progress.status === 'complete') {
                      setIsMoodAnalyzing(false);
                      setTimeout(() => setMoodProgress(null), 5000);
                    }
                    
                    if (progress.status === 'error') {
                      setIsMoodAnalyzing(false);
                      setMoodProgress(null);
                      setMoodStatus(`Error: ${progress.error || progress.message}`);
                    }
                  });

                  return () => eventSource.close();
                }}
                disabled={isMoodAnalyzing || (!geminiKey && !keySaved)}
                                variant={isMoodAnalyzing || (!geminiKey && !keySaved) ? 'secondary' : 'primary'}
                                accent="brand"
                                className="self-start px-4 py-2 font-bold"
              >
                {isMoodAnalyzing ? (
                  <>
                    <span className="animate-spin mr-2">⏳</span> Analyzing Moods...
                  </>
                ) : (
                  <>🎭 Analyze Library Moods</>
                )}
                            </Button>
              
              {/* Progress Bar */}
              {moodProgress && moodProgress.totalSongs > 0 && (
                <div className="w-full">
                  <div className="flex justify-between text-xs text-text-subtle mb-1">
                    <span>Batch {moodProgress.currentBatch} of {moodProgress.totalBatches}</span>
                    <span>{moodProgress.processedSongs} / {moodProgress.totalSongs} songs</span>
                  </div>
                  <div className="w-full bg-surface-3 rounded-full h-2 overflow-hidden">
                    <div 
                                            className="bg-brand h-full rounded-full transition-all duration-300 ease-out"
                      style={{ 
                        width: `${Math.round((moodProgress.processedSongs / moodProgress.totalSongs) * 100)}%` 
                      }}
                    />
                  </div>
                </div>
              )}
              
              {moodStatus && (
                <div className={`text-sm mt-2 ${
                                    moodStatus.startsWith('Error') ? 'text-error' : 
                                    moodStatus.includes('complete') || moodStatus.startsWith('Success') ? 'text-success' : 
                  'text-text-subtle'
                }`}>
                  {moodStatus}
                </div>
              )}
            </div>
          </div>

          {/* Year Backfill */}
          <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
            <h3 className="text-lg font-bold text-text-main mb-2">Year Data Backfill</h3>
            <p className="text-text-subtle text-sm mb-4">
              Populate missing song year values from album metadata release dates.
              This enables the AI DJ to correctly filter by decade (e.g., "90s hip hop").
              Run this after enriching album metadata from Spotify.
            </p>

            <div className="flex flex-col gap-3">
              <Button
                onClick={async () => {
                  setYearBackfillStatus('Backfilling years...');
                  try {
                    const result = await api.backfillSongYears();
                    setYearBackfillStatus(`Success: ${result.message}`);
                  } catch (err: unknown) {
                    setYearBackfillStatus(`Error: ${err instanceof Error ? err.message : 'Failed to backfill years'}`);
                  }
                }}
                variant="primary"
                accent="brand"
                className="self-start px-4 py-2 font-bold"
              >
                📅 Backfill Song Years
              </Button>
              
              {yearBackfillStatus && (
                <div className={`text-sm mt-2 ${
                  yearBackfillStatus.startsWith('Error') ? 'text-error' : 
                  yearBackfillStatus.startsWith('Success') ? 'text-success' : 
                  'text-text-subtle'
                }`}>
                  {yearBackfillStatus}
                </div>
              )}
            </div>
          </div>

          {/* Remaster Detection */}
          <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
            <h3 className="text-lg font-bold text-text-main mb-2">Original Year Detection</h3>
            <p className="text-text-subtle text-sm mb-4">
              Detect songs that may have remaster dates instead of original release years.
              Step 1: Pattern matching finds "Remastered", "Deluxe Edition", etc.
              Step 2: AI analyzes flagged songs to determine the original release year.
            </p>

            <div className="flex flex-col gap-4">
              {/* Step 1: Detect Remasters */}
              <div>
                <Button
                  onClick={async () => {
                    setRemasterStatus('Detecting remasters...');
                    try {
                      const result = await api.detectRemasters();
                      setRemasterStatus(`Success: ${result.message}`);
                    } catch (err: unknown) {
                      setRemasterStatus(`Error: ${err instanceof Error ? err.message : 'Failed to detect remasters'}`);
                    }
                  }}
                  variant="primary"
                  accent="brand"
                  className="self-start px-4 py-2 font-bold"
                >
                  🔍 Step 1: Detect Remasters
                </Button>
                
                {remasterStatus && (
                  <div className={`text-sm mt-2 ${
                    remasterStatus.startsWith('Error') ? 'text-error' : 
                    remasterStatus.startsWith('Success') ? 'text-success' : 
                    'text-text-subtle'
                  }`}>
                    {remasterStatus}
                  </div>
                )}
              </div>

              {/* Step 2: AI Year Enrichment */}
              <div>
                <Button
                  onClick={() => {
                    setRemasterStatus('Starting AI year analysis...');
                    api.enrichOriginalYearsStream(
                      (progress) => {
                        setRemasterStatus(`Processing: ${progress.message} (${progress.processedSongs}/${progress.totalSongs})`);
                      },
                      (progress) => {
                        setRemasterStatus(`Success: ${progress.message}`);
                      },
                      (error) => {
                        setRemasterStatus(`Error: ${error}`);
                      }
                    );
                  }}
                  variant="primary"
                  accent="brand"
                  className="self-start px-4 py-2 font-bold"
                >
                  ✨ Step 2: AI Year Analysis
                </Button>
              </div>
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
                      <Button
                          variant="ghost"
                          onClick={() => setShowFolderBrowser(false)}
                          className="p-2"
                          aria-label="Close folder browser"
                      >
                          <X size={20} />
                      </Button>
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
                              {browserEntries.map((entry, idx) => {
                                  const isRoot = isDriveLetter(entry.name);
                                  return (
                                      <button
                                          key={idx}
                                          onClick={() => navigateFolder(entry.path)}
                                          className="w-full flex items-center gap-3 p-3 hover:bg-surface-hover transition-colors text-left"
                                      >
                                          {isRoot ? (
                                              <HardDrive size={18} className="text-brand flex-shrink-0" />
                                          ) : (
                                              <FolderOpen size={18} className="text-brand flex-shrink-0" />
                                          )}
                                          <span className="text-text-main truncate font-medium">{entry.name}</span>
                                          {isRoot && <span className="text-xs text-text-subtle ml-auto">Drive</span>}
                                      </button>
                                  );
                              })}
                              {browserEntries.length === 0 && (
                                  <div className="p-4 text-center text-text-subtle">No subfolders found</div>
                              )}
                          </div>
                      )}
                  </div>
                  
                  {/* Actions */}
                  <div className="flex items-center justify-end gap-3">
                      <Button
                          variant="ghost"
                          onClick={() => setShowFolderBrowser(false)}
                      >
                          Cancel
                      </Button>
                      <Button
                          variant="primary"
                          accent="brand"
                          onClick={selectCurrentFolder}
                          disabled={!browserPath}
                          leftIcon={<Plus size={16} />}
                          className="px-6 py-2 font-bold"
                      >
                          Add This Folder
                      </Button>
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
                      <Button
                          variant="ghost"
                          onClick={() => setShowDownloadFolderBrowser(false)}
                          className="p-2"
                          aria-label="Close download folder browser"
                      >
                          <X size={20} />
                      </Button>
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
                              {downloadBrowserEntries.map((entry, idx) => {
                                  const isRoot = isDriveLetter(entry.name);
                                  return (
                                      <button
                                          key={idx}
                                          onClick={() => navigateDownloadFolder(entry.path)}
                                          className="w-full flex items-center gap-3 p-3 hover:bg-surface-hover transition-colors text-left"
                                      >
                                          {isRoot ? (
                                              <HardDrive size={18} className="text-brand flex-shrink-0" />
                                          ) : (
                                              <FolderOpen size={18} className="text-brand flex-shrink-0" />
                                          )}
                                          <span className="text-text-main truncate font-medium">{entry.name}</span>
                                          {isRoot && <span className="text-xs text-text-subtle ml-auto">Drive</span>}
                                      </button>
                                  );
                              })}
                              {downloadBrowserEntries.length === 0 && (
                                  <div className="p-4 text-center text-text-subtle">No subfolders found</div>
                              )}
                          </div>
                      )}
                  </div>
                  
                  {/* Actions */}
                  <div className="flex items-center justify-end gap-3">
                      <Button
                          variant="ghost"
                          onClick={() => setShowDownloadFolderBrowser(false)}
                      >
                          Cancel
                      </Button>
                      <Button
                          variant="primary"
                          accent="brand"
                          onClick={selectDownloadFolder}
                          disabled={!downloadBrowserPath}
                          leftIcon={<FolderOpen size={16} />}
                          className="px-6 py-2 font-bold"
                      >
                          Select This Folder
                      </Button>
                  </div>
              </div>
          </div>
      )}

        </Page>
  );
};
