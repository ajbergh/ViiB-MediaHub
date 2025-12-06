/**
 * ViiB MediaHub - First Launch Configuration Dialog
 * 
 * Welcome screen shown on first launch to guide users through initial setup.
 * 
 * Features:
 * - Multi-step wizard interface
 * - Music folder selection with browser
 * - Optional Spotify integration setup
 * - Skip option for minimal setup
 * - Progress indicators
 * 
 * Setup Steps:
 * 1. Welcome screen with overview
 * 2. Add music folder(s)
 * 3. Configure Spotify credentials (optional)
 * 4. Complete and start scanning
 * 
 * @module FirstLaunchDialog
 */

import React, { useState, useEffect } from 'react';
import { Music, FolderOpen, Wifi, Check, Loader2, X, Plus, ChevronRight, Sparkles, HardDrive } from 'lucide-react';
import { useStore } from '../store';
import { api } from '../services/api';

interface FirstLaunchDialogProps {
  isOpen: boolean;
  onComplete: () => void;
}

export const FirstLaunchDialog: React.FC<FirstLaunchDialogProps> = ({ isOpen, onComplete }) => {
  const [step, setStep] = useState(1);
  const [isScanning, setIsScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState('');

  // Folder browser state
  const [showFolderBrowser, setShowFolderBrowser] = useState(false);
  const [browserPath, setBrowserPath] = useState('');
  const [browserEntries, setBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingBrowser, setLoadingBrowser] = useState(false);

  // Spotify credentials state
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [savingCredentials, setSavingCredentials] = useState(false);

  // Gemini API key state
  const [geminiKey, setGeminiKey] = useState('');
  const [savingGemini, setSavingGemini] = useState(false);

  // Spotify download folder browser state
  const [showDownloadFolderBrowser, setShowDownloadFolderBrowser] = useState(false);
  const [downloadBrowserPath, setDownloadBrowserPath] = useState('');
  const [downloadBrowserEntries, setDownloadBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingDownloadBrowser, setLoadingDownloadBrowser] = useState(false);
  const [spotifyDownloadPath, setSpotifyDownloadPath] = useState('');

  const {
    scanFolders,
    loadScanFolders,
    addScanFolder,
    removeScanFolder,
    setSpotifyCredentials,
    addLog
  } = useStore();

  // Load scan folders when dialog opens
  useEffect(() => {
    if (isOpen) {
      loadScanFolders();
    }
  }, [isOpen, loadScanFolders]);

  if (!isOpen) return null;

  // Helper to determine if an entry is a drive letter (e.g., "C:")
  const isDriveLetter = (name: string): boolean => {
      return /^[A-Z]:$/.test(name);
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
    }
  };

  const handleSaveSpotifyCredentials = async () => {
    if (!clientId.trim() || !clientSecret.trim()) {
      return;
    }

    setSavingCredentials(true);
    try {
      setSpotifyCredentials(clientId, clientSecret);
      await api.saveSpotifyCredentials({
        clientId,
        clientSecret,
        accessToken: '',
        refreshToken: '',
        expiry: 0
      });
      addLog('success', 'Spotify credentials saved');

      // Save download path if specified
      if (spotifyDownloadPath.trim()) {
        try {
          await api.setSetting('spotify_download_path', spotifyDownloadPath);
          addLog('success', `Download location set to: ${spotifyDownloadPath}`);
        } catch (e) {
          addLog('warn', 'Failed to save download location', e);
        }
      }

      setStep(4);
    } catch (e) {
      addLog('error', 'Failed to save Spotify credentials', e);
    } finally {
      setSavingCredentials(false);
    }
  };

  const handleSaveGeminiKey = async () => {
    if (!geminiKey.trim()) return;
    setSavingGemini(true);
    try {
      await api.setSetting('gemini_api_key', geminiKey);
      addLog('success', 'Gemini API key saved');
      setStep(5);
    } catch (e) {
      addLog('error', 'Failed to save Gemini API key', e);
    } finally {
      setSavingGemini(false);
    }
  };

  const handleStartScanAndClose = async () => {
    if (scanFolders.length > 0) {
      try {
        await api.startScan();
        addLog('success', 'Library scan started in background');
      } catch (e) {
        addLog('error', 'Failed to start scan', e);
      }
    }
    onComplete();
  };

  const handleStartScan = async () => {
    if (scanFolders.length === 0) {
      addLog('warn', 'No folders to scan');
      return;
    }

    setIsScanning(true);
    setScanProgress('Starting scan...');

    try {
      await api.startScan();

      // Poll for scan status
      const pollInterval = setInterval(async () => {
        try {
          const status = await api.getScanStatus();
          setScanProgress(status.progress);

          if (!status.scanning) {
            clearInterval(pollInterval);
            setIsScanning(false);
            setScanProgress('');
            onComplete();
          }
        } catch (e) {
          console.error('Failed to poll scan status', e);
        }
      }, 500);

      // Safety timeout after 5 minutes
      setTimeout(() => {
        clearInterval(pollInterval);
        setIsScanning(false);
        onComplete();
      }, 300000);

    } catch (e) {
      addLog('error', 'Failed to start scan', e);
      setIsScanning(false);
      setScanProgress('');
    }
  };

  const handleSkipToEnd = () => {
    setStep(5);
  };

  const handleFinish = () => {
    onComplete();
  };

  // Step 1: Welcome
  const renderWelcome = () => (
    <div className="text-center py-8 animate-fade-in">
      <div className="flex justify-center mb-8">
        <div className="relative">
          <div className="absolute inset-0 bg-brand blur-3xl opacity-20 rounded-full"></div>
          <div className="relative p-8 bg-gradient-to-br from-surface-2 to-surface-1 border border-surface-border rounded-full shadow-2xl shadow-black/50">
            <Music size={64} className="text-brand drop-shadow-lg" />
          </div>
        </div>
      </div>

      <h1 className="text-5xl font-bold text-white mb-6 tracking-tight">
        Welcome to <span className="text-transparent bg-clip-text bg-gradient-to-r from-brand to-brand-hover">ViiB MediaHub</span>
      </h1>

      <p className="text-xl text-text-secondary max-w-2xl mx-auto mb-12 leading-relaxed font-light">
        Your personal music player with powerful library management and Spotify integration.
        Let's get you set up in just a few steps.
      </p>

      <div className="grid grid-cols-4 gap-4 max-w-4xl mx-auto mb-12">
        <div className="group bg-surface-1/50 backdrop-blur-sm border border-surface-border hover:border-brand/50 rounded-2xl p-4 transition-all duration-300 hover:-translate-y-1 hover:shadow-xl hover:shadow-brand/5">
          <div className="flex justify-center mb-4">
            <div className="p-4 bg-surface-2 rounded-xl group-hover:bg-brand/10 transition-colors">
              <FolderOpen size={32} className="text-brand" />
            </div>
          </div>
          <h3 className="text-lg font-bold text-white mb-2">Add Music</h3>
          <p className="text-xs text-text-subtle group-hover:text-text-secondary transition-colors">
            Select folders containing your music collection
          </p>
        </div>

        <div className="group bg-surface-1/50 backdrop-blur-sm border border-surface-border hover:border-brand/50 rounded-2xl p-4 transition-all duration-300 hover:-translate-y-1 hover:shadow-xl hover:shadow-brand/5">
          <div className="flex justify-center mb-4">
            <div className="p-4 bg-surface-2 rounded-xl group-hover:bg-brand/10 transition-colors">
              <Wifi size={32} className="text-brand" />
            </div>
          </div>
          <h3 className="text-lg font-bold text-white mb-2">Spotify Sync</h3>
          <p className="text-xs text-text-subtle group-hover:text-text-secondary transition-colors">
            Optional metadata enrichment and downloads
          </p>
        </div>

        <div className="group bg-surface-1/50 backdrop-blur-sm border border-surface-border hover:border-brand/50 rounded-2xl p-4 transition-all duration-300 hover:-translate-y-1 hover:shadow-xl hover:shadow-brand/5">
          <div className="flex justify-center mb-4">
            <div className="p-4 bg-surface-2 rounded-xl group-hover:bg-brand/10 transition-colors">
              <Sparkles size={32} className="text-brand" />
            </div>
          </div>
          <h3 className="text-lg font-bold text-white mb-2">AI Enrichment</h3>
          <p className="text-xs text-text-subtle group-hover:text-text-secondary transition-colors">
            Auto-tag genres and moods with Gemini AI
          </p>
        </div>

        <div className="group bg-surface-1/50 backdrop-blur-sm border border-surface-border hover:border-brand/50 rounded-2xl p-4 transition-all duration-300 hover:-translate-y-1 hover:shadow-xl hover:shadow-brand/5">
          <div className="flex justify-center mb-4">
            <div className="p-4 bg-surface-2 rounded-xl group-hover:bg-brand/10 transition-colors">
              <Music size={32} className="text-brand" />
            </div>
          </div>
          <h3 className="text-lg font-bold text-white mb-2">Start Listening</h3>
          <p className="text-xs text-text-subtle group-hover:text-text-secondary transition-colors">
            Enjoy your music with powerful features
          </p>
        </div>
      </div>

      <div className="flex flex-col items-center gap-4">
        <button
          onClick={() => setStep(2)}
          className="group relative bg-brand hover:bg-brand-hover text-black font-bold py-4 px-10 rounded-full transition-all shadow-lg shadow-brand/20 hover:shadow-brand/40 hover:scale-105 flex items-center gap-3 text-lg"
        >
          <span>Let's Get Started</span>
          <ChevronRight size={24} className="group-hover:translate-x-1 transition-transform" />
        </button>

        <button
          onClick={handleFinish}
          className="text-text-subtle hover:text-text-secondary text-sm hover:underline transition-colors"
        >
          Skip setup for now
        </button>
      </div>
    </div>
  );

  // Step 2: Add Music Folders
  const renderFolderSetup = () => (
    <div className="py-6">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 bg-brand/10 rounded-lg">
          <FolderOpen size={28} className="text-brand" />
        </div>
        <div>
          <h2 className="text-2xl font-bold text-white">Add Your Music</h2>
          <p className="text-text-secondary">Select folders containing your audio files</p>
        </div>
      </div>

      <div className="bg-surface-1 border border-surface-border rounded-xl p-6 mb-6">
        <p className="text-sm text-text-secondary mb-4">
          ViiB MediaHub will scan these folders for MP3 and OGG files. You can add multiple folders and manage them later in Settings.
        </p>

        {/* List of scan folders */}
        <div className="space-y-2 mb-4">
          {scanFolders.length === 0 ? (
            <div className="text-text-subtle text-sm italic p-6 bg-surface-2 rounded-lg text-center border-2 border-dashed border-surface-border">
              No folders added yet. Click "Add Folder" below to get started.
            </div>
          ) : (
            scanFolders.map(folder => (
              <div key={folder.id} className="flex items-center justify-between bg-surface-2 p-4 rounded-lg border border-surface-border">
                <div className="flex items-center gap-3 flex-1 min-w-0">
                  <FolderOpen size={20} className="text-brand flex-shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="font-mono text-sm text-text-main truncate">{folder.path}</div>
                  </div>
                </div>
                <button
                  onClick={() => removeScanFolder(folder.id)}
                  className="p-2 text-text-subtle hover:text-red-500 transition-colors ml-2"
                  title="Remove folder"
                >
                  <X size={18} />
                </button>
              </div>
            ))
          )}
        </div>

        <button
          onClick={openFolderBrowser}
          className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main font-bold py-3 px-5 rounded-lg transition-all border border-surface-border w-full justify-center"
        >
          <Plus size={18} /> Add Folder
        </button>
      </div>

      <div className="flex items-center justify-between">
        <button
          onClick={() => setStep(1)}
          className="px-5 py-2 text-text-secondary hover:text-text-main transition-colors"
        >
          ← Back
        </button>

        <div className="flex items-center gap-3">
          <button
            onClick={handleSkipToEnd}
            className="px-5 py-2 text-text-subtle hover:text-text-secondary transition-colors underline"
          >
            Skip
          </button>
          <button
            onClick={() => setStep(3)}
            disabled={scanFolders.length === 0}
            className="bg-brand hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-6 rounded-lg transition-all flex items-center gap-2"
          >
            Continue <ChevronRight size={18} />
          </button>
        </div>
      </div>
    </div>
  );

  // Step 3: Spotify Integration (Optional)
  const renderSpotifySetup = () => (
    <div className="py-6">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 bg-brand/10 rounded-lg">
          <Wifi size={28} className="text-brand" />
        </div>
        <div>
          <h2 className="text-2xl font-bold text-white">Spotify Integration</h2>
          <p className="text-text-secondary">Optional - Enhance your library with metadata</p>
        </div>
      </div>

      <div className="bg-surface-1 border border-surface-border rounded-xl p-6 mb-6">
        <p className="text-sm text-text-secondary mb-4">
          Connect your Spotify Developer account to automatically fetch high-quality album artwork,
          artist images, and metadata for your local music library.
        </p>

        <div className="bg-surface-2 border border-brand/30 rounded-lg p-4 mb-6">
          <h4 className="text-sm font-bold text-white mb-2">What you'll get:</h4>
          <ul className="text-sm text-text-secondary space-y-1">
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>High-resolution album artwork and artist images</span>
            </li>
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>Rich metadata including genres, release dates, and descriptions</span>
            </li>
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>Download music directly from Spotify (requires Premium)</span>
            </li>
          </ul>
        </div>

        <div className="space-y-4 mb-4">
          <div>
            <label className="block text-xs font-bold text-text-subtle uppercase mb-2">
              Spotify Client ID
            </label>
            <input
              type="text"
              value={clientId}
              onChange={(e) => setClientId(e.target.value)}
              placeholder="Enter your Client ID"
              className="w-full bg-surface-2 border border-surface-border rounded-lg px-4 py-3 text-text-main focus:border-brand outline-none transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-bold text-text-subtle uppercase mb-2">
              Spotify Client Secret
            </label>
            <input
              type="password"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              placeholder="Enter your Client Secret"
              className="w-full bg-surface-2 border border-surface-border rounded-lg px-4 py-3 text-text-main focus:border-brand outline-none transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-bold text-text-subtle uppercase mb-2">
              Download Location (Optional)
            </label>
            <p className="text-xs text-text-subtle mb-2">
              Where should Spotify downloads be saved? Leave empty for default location.
            </p>
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={spotifyDownloadPath}
                onChange={(e) => setSpotifyDownloadPath(e.target.value)}
                placeholder="Default: AppData/ViiB-MediaHub/spotify_downloads"
                className="flex-1 bg-surface-2 border border-surface-border rounded-lg px-4 py-3 text-text-main focus:border-brand outline-none font-mono text-sm"
              />
              <button
                onClick={openDownloadFolderBrowser}
                className="bg-surface-2 border border-surface-border hover:bg-surface-hover text-text-main font-bold py-3 px-4 rounded-lg transition-colors flex items-center gap-2"
                title="Browse folders"
              >
                <FolderOpen size={18} />
              </button>
            </div>
          </div>
        </div>

        <div className="text-xs text-text-subtle">
          Don't have credentials? Create a free app at{' '}
          <a
            href="https://developer.spotify.com/dashboard"
            target="_blank"
            rel="noreferrer"
            className="text-brand hover:underline"
          >
            developer.spotify.com/dashboard
          </a>
        </div>
      </div>

      <div className="flex items-center justify-between">
        <button
          onClick={() => setStep(2)}
          className="px-5 py-2 text-text-secondary hover:text-text-main transition-colors"
        >
          ← Back
        </button>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setStep(4)}
            className="px-5 py-2 text-text-subtle hover:text-text-secondary transition-colors underline"
          >
            Skip for now
          </button>
          <button
            onClick={handleSaveSpotifyCredentials}
            disabled={!clientId.trim() || !clientSecret.trim() || savingCredentials}
            className="bg-brand hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-6 rounded-lg transition-all flex items-center gap-2"
          >
            {savingCredentials ? (
              <>
                <Loader2 size={18} className="animate-spin" />
                Saving...
              </>
            ) : (
              <>
                Save & Continue <ChevronRight size={18} />
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );

  // Step 4: Gemini Integration (Optional)
  const renderGeminiSetup = () => (
    <div className="py-6">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 bg-brand/10 rounded-lg">
          <Sparkles size={28} className="text-brand" />
        </div>
        <div>
          <h2 className="text-2xl font-bold text-white">AI Enrichment</h2>
          <p className="text-text-secondary">Optional - Enhance metadata with Google Gemini</p>
        </div>
      </div>

      <div className="bg-surface-1 border border-surface-border rounded-xl p-6 mb-6">
        <p className="text-sm text-text-secondary mb-4">
          Connect your Google Gemini API key to automatically fill in missing genres, moods, and other metadata using AI analysis.
        </p>

        <div className="bg-surface-2 border border-brand/30 rounded-lg p-4 mb-6">
          <h4 className="text-sm font-bold text-white mb-2">Capabilities:</h4>
          <ul className="text-sm text-text-secondary space-y-1">
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>Intelligent genre classification</span>
            </li>
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>Mood and style detection</span>
            </li>
            <li className="flex items-start gap-2">
              <Check size={16} className="text-brand mt-0.5 flex-shrink-0" />
              <span>Contextual metadata enrichment</span>
            </li>
          </ul>
        </div>

        <div className="space-y-4 mb-4">
          <div>
            <label className="block text-xs font-bold text-text-subtle uppercase mb-2">
              Gemini API Key
            </label>
            <input
              type="password"
              value={geminiKey}
              onChange={(e) => setGeminiKey(e.target.value)}
              placeholder="Enter your Gemini API Key"
              className="w-full bg-surface-2 border border-surface-border rounded-lg px-4 py-3 text-text-main focus:border-brand outline-none transition-colors"
            />
          </div>
        </div>

        <div className="text-xs text-text-subtle">
          Don't have a key? Get one at{' '}
          <a
            href="https://makersuite.google.com/app/apikey"
            target="_blank"
            rel="noreferrer"
            className="text-brand hover:underline"
          >
            makersuite.google.com
          </a>
        </div>
      </div>

      <div className="flex items-center justify-between">
        <button
          onClick={() => setStep(3)}
          className="px-5 py-2 text-text-secondary hover:text-text-main transition-colors"
        >
          ← Back
        </button>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setStep(5)}
            className="px-5 py-2 text-text-subtle hover:text-text-secondary transition-colors underline"
          >
            Skip for now
          </button>
          <button
            onClick={handleSaveGeminiKey}
            disabled={!geminiKey.trim() || savingGemini}
            className="bg-brand hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-6 rounded-lg transition-all flex items-center gap-2"
          >
            {savingGemini ? (
              <>
                <Loader2 size={18} className="animate-spin" />
                Saving...
              </>
            ) : (
              <>
                Save & Continue <ChevronRight size={18} />
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );

  // Step 5: Complete Setup
  const renderComplete = () => (
    <div className="text-center py-8">
      <div className="flex justify-center mb-6">
        <div className="p-6 bg-gradient-to-br from-green-500/20 to-green-500/5 rounded-full">
          <Check size={64} className="text-green-500" />
        </div>
      </div>

      <h2 className="text-3xl font-bold text-white mb-4">
        You're All Set!
      </h2>

      <p className="text-lg text-text-secondary max-w-lg mx-auto mb-8 leading-relaxed">
        {scanFolders.length > 0
          ? `Ready to scan ${scanFolders.length} folder${scanFolders.length > 1 ? 's' : ''} and build your music library.`
          : 'You can add music folders anytime from Settings.'}
      </p>

      <div className="bg-surface-1 border border-surface-border rounded-xl p-6 max-w-md mx-auto mb-8">
        <h3 className="font-bold text-white mb-4">What's Next?</h3>
        <div className="space-y-3 text-left">
          {scanFolders.length > 0 && (
            <div className="flex items-start gap-3">
              <div className="p-2 bg-brand/10 rounded-lg flex-shrink-0">
                <FolderOpen size={18} className="text-brand" />
              </div>
              <div>
                <p className="text-sm font-medium text-white">Scan Your Music</p>
                <p className="text-xs text-text-subtle">
                  Click "Start Scanning" to import your music files
                </p>
              </div>
            </div>
          )}
          <div className="flex items-start gap-3">
            <div className="p-2 bg-brand/10 rounded-lg flex-shrink-0">
              <Sparkles size={18} className="text-brand" />
            </div>
            <div>
              <p className="text-sm font-medium text-white">Explore Features</p>
              <p className="text-xs text-text-subtle">
                Create playlists, use the equalizer, and discover smart mixes
              </p>
            </div>
          </div>
          <div className="flex items-start gap-3">
            <div className="p-2 bg-brand/10 rounded-lg flex-shrink-0">
              <Music size={18} className="text-brand" />
            </div>
            <div>
              <p className="text-sm font-medium text-white">Enjoy Your Music</p>
              <p className="text-xs text-text-subtle">
                High-quality playback with crossfade and gapless support
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="flex flex-col items-center gap-4">
        {scanFolders.length > 0 ? (
          <button
            onClick={handleStartScanAndClose}
            className="bg-brand hover:bg-brand-hover text-black font-bold py-4 px-10 rounded-full transition-all shadow-lg shadow-brand/20 hover:shadow-brand/40 hover:scale-105 flex items-center gap-3 text-lg"
          >
            <Music size={24} />
            Start Scanning & Launch
          </button>
        ) : (
          <button
            onClick={handleFinish}
            className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-8 rounded-full transition-all shadow-lg shadow-brand/20 flex items-center gap-2"
          >
            <Music size={20} />
            Start Using ViiB MediaHub
          </button>
        )}
        
        {scanFolders.length > 0 && (
          <p className="text-sm text-text-subtle">
            Scanning will continue in the background while you use the app.
          </p>
        )}
      </div>
    </div>
  );

  return (
    <>
      <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/90 backdrop-blur-sm animate-in fade-in duration-300">
        <div className="bg-surface-2 border border-surface-3 rounded-2xl p-8 max-w-3xl w-full mx-4 shadow-2xl max-h-[90vh] overflow-y-auto">
          {/* Progress Indicator */}
          {step > 1 && step < 5 && (
            <div className="mb-8">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-bold text-text-subtle uppercase">Setup Progress</span>
                <span className="text-xs text-text-subtle">Step {step - 1} of 4</span>
              </div>
              <div className="h-2 bg-surface-1 rounded-full overflow-hidden">
                <div
                  className="h-full bg-brand transition-all duration-500 ease-out rounded-full"
                  style={{ width: `${((step - 1) / 4) * 100}%` }}
                />
              </div>
            </div>
          )}

          {/* Step Content */}
          {step === 1 && renderWelcome()}
          {step === 2 && renderFolderSetup()}
          {step === 3 && renderSpotifySetup()}
          {step === 4 && renderGeminiSetup()}
          {step === 5 && renderComplete()}
        </div>
      </div>

      {/* Folder Browser Modal */}
      {showFolderBrowser && (
        <div className="fixed inset-0 z-[300] flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-2xl w-full mx-4 shadow-2xl max-h-[80vh] flex flex-col">
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
        <div className="fixed inset-0 z-[300] flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-2xl w-full mx-4 shadow-2xl max-h-[80vh] flex flex-col">
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
    </>
  );
};

export default FirstLaunchDialog;
