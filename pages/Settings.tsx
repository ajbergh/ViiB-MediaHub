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
 *   - AI Provider: Configure LLM provider (Gemini, OpenAI, Anthropic, Ollama, X.AI)
 *   - Last.FM Integration: Community-sourced metadata enrichment (added 2025-12-31)
 *   - Genre Enrichment: Uses configured AI to populate genre metadata
 *   - Unified Enrichment: Full metadata enrichment (genres, mood, energy, tempo, BPM, year)
 * - Activity Log: Debug log viewer
 * 
 * Last.FM Features (added 2025-12-31):
 * - API key and shared secret configuration
 * - Connection testing
 * - Optional scrobbling with username/password authentication
 * - Last.FM-based enrichment trigger
 * 
 * Folder browser dialogs allow navigation and selection of:
 * - Music scan directories
 * - Spotify download destination
 * 
 * AI Features (requires configured AI provider):
 * - All AI features use the configured LLM provider (AI DJ Provider section)
 * - Genre enrichment runs during library scans or can be triggered manually
 * - Mood analysis detects emotional characteristics without audio processing
 * - Results are stored in the songs table for AI DJ playlist generation
 * 
 * @module Settings
 */

import React, { useState, useRef, useEffect } from 'react';
import { useLocation } from 'react-router';
import { Wifi, Volume2, HardDrive, Trash2, Terminal, XCircle, SlidersHorizontal, Activity, Layers, Sparkles, FolderOpen, Loader2, AlertTriangle, Plus, X, RefreshCw, Server, MonitorOff, BarChart3, Zap, Music, Headphones, Speaker, Copy, ShieldCheck } from 'lucide-react';
import { useStore } from '../store';
import { HomeLayoutVariant, VisualizerMode, Song } from '../types';
import { parseSong } from '../metadata';
import { api } from '../services/api';
import { Button } from '../components/ui/Button';
import { Page } from '../components/ui/Page';
import { TextInput } from '../components/ui/TextInput';
import { SPOTIFY_DESKTOP_CALLBACK_URL } from '../utils';
import { LibraryOperationsPanel } from './LibraryOperations';

const HOME_LAYOUT_OPTIONS: Array<{
  value: HomeLayoutVariant;
  label: string;
  description: string;
}> = [
  {
    value: 'shelves',
    label: 'Music Shelves',
    description: 'Spotlight feature with album, artist, and mix shelves.',
  },
  {
    value: 'coverWall',
    label: 'Cover Wall',
    description: 'Large album-art collage hero with shelves below.',
  },
  {
    value: 'dashboard',
    label: 'Compact Dashboard',
    description: 'Dense personalized rows for faster browsing.',
  },
];

/**
 * AudioOutputSettings - DJ Mode audio output device configuration
 * 
 * Allows the user to select separate audio output devices for:
 * - Main/Live output (speakers/PA system)
 * - Headphone/Cue output (DJ headphones for previewing tracks)
 * 
 * Uses the Web Audio API's setSinkId for device routing.
 */
const AudioOutputSettings: React.FC = () => {
    const { audioSettings, setMainOutputDevice, setHeadphoneOutputDevice } = useStore();
    const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [permissionDenied, setPermissionDenied] = useState(false);

    // Enumerate available audio output devices
    useEffect(() => {
        const loadDevices = async () => {
            try {
                setIsLoading(true);
                // Request permission to enumerate devices (requires user gesture in some browsers)
                await navigator.mediaDevices.getUserMedia({ audio: true }).then(stream => {
                    // Stop the stream immediately - we just needed permission
                    stream.getTracks().forEach(track => track.stop());
                }).catch(() => {
                    // Permission denied or not available - still try to enumerate
                    console.warn('Could not get audio permission, device list may be limited');
                });

                const devices = await navigator.mediaDevices.enumerateDevices();
                const outputDevices = devices.filter(d => d.kind === 'audiooutput');
                setAudioDevices(outputDevices);
                setPermissionDenied(false);
            } catch (error) {
                console.error('Failed to enumerate audio devices:', error);
                setPermissionDenied(true);
            } finally {
                setIsLoading(false);
            }
        };

        loadDevices();

        // Listen for device changes (USB devices connected/disconnected)
        const handleDeviceChange = () => {
            loadDevices();
        };
        navigator.mediaDevices.addEventListener('devicechange', handleDeviceChange);
        return () => {
            navigator.mediaDevices.removeEventListener('devicechange', handleDeviceChange);
        };
    }, []);

    // Check if selected device is still available
    const isDeviceAvailable = (deviceId: string) => {
        if (!deviceId) return true; // Default device always available
        return audioDevices.some(d => d.deviceId === deviceId);
    };

    const getDeviceLabel = (deviceId: string) => {
        if (!deviceId) return 'System Default';
        const device = audioDevices.find(d => d.deviceId === deviceId);
        return device?.label || 'Unknown Device';
    };

    if (isLoading) {
        return (
            <div className="pt-4 mt-4 border-t border-surface-hover">
                <div className="flex items-center gap-3 text-text-subtle">
                    <Loader2 className="w-4 h-4 animate-spin" />
                    <span>Loading audio devices...</span>
                </div>
            </div>
        );
    }

    if (permissionDenied) {
        return (
            <div className="pt-4 mt-4 border-t border-surface-hover">
                <div className="flex items-center gap-3 text-amber-400">
                    <AlertTriangle className="w-4 h-4" />
                    <span className="text-sm">Audio device permission required for DJ output routing</span>
                </div>
            </div>
        );
    }

    return (
        <div className="pt-4 mt-4 border-t border-surface-hover space-y-4">
            <div className="flex items-center gap-2 text-brand">
                <Headphones size={16} />
                <h3 className="font-medium text-text-main">DJ Mode Audio Output</h3>
            </div>
            <p className="text-sm text-text-subtle">
                Configure separate audio outputs for DJ Mode. Main output goes to your speakers/PA, 
                headphone output allows previewing (cueing) tracks before mixing them live.
            </p>

            {audioDevices.length === 0 ? (
                <p className="text-sm text-amber-400">No audio output devices found</p>
            ) : (
                <div className="space-y-4">
                    {/* Main Output Device */}
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                            <div className="p-2 bg-surface-hover rounded-lg text-text-main">
                                <Speaker size={18} />
                            </div>
                            <div>
                                <h4 className="font-medium text-text-main">Main Output (Live)</h4>
                                <p className="text-xs text-text-subtle">Speakers or PA system for the audience</p>
                            </div>
                        </div>
                        <select
                            value={audioSettings.mainOutputDevice || ''}
                            onChange={(e) => setMainOutputDevice(e.target.value)}
                            className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none max-w-[250px]"
                        >
                            <option value="">System Default</option>
                            {audioDevices.map(device => (
                                <option key={device.deviceId} value={device.deviceId}>
                                    {device.label || `Device ${device.deviceId.slice(0, 8)}...`}
                                </option>
                            ))}
                        </select>
                    </div>

                    {/* Headphone Output Device */}
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                            <div className="p-2 bg-surface-hover rounded-lg text-text-main">
                                <Headphones size={18} />
                            </div>
                            <div>
                                <h4 className="font-medium text-text-main">Headphone Output (Cue)</h4>
                                <p className="text-xs text-text-subtle">Preview next track while mixing</p>
                            </div>
                        </div>
                        <select
                            value={audioSettings.headphoneOutputDevice || ''}
                            onChange={(e) => setHeadphoneOutputDevice(e.target.value)}
                            className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none max-w-[250px]"
                        >
                            <option value="">System Default</option>
                            {audioDevices.map(device => (
                                <option key={device.deviceId} value={device.deviceId}>
                                    {device.label || `Device ${device.deviceId.slice(0, 8)}...`}
                                </option>
                            ))}
                        </select>
                    </div>

                    {/* Warning if devices are the same */}
                    {audioSettings.mainOutputDevice === audioSettings.headphoneOutputDevice && (
                        <div className="flex items-center gap-2 text-amber-400 text-xs bg-amber-400/10 p-2 rounded">
                            <AlertTriangle size={14} />
                            <span>
                                Main and headphone outputs are the same device. 
                                For proper cueing, select different devices.
                            </span>
                        </div>
                    )}

                    {/* Warning if selected device no longer available */}
                    {audioSettings.mainOutputDevice && !isDeviceAvailable(audioSettings.mainOutputDevice) && (
                        <div className="flex items-center gap-2 text-red-400 text-xs bg-red-400/10 p-2 rounded">
                            <AlertTriangle size={14} />
                            <span>Main output device no longer available. Please select a new device.</span>
                        </div>
                    )}
                    {audioSettings.headphoneOutputDevice && !isDeviceAvailable(audioSettings.headphoneOutputDevice) && (
                        <div className="flex items-center gap-2 text-red-400 text-xs bg-red-400/10 p-2 rounded">
                            <AlertTriangle size={14} />
                            <span>Headphone output device no longer available. Please select a new device.</span>
                        </div>
                    )}

                    <p className="text-xs text-text-subtle italic">
                        💡 Tip: Connect a USB audio interface with multiple outputs for professional DJ setups.
                        Changes take effect in DJ Mode when audio is next loaded.
                    </p>
                </div>
            )}
        </div>
    );
};

export const Settings: React.FC = () => {
  const { 
      audioSettings, setCrossfade, setGapless, setNormalization,
      setMainOutputDevice, setHeadphoneOutputDevice,
      setVisualizerMode, setVisualizerArtworkOpacity, 
      setVisualizerFullscreenEnabled, setVisualizerFullscreenOpacity,
      setEqEnabled, toggleEqPanel,
      // Milkdrop settings
      milkdropSettings, setMilkdropSettings, milkdropPresetKeys,
      showSmartMixes, setShowSmartMixes,
      homeLayoutVariant, setHomeLayoutVariant,
      spotifyClientId, spotifyClientSecret, setSpotifyCredentials,
      streamingEnabled, streamingQuality, setStreamingEnabled, setStreamingQuality,
      preferLocalPlayback, setPreferLocalPlayback,
      streamingStats, resetStreamingStats,
      logs, clearLogs, addLog, addSongs, resetLibrary,
      isScanning, scanProgress, setScanning, setScanProgress,
      backendAvailable, scanFolders, loadScanFolders, addScanFolder, removeScanFolder, startBackendScan, startQuickScan,
      setEnrichmentStatus, refreshLibrary
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
  
  // Enrichment State
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

  const updateGlobalEnrichmentStatus = (progress: import('../services/api').EnrichmentProgress, fallbackMessage: string) => {
    const isFinished = progress.status === 'complete' || progress.status === 'error';
    setEnrichmentStatus({
      isEnriching: !isFinished,
      totalSongs: progress.totalSongs || 0,
      processedSongs: progress.processedSongs || 0,
      currentBatch: progress.currentBatch || 0,
      totalBatches: progress.totalBatches || 0,
      message: progress.status === 'error'
        ? `Error: ${progress.error || progress.message || fallbackMessage}`
        : progress.message || fallbackMessage,
    });

    const details = {
      processedSongs: progress.processedSongs || 0,
      totalSongs: progress.totalSongs || 0,
      currentBatch: progress.currentBatch || 0,
      totalBatches: progress.totalBatches || 0,
    };
    if (progress.status === 'started') {
      addLog('info', `[AI Enrichment] ${progress.message || fallbackMessage}`, details);
    } else if (progress.status === 'batch_complete') {
      addLog('info', `[AI Enrichment] ${progress.message || fallbackMessage}`, details);
    } else if (progress.status === 'complete') {
      addLog('success', `[AI Enrichment] ${progress.message || 'Completed'}`, details);
    } else if (progress.status === 'error') {
      addLog('error', `[AI Enrichment] ${progress.error || progress.message || 'Failed'}`, details);
    }
  };

  // Genre Normalization State
  const [isNormalizingGenres, setIsNormalizingGenres] = useState(false);
  const [normalizeGenresStatus, setNormalizeGenresStatus] = useState('');

  // LLM Provider Settings State
  const [llmProvider, setLlmProvider] = useState('ollama');
  const [llmModel, setLlmModel] = useState('llama3.2:8b');
  const [llmApiKey, setLlmApiKey] = useState('');
  const [llmBaseURL, setLlmBaseURL] = useState('http://localhost:11434');
  const [llmProviders, setLlmProviders] = useState<import('../services/api').LLMProviderInfo[]>([]);
  const [llmModels, setLlmModels] = useState<Record<string, import('../services/api').LLMModelInfo[]>>({});
  const [llmLoading, setLlmLoading] = useState(false);
  const [llmSaveStatus, setLlmSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [llmTestStatus, setLlmTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
  const [llmTestMessage, setLlmTestMessage] = useState('');

  // Semantic retrieval settings stay separate from the chat provider because
  // changing a chat model must never silently change the vector space.
  const [semanticProvider, setSemanticProvider] = useState<'auto' | 'ollama' | 'openai' | 'openrouter' | 'gemini' | 'disabled'>('auto');
  const [semanticModel, setSemanticModel] = useState('');
  const [semanticBaseURL, setSemanticBaseURL] = useState('http://localhost:11434');
  const [semanticDimensions, setSemanticDimensions] = useState(0);
  const [semanticAPIKey, setSemanticAPIKey] = useState('');
  const [semanticCloudCost, setSemanticCloudCost] = useState<import('../services/api').SemanticCloudCost | null>(null);
  const [semanticCloudConfirmed, setSemanticCloudConfirmed] = useState(false);
  const [semanticStatus, setSemanticStatus] = useState<import('../services/api').SemanticStatus | null>(null);
  const [semanticLoading, setSemanticLoading] = useState(false);
  const [semanticSaveStatus, setSemanticSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [semanticActionStatus, setSemanticActionStatus] = useState<'idle' | 'working' | 'success' | 'error'>('idle');
  const [semanticActionMessage, setSemanticActionMessage] = useState('');

  const [browserEntries, setBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingBrowser, setLoadingBrowser] = useState(false);

  // Last.FM Settings State
  const [lastfmApiKey, setLastfmApiKey] = useState('');
  const [lastfmSecret, setLastfmSecret] = useState('');
  const [lastfmEnabled, setLastfmEnabled] = useState(false);
  const [lastfmUsername, setLastfmUsername] = useState('');
  const [lastfmPassword, setLastfmPassword] = useState('');
  const [lastfmConfigured, setLastfmConfigured] = useState(false);
  const [lastfmConnected, setLastfmConnected] = useState(false);
  const [lastfmCanScrobble, setLastfmCanScrobble] = useState(false);
  const [lastfmLoading, setLastfmLoading] = useState(false);
  const [lastfmSaveStatus, setLastfmSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [lastfmTestStatus, setLastfmTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
  const [lastfmTestMessage, setLastfmTestMessage] = useState('');
  const [lastfmEnrichStatus, setLastfmEnrichStatus] = useState('');
  const [lastfmEnrichProgress, setLastfmEnrichProgress] = useState<{ queued: number } | null>(null);
  const [isLastfmEnriching, setIsLastfmEnriching] = useState(false);
  
  // Enrichment Source Selection - controls whether to use AI or Last.FM for metadata
  const [enrichmentSource, setEnrichmentSource] = useState<'ai' | 'lastfm' | 'hybrid'>('ai');

  // Load LLM Settings
  useEffect(() => {
      const loadLLMSettings = async () => {
          try {
              setLlmLoading(true);
              const settings = await api.getLLMSettings();
              setLlmProvider(settings.provider || 'ollama');
              setLlmModel(settings.model || 'llama3.2:8b');
              setLlmApiKey(settings.apiKey || '');
              setLlmBaseURL(settings.baseURL || (settings.provider === 'ollama' ? 'http://localhost:11434' : ''));
              setLlmProviders(settings.providers || []);
              setLlmModels(settings.models || {});
          } catch (e) {
              console.error('Failed to load LLM settings:', e);
          } finally {
              setLlmLoading(false);
          }
      };
      if (backendAvailable) {
          loadLLMSettings();
      }
  }, [backendAvailable]);

  const refreshSemanticStatus = async () => {
      const status = await api.getSemanticStatus();
      setSemanticStatus(status);
  };

  // Load semantic configuration and then keep the non-blocking background
  // indexer's visible status current while this page is open.
  useEffect(() => {
      if (!backendAvailable) return;
      let active = true;
      const loadSemanticSettings = async () => {
          try {
              setSemanticLoading(true);
              const [settings, status] = await Promise.all([
                  api.getSemanticSettings(),
                  api.getSemanticStatus(),
              ]);
              if (!active) return;
              setSemanticProvider(settings.provider);
              setSemanticModel(settings.model || '');
              setSemanticBaseURL(settings.baseURL || 'http://localhost:11434');
              setSemanticDimensions(settings.dimensions || (settings.provider === 'gemini' ? 768 : settings.provider === 'openai' || settings.provider === 'openrouter' ? 512 : 0));
              setSemanticAPIKey('');
              setSemanticCloudCost(settings.cloudCost || null);
              setSemanticCloudConfirmed(false);
              setSemanticStatus(status);
          } catch (e) {
              console.error('Failed to load semantic index settings:', e);
          } finally {
              if (active) setSemanticLoading(false);
          }
      };
      void loadSemanticSettings();
      const timer = window.setInterval(() => {
          void refreshSemanticStatus().catch((e) => console.error('Failed to refresh semantic index status:', e));
      }, 4000);
      return () => {
          active = false;
          window.clearInterval(timer);
      };
  }, [backendAvailable]);

  // Load Last.FM Settings
  useEffect(() => {
      const loadLastFMSettings = async () => {
          try {
              setLastfmLoading(true);
              const settings = await api.getLastFMSettings();
              setLastfmConfigured(settings.apiKey !== '');
              setLastfmEnabled(settings.enabled);
              setLastfmUsername(settings.username || '');
              // Load enrichment source preference (default to 'ai' if not set)
              setEnrichmentSource(settings.enrichmentSource || 'ai');
              // Don't load masked API key into input
              if (settings.apiKey && !settings.apiKey.includes('...')) {
                  setLastfmApiKey(settings.apiKey);
              }
              
              // Get status for connection info
              try {
                  const status = await api.getLastFMStatus();
                  setLastfmConnected(status.connected);
                  setLastfmCanScrobble(status.canScrobble);
              } catch (e) {
                  console.error('Failed to get Last.FM status:', e);
              }
          } catch (e) {
              console.error('Failed to load Last.FM settings:', e);
          } finally {
              setLastfmLoading(false);
          }
      };
      if (backendAvailable) {
          loadLastFMSettings();
      }
  }, [backendAvailable]);

  // Download folder browser state
  const [showDownloadFolderBrowser, setShowDownloadFolderBrowser] = useState(false);
  const [downloadBrowserPath, setDownloadBrowserPath] = useState('');
  const [downloadBrowserEntries, setDownloadBrowserEntries] = useState<{ name: string; path: string; isDir: boolean }[]>([]);
  const [loadingDownloadBrowser, setLoadingDownloadBrowser] = useState(false);

  // Spotify download location
  const [spotifyDownloadPath, setSpotifyDownloadPath] = useState('');
  const [downloadPathSaved, setDownloadPathSaved] = useState(false);

  // Settings Tab State
  type SettingsTab = 'library' | 'health' | 'audio' | 'integrations' | 'ai' | 'appearance' | 'system';
  const location = useLocation();
  const initialTab = (location.state as { tab?: SettingsTab } | null)?.tab || 'library';
  const [activeTab, setActiveTab] = useState<SettingsTab>(initialTab);

  useEffect(() => {
    const tabFromState = (location.state as { tab?: SettingsTab } | null)?.tab;
    if (tabFromState) {
      setActiveTab(tabFromState);
    }
  }, [location.state]);

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

  const handleNormalizeGenres = async () => {
      setIsNormalizingGenres(true);
      setNormalizeGenresStatus('Normalizing genre capitalization...');
      try {
          const result = await api.normalizeGenres();
          setNormalizeGenresStatus(`✓ Normalized ${result.normalized} songs${result.errors > 0 ? `, ${result.errors} errors` : ''}`);
          addLog('info', `Genre normalization complete: ${result.normalized} songs normalized`);
      } catch (e) {
          console.error("Genre normalization failed", e);
          setNormalizeGenresStatus('✗ Normalization failed');
          addLog('error', 'Genre normalization failed', e);
      } finally {
          setIsNormalizingGenres(false);
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
        alert(`Import complete! Added ${total} songs.`);

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
        <Page className="max-w-[1400px] w-full mx-auto px-4 sm:px-6 lg:px-8">
            <h1 className="text-display mb-6">Settings</h1>

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

      {/* Category Tabs Header */}
      <div className="flex items-center gap-2 mb-8 overflow-x-auto pb-2 border-b border-surface-border scrollbar-none">
        <button
          type="button"
          onClick={() => setActiveTab('library')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'library'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <FolderOpen size={16} />
          Library & Storage
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('health')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'health'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <ShieldCheck size={16} />
          Library Health
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('audio')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'audio'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <Volume2 size={16} />
          Audio & Playback
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('integrations')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'integrations'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <Wifi size={16} />
          Integrations & Spotify
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('ai')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'ai'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <Sparkles size={16} />
          AI & Intelligence
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('appearance')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'appearance'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <SlidersHorizontal size={16} />
          Appearance
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('system')}
          className={`flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold transition-all whitespace-nowrap ${
            activeTab === 'system'
              ? 'bg-brand text-black shadow-md shadow-brand/20'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-3'
          }`}
        >
          <Terminal size={16} />
          System & Logs
        </button>
      </div>

      {/* TAB 1: LIBRARY & STORAGE */}
      {activeTab === 'library' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          {/* Library Management */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-6 text-brand">
                <FolderOpen size={20} />
                <h2 className="text-card font-semibold text-text-main">Library Folders & Scanning</h2>
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
                        Import music from your local folders. Supported formats: MP3, OGG, FLAC.
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

          {/* Storage & Downloads */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
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

            {/* Genre Normalization */}
            <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg">
                <div>
                    <h3 className="text-sm text-text-secondary mb-1 font-bold">Genre Normalization</h3>
                    <p className="text-xs text-text-subtle">
                        {normalizeGenresStatus || 'Fix inconsistent genre capitalization (e.g., "acid jazz" → "Acid Jazz").'}
                    </p>
                </div>
                <Button
                    variant="secondary"
                    onClick={handleNormalizeGenres}
                    disabled={isNormalizingGenres}
                    leftIcon={isNormalizingGenres ? <Loader2 size={16} className="animate-spin" /> : <SlidersHorizontal size={16} />}
                    className="px-4 py-2 font-bold text-sm"
                >
                    {isNormalizingGenres ? 'Normalizing...' : 'Normalize Genres'}
                </Button>
            </div>
          </section>

          {/* Danger Zone */}
          <section className="rounded-xl border border-error/30 bg-error/5 p-6">
            <div className="flex items-center gap-2 mb-3">
              <AlertTriangle size={18} className="text-error flex-shrink-0" />
              <h3 className="text-sm font-bold text-error uppercase tracking-wider">Danger Zone</h3>
            </div>
            <div className="flex items-center justify-between">
              <div>
                  <h4 className="text-sm font-semibold text-text-main mb-1">Database &amp; Cache</h4>
                  <p className="text-xs text-text-subtle">Clear local database to resolve indexing issues or reset app state.</p>
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
          </section>
        </div>
      )}

      {/* TAB: LIBRARY HEALTH & OPERATIONS */}
      {activeTab === 'health' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-6 text-brand">
              <ShieldCheck size={20} />
              <h2 className="text-lg font-bold text-text-main">Library Health & Operations</h2>
            </div>
            <LibraryOperationsPanel />
          </section>
        </div>
      )}

      {/* TAB 2: AUDIO & PLAYBACK */}
      {activeTab === 'audio' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
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
                        <option value="FLAME_SPECTRUM">Flame Spectrum Crown</option>
                        <option value="STARDUST_HALO">Stardust Pulse Halo</option>
                        <option value="AURORA_RIBBON">Aurora Ribbon</option>
                        <option value="ELECTRIC_ARC">Electric Arc Wireframe</option>
                        <option value="GRASS_OSCILLOSCOPE">Growing Grass Oscilloscope</option>
                        <option value="FIREFLY_FIELD">Holiday Firefly Field</option>
                        <option value="TUNNEL_WAVEFORM">3D Tunnel Waveform</option>
                        <option value="WIND_FIELD">Soft Wind Field</option>
                        <option value="MILKDROP">Milkdrop (WebGL)</option>
                    </select>
                </div>

                {/* Milkdrop Settings - Only show when Milkdrop is selected */}
                {audioSettings.visualizerMode === 'MILKDROP' && (
                    <div className="pl-4 ml-4 border-l-2 border-surface-hover space-y-4">
                        {/* Current Preset Display */}
                        <div className="flex items-center justify-between">
                            <div>
                                <h3 className="font-medium text-text-main">Current Preset</h3>
                                <p className="text-sm text-text-subtle">
                                    {milkdropSettings.currentPreset 
                                        ? milkdropSettings.currentPreset.replace(/_/g, ' ').replace(/\.milk$/i, '')
                                        : 'Random (auto-selected)'
                                    }
                                </p>
                            </div>
                            <span className="text-xs text-text-subtle">
                                {milkdropPresetKeys.length} presets available
                            </span>
                        </div>

                        {/* Auto-Cycle Toggle */}
                        <div className="flex items-center justify-between">
                            <div>
                                <h3 className="font-medium text-text-main">Auto-Cycle Presets</h3>
                                <p className="text-sm text-text-subtle">Automatically switch presets over time</p>
                            </div>
                            <div 
                                className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${milkdropSettings.presetCycleEnabled ? 'bg-brand' : 'bg-surface-border'}`}
                                onClick={() => setMilkdropSettings({ presetCycleEnabled: !milkdropSettings.presetCycleEnabled })}
                            >
                                <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${milkdropSettings.presetCycleEnabled ? 'right-1' : 'left-1'}`}></div>
                            </div>
                        </div>

                        {/* Cycle Interval */}
                        {milkdropSettings.presetCycleEnabled && (
                            <div className="flex items-center justify-between">
                                <div>
                                    <h3 className="font-medium text-text-main">Cycle Interval</h3>
                                    <p className="text-sm text-text-subtle">Seconds between preset changes</p>
                                </div>
                                <select
                                    value={milkdropSettings.presetCycleInterval}
                                    onChange={(e) => setMilkdropSettings({ presetCycleInterval: parseInt(e.target.value) })}
                                    className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
                                >
                                    <option value={15}>15 seconds</option>
                                    <option value={30}>30 seconds</option>
                                    <option value={45}>45 seconds</option>
                                    <option value={60}>1 minute</option>
                                    <option value={90}>90 seconds</option>
                                    <option value={120}>2 minutes</option>
                                </select>
                            </div>
                        )}

                        {/* Blend Duration */}
                        <div className="flex items-center justify-between">
                            <div>
                                <h3 className="font-medium text-text-main">Blend Duration</h3>
                                <p className="text-sm text-text-subtle">Transition time between presets</p>
                            </div>
                            <select
                                value={milkdropSettings.blendDuration}
                                onChange={(e) => setMilkdropSettings({ blendDuration: parseFloat(e.target.value) })}
                                className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
                            >
                                <option value={0}>Instant</option>
                                <option value={1}>1 second</option>
                                <option value={2}>2 seconds</option>
                                <option value={2.7}>2.7s (classic)</option>
                                <option value={4}>4 seconds</option>
                                <option value={5}>5 seconds</option>
                            </select>
                        </div>

                        {/* Quality Setting */}
                        <div className="flex items-center justify-between">
                            <div>
                                <h3 className="font-medium text-text-main">Quality</h3>
                                <p className="text-sm text-text-subtle">Rendering resolution (higher = more GPU usage)</p>
                            </div>
                            <select
                                value={milkdropSettings.quality}
                                onChange={(e) => setMilkdropSettings({ quality: e.target.value as 'low' | 'medium' | 'high' })}
                                className="bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
                            >
                                <option value="low">Low (640×480)</option>
                                <option value="medium">Medium (720p)</option>
                                <option value="high">High (1080p)</option>
                            </select>
                        </div>

                        {/* Favorites Count */}
                        {milkdropSettings.favoritePresets.length > 0 && (
                            <div className="text-xs text-text-subtle">
                                ★ {milkdropSettings.favoritePresets.length} favorite preset{milkdropSettings.favoritePresets.length !== 1 ? 's' : ''}
                            </div>
                        )}
                    </div>
                )}

                {/* Visualizer Overlay Settings - Show when visualizer is not OFF */}
                {audioSettings.visualizerMode !== 'OFF' && (
                    <div className="space-y-4 pl-4 ml-4 border-l-2 border-surface-hover">
                        {/* Album Art Opacity */}
                        <div>
                            <div className="flex items-center justify-between mb-2">
                                <div>
                                    <h3 className="font-medium text-text-main">Album Art Opacity</h3>
                                    <p className="text-sm text-text-subtle">Visibility of album artwork behind visualizer</p>
                                </div>
                                <span className="font-mono text-sm text-brand">{audioSettings.visualizerArtworkOpacity}%</span>
                            </div>
                            <input 
                                type="range" 
                                min="0" 
                                max="100" 
                                step="5"
                                value={audioSettings.visualizerArtworkOpacity}
                                onChange={(e) => setVisualizerArtworkOpacity(parseInt(e.target.value))}
                                className="w-full accent-brand"
                            />
                        </div>

                        {/* Fullscreen Background Visualizer Toggle */}
                        <div className="flex items-center justify-between">
                            <div>
                                <h3 className="font-medium text-text-main">Fullscreen Background</h3>
                                <p className="text-sm text-text-subtle">Show visualizer across entire Now Playing screen</p>
                            </div>
                            <div 
                                className={`w-12 h-6 rounded-full relative cursor-pointer transition-colors ${audioSettings.visualizerFullscreenEnabled ? 'bg-brand' : 'bg-surface-border'}`}
                                onClick={() => setVisualizerFullscreenEnabled(!audioSettings.visualizerFullscreenEnabled)}
                            >
                                <div className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-all ${audioSettings.visualizerFullscreenEnabled ? 'right-1' : 'left-1'}`}></div>
                            </div>
                        </div>

                        {/* Fullscreen Opacity - Only show when fullscreen is enabled */}
                        {audioSettings.visualizerFullscreenEnabled && (
                            <div>
                                <div className="flex items-center justify-between mb-2">
                                    <div>
                                        <h3 className="font-medium text-text-main">Background Opacity</h3>
                                        <p className="text-sm text-text-subtle">Intensity of fullscreen visualizer</p>
                                    </div>
                                    <span className="font-mono text-sm text-brand">{audioSettings.visualizerFullscreenOpacity}%</span>
                                </div>
                                <input 
                                    type="range" 
                                    min="5" 
                                    max="50" 
                                    step="5"
                                    value={audioSettings.visualizerFullscreenOpacity}
                                    onChange={(e) => setVisualizerFullscreenOpacity(parseInt(e.target.value))}
                                    className="w-full accent-brand"
                                />
                            </div>
                        )}
                    </div>
                )}

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

                {/* DJ Audio Output Devices */}
                <AudioOutputSettings />
            </div>
          </section>
        </div>
      )}

      {/* TAB 3: INTEGRATIONS & SPOTIFY */}
      {activeTab === 'integrations' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          {/* Spotify Integration */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-4 text-brand">
                <Wifi size={20} />
                <h2 className="text-lg font-bold text-text-main">Spotify Developer Application</h2>
            </div>
            
            <div className="space-y-4">
                <p className="text-sm text-text-secondary">
                    Create your own Spotify Developer app, add the callback below in its settings, then paste its credentials here.
                </p>
                <div className="bg-surface-1 border border-surface-border rounded-lg p-4 text-sm text-text-secondary space-y-2">
                    <p><span className="font-bold text-text-main">1.</span> Create an app in the <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-brand hover:underline">Spotify Developer Dashboard</a>.</p>
                    <p><span className="font-bold text-text-main">2.</span> Add this exact Redirect URI in the app's settings:</p>
                    <div className="flex items-center gap-2">
                        <code className="flex-1 bg-surface-2 px-3 py-2 rounded text-xs text-text-main break-all select-all">{SPOTIFY_DESKTOP_CALLBACK_URL}</code>
                        <button
                            onClick={() => navigator.clipboard.writeText(SPOTIFY_DESKTOP_CALLBACK_URL)}
                            className="p-2 rounded hover:bg-surface-2 text-text-secondary hover:text-text-main transition-colors"
                            title="Copy Spotify redirect URI"
                            aria-label="Copy Spotify redirect URI"
                        >
                            <Copy size={14} />
                        </button>
                    </div>
                    <p><span className="font-bold text-text-main">3.</span> Copy the Client ID and Client Secret from that Spotify app into the fields below.</p>
                </div>
                
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
                    <p className="text-xs text-text-subtle">Save both values, then return to the Spotify page and select Connect Spotify.</p>
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

          {/* Spotify Direct Streaming */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-6 text-brand">
                <Music size={20} />
                <h2 className="text-lg font-bold text-text-main">Spotify Direct Streaming</h2>
            </div>

            <div className="space-y-4">
                <p className="text-xs text-text-subtle mb-3">
                    Stream Spotify tracks directly without downloading first. Requires Spotify Premium.
                </p>
                
                {/* Enable/Disable Streaming Toggle */}
                <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg">
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
                <div className="flex items-center justify-between bg-surface-1 p-4 rounded-lg">
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
                <div className="bg-surface-1 p-4 rounded-lg">
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
                        <div className="bg-surface-2 p-3 rounded-lg">
                            <div className="text-lg font-bold text-brand">{streamingStats.totalStreams}</div>
                            <div className="text-xs text-text-subtle">Total Streams</div>
                        </div>
                        
                        <div className="bg-surface-2 p-3 rounded-lg">
                            <div className="text-lg font-bold text-success">
                                {streamingStats.totalStreams > 0 
                                    ? Math.round((streamingStats.successfulStreams / streamingStats.totalStreams) * 100)
                                    : 100}%
                            </div>
                            <div className="text-xs text-text-subtle">Success Rate</div>
                        </div>
                        
                        <div className="bg-surface-2 p-3 rounded-lg">
                            <div className="text-lg font-bold text-warning">{streamingStats.bufferingEvents}</div>
                            <div className="text-xs text-text-subtle">Buffer Events</div>
                        </div>
                        
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
          </section>

          {/* Last.FM Integration */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-2">
              <Music size={20} className="text-[#d51007]" />
              <h2 className="text-lg font-bold text-text-main">Last.FM Integration</h2>
            </div>
            <p className="text-text-subtle text-sm mb-4">
              Use Last.FM's community-sourced tags and metadata as an alternative to AI enrichment.
              Free API with no usage costs. Can also enable scrobbling to track your listening history.
            </p>

            {lastfmLoading ? (
              <div className="flex items-center gap-2 text-text-subtle">
                <Loader2 size={16} className="animate-spin" />
                Loading Last.FM settings...
              </div>
            ) : (
              <div className="flex flex-col gap-4">
                {/* Connection Status */}
                {lastfmConfigured && (
                  <div className={`flex items-center gap-2 text-sm ${lastfmConnected ? 'text-success' : 'text-text-subtle'}`}>
                    <span className={`w-2 h-2 rounded-full ${lastfmConnected ? 'bg-success' : 'bg-text-subtle'} inline-block`}></span>
                    {lastfmConnected ? 'Connected to Last.FM' : 'Not connected'} 
                    {lastfmCanScrobble && ' • Scrobbling enabled'}
                    {lastfmUsername && ` • ${lastfmUsername}`}
                  </div>
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                      API Key
                    </label>
                    <TextInput
                      type="password"
                      value={lastfmApiKey}
                      onChange={(e) => {
                        setLastfmApiKey(e.target.value);
                        setLastfmSaveStatus('idle');
                      }}
                      placeholder={lastfmConfigured ? 'API Key saved (hidden)' : 'Enter your Last.FM API Key'}
                      className="w-full bg-surface-1"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                      Shared Secret
                    </label>
                    <TextInput
                      type="password"
                      value={lastfmSecret}
                      onChange={(e) => {
                        setLastfmSecret(e.target.value);
                        setLastfmSaveStatus('idle');
                      }}
                      placeholder="Enter your Shared Secret"
                      className="w-full bg-surface-1"
                    />
                    <p className="text-xs text-text-subtle mt-1">
                      Get your API key from <a href="https://www.last.fm/api/account/create" target="_blank" rel="noreferrer" className="text-brand hover:underline">last.fm/api/account/create</a>
                    </p>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="lastfm-enabled"
                    checked={lastfmEnabled}
                    onChange={(e) => {
                      setLastfmEnabled(e.target.checked);
                      setLastfmSaveStatus('idle');
                    }}
                    className="rounded border-surface-border bg-surface-1 text-brand focus:ring-brand"
                  />
                  <label htmlFor="lastfm-enabled" className="text-sm text-text-main cursor-pointer select-none">
                    Enable Last.FM integration
                  </label>
                </div>

                <div className="flex gap-2 flex-wrap">
                  <Button
                    variant="primary"
                    onClick={async () => {
                      try {
                        setLastfmSaveStatus('saving');
                        await api.saveLastFMSettings({
                          apiKey: lastfmApiKey || undefined,
                          sharedSecret: lastfmSecret || undefined,
                          enabled: lastfmEnabled,
                        });
                        setLastfmConfigured(true);
                        setLastfmSaveStatus('saved');
                        setTimeout(() => setLastfmSaveStatus('idle'), 3000);
                      } catch (e) {
                        console.error('Failed to save Last.FM settings:', e);
                        setLastfmSaveStatus('error');
                      }
                    }}
                    disabled={lastfmSaveStatus === 'saving'}
                    leftIcon={lastfmSaveStatus === 'saving' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                    className="text-sm font-bold"
                  >
                    {lastfmSaveStatus === 'saved' ? 'Saved!' : lastfmSaveStatus === 'saving' ? 'Saving...' : 'Save Settings'}
                  </Button>

                  <Button
                    variant="secondary"
                    onClick={async () => {
                      try {
                        setLastfmTestStatus('testing');
                        setLastfmTestMessage('');
                        const result = await api.testLastFMConnection();
                        setLastfmTestStatus(result.success ? 'success' : 'error');
                        setLastfmTestMessage(result.message);
                        setLastfmConnected(result.success);
                        setTimeout(() => setLastfmTestStatus('idle'), 5000);
                      } catch (e) {
                        console.error('Failed to test Last.FM connection:', e);
                        setLastfmTestStatus('error');
                        setLastfmTestMessage(e instanceof Error ? e.message : 'Unknown error');
                      }
                    }}
                    disabled={lastfmTestStatus === 'testing' || !lastfmConfigured}
                    leftIcon={lastfmTestStatus === 'testing' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                    className="text-sm font-bold"
                  >
                    {lastfmTestStatus === 'testing' ? 'Testing...' : 'Test Connection'}
                  </Button>
                </div>

                {lastfmTestStatus !== 'idle' && lastfmTestStatus !== 'testing' && lastfmTestMessage && (
                  <div className={`text-sm p-2 rounded ${lastfmTestStatus === 'success' ? 'bg-success/20 text-success' : 'bg-error/20 text-error'}`}>
                    {lastfmTestMessage}
                  </div>
                )}
              </div>
            )}
          </section>
        </div>
      )}

      {/* TAB 4: AI & INTELLIGENCE */}
      {activeTab === 'ai' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          <div className="bg-surface-2 rounded-xl p-6 border border-surface-border">
            <div className="flex items-center gap-3 mb-6">
              <Sparkles className="text-brand" size={24} />
              <h2 className="text-xl font-bold text-text-main">Library Intelligence & AI</h2>
            </div>

            <div className="space-y-6">
              {/* AI DJ Provider Settings */}
              <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
                <h3 className="text-lg font-bold text-text-main mb-2">AI DJ Provider</h3>
                <p className="text-text-subtle text-sm mb-4">
                  Choose which AI provider powers the AI DJ feature for natural language playlist generation.
                  Ollama runs locally (free, no API key), or use cloud providers for more powerful models.
                </p>
                
                {llmLoading ? (
                  <div className="flex items-center gap-2 text-text-subtle">
                    <Loader2 size={16} className="animate-spin" />
                    Loading provider settings...
                  </div>
                ) : (
                  <div className="flex flex-col gap-4">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {/* Provider Select */}
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                          Provider
                        </label>
                        <select
                          value={llmProvider}
                          onChange={(e) => {
                            const newProvider = e.target.value;
                            setLlmProvider(newProvider);
                            const providerModels = llmModels[newProvider];
                            if (providerModels && providerModels.length > 0) {
                              setLlmModel(providerModels[0].id);
                            }
                            if (newProvider === 'ollama') {
                              setLlmBaseURL('http://localhost:11434');
                            } else {
                              setLlmBaseURL('');
                            }
                            setLlmSaveStatus('idle');
                          }}
                          className="w-full px-3 py-2 rounded-lg bg-surface-3 border border-surface-border text-text-main focus:outline-none focus:ring-2 focus:ring-brand"
                        >
                          {llmProviders.map((p) => (
                            <option key={p.id} value={p.id}>
                              {p.name} {!p.requiresKey && '(No API Key)'}
                            </option>
                          ))}
                        </select>
                      </div>

                      {/* Model Select / Freeform */}
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                          Model
                        </label>
                        {llmProviders.find(p => p.id === llmProvider)?.freeformModel ? (
                          <div>
                            <TextInput
                              value={llmModel}
                              onChange={(e) => {
                                setLlmModel(e.target.value);
                                setLlmSaveStatus('idle');
                              }}
                              placeholder="e.g., llama3.2:8b, qwen3:4b, mistral:7b"
                              className="w-full bg-surface-3"
                            />
                          </div>
                        ) : (
                          <select
                            value={llmModel}
                            onChange={(e) => {
                              setLlmModel(e.target.value);
                              setLlmSaveStatus('idle');
                            }}
                            className="w-full px-3 py-2 rounded-lg bg-surface-3 border border-surface-border text-text-main focus:outline-none focus:ring-2 focus:ring-brand"
                          >
                            {(llmModels[llmProvider] || []).map((m) => (
                              <option key={m.id} value={m.id}>
                                {m.name}
                              </option>
                            ))}
                          </select>
                        )}
                      </div>
                    </div>

                    {/* API Key */}
                    {llmProviders.find(p => p.id === llmProvider)?.requiresKey && (
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                          API Key
                        </label>
                        <TextInput
                          type="password"
                          value={llmApiKey}
                          onChange={(e) => {
                            setLlmApiKey(e.target.value);
                            setLlmSaveStatus('idle');
                          }}
                          placeholder={llmApiKey.startsWith('****') ? 'API Key Saved (Hidden)' : 'Enter your API Key'}
                          className="w-full bg-surface-3"
                        />
                      </div>
                    )}

                    {/* Base URL */}
                    {llmProvider === 'ollama' && (
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">
                          Ollama Server URL
                        </label>
                        <TextInput
                          value={llmBaseURL}
                          onChange={(e) => {
                            setLlmBaseURL(e.target.value);
                            setLlmSaveStatus('idle');
                          }}
                          placeholder="http://localhost:11434"
                          className="w-full bg-surface-3"
                        />
                        <p className="text-xs text-text-subtle mt-1">
                          Default: http://localhost:11434. Install Ollama from <a href="https://ollama.ai" target="_blank" rel="noreferrer" className="text-brand hover:underline">ollama.ai</a>
                        </p>
                      </div>
                    )}

                    {/* Action Buttons */}
                    <div className="flex gap-2 flex-wrap">
                      <Button
                        variant="primary"
                        onClick={async () => {
                          try {
                            setLlmSaveStatus('saving');
                            await api.updateLLMSettings({
                              provider: llmProvider,
                              model: llmModel,
                              apiKey: llmApiKey.startsWith('****') ? '' : llmApiKey,
                              baseURL: llmBaseURL,
                            });
                            if (llmProvider === 'openrouter') {
                              const settings = await api.getLLMSettings();
                              setLlmModels(settings.models || {});
                            }
                            setLlmSaveStatus('saved');
                            setTimeout(() => setLlmSaveStatus('idle'), 3000);
                          } catch (e) {
                            console.error('Failed to save LLM settings:', e);
                            setLlmSaveStatus('error');
                          }
                        }}
                        disabled={llmSaveStatus === 'saving'}
                        leftIcon={llmSaveStatus === 'saving' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                        className="text-sm font-bold"
                      >
                        {llmSaveStatus === 'saved' ? 'Saved!' : llmSaveStatus === 'saving' ? 'Saving...' : 'Save Settings'}
                      </Button>

                      <Button
                        variant="secondary"
                        onClick={async () => {
                          try {
                            setLlmTestStatus('testing');
                            setLlmTestMessage('');
                            const result = await api.testLLMConnection();
                            setLlmTestStatus(result.success ? 'success' : 'error');
                            setLlmTestMessage(result.message);
                            setTimeout(() => setLlmTestStatus('idle'), 5000);
                          } catch (e) {
                            console.error('Failed to test LLM connection:', e);
                            setLlmTestStatus('error');
                            setLlmTestMessage(e instanceof Error ? e.message : 'Unknown error');
                          }
                        }}
                        disabled={llmTestStatus === 'testing'}
                        leftIcon={llmTestStatus === 'testing' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                        className="text-sm font-bold"
                      >
                        {llmTestStatus === 'testing' ? 'Testing...' : 'Test Connection'}
                      </Button>
                    </div>

                    {llmTestStatus !== 'idle' && llmTestStatus !== 'testing' && llmTestMessage && (
                      <div className={`text-sm p-2 rounded ${llmTestStatus === 'success' ? 'bg-success/20 text-success' : 'bg-error/20 text-error'}`}>
                        {llmTestMessage}
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* Semantic Retrieval Index */}
              <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
                <div className="flex items-start justify-between gap-4 mb-4">
                  <div>
                    <h3 className="text-lg font-bold text-text-main mb-1">Semantic Retrieval Index</h3>
                    <p className="text-text-subtle text-sm">
                      Builds a separate meaning-based index for AI DJ retrieval. It never changes your chat provider or library metadata.
                    </p>
                  </div>
                  <span className={`shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold ${semanticStatus?.state === 'ready' ? 'bg-success/20 text-success' : semanticStatus?.state === 'error' ? 'bg-error/20 text-error' : 'bg-surface-3 text-text-subtle'}`}>
                    {semanticStatus?.state || 'Loading'}
                  </span>
                </div>

                {semanticLoading ? (
                  <div className="flex items-center gap-2 text-text-subtle text-sm">
                    <Loader2 size={16} className="animate-spin" />
                    Loading semantic index settings...
                  </div>
                ) : (
                  <div className="space-y-4">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">Embedding provider</label>
                        <select
                          value={semanticProvider}
                          onChange={(e) => {
                            const provider = e.target.value as 'auto' | 'ollama' | 'openai' | 'openrouter' | 'gemini' | 'disabled';
                            setSemanticProvider(provider);
                            if (provider === 'openai') {
							  if (!semanticModel || semanticModel === 'nomic-embed-text' || semanticModel.startsWith('gemini-embedding') || semanticModel.includes('/')) setSemanticModel('text-embedding-3-small');
                              setSemanticDimensions(512);
                            }
							if (provider === 'openrouter') {
							  if (!semanticModel || semanticModel === 'nomic-embed-text' || semanticModel.startsWith('gemini-embedding') || semanticModel.startsWith('text-embedding-')) setSemanticModel('openai/text-embedding-3-small');
							  setSemanticDimensions(512);
							}
							if (provider === 'gemini') {
							  if (!semanticModel || semanticModel === 'nomic-embed-text' || semanticModel.startsWith('text-embedding') || semanticModel.includes('/text-embedding')) setSemanticModel('gemini-embedding-001');
							  setSemanticDimensions(768);
                            }
                            setSemanticCloudConfirmed(false);
							setSemanticCloudCost(null);
                            setSemanticSaveStatus('idle');
                          }}
                          className="w-full px-3 py-2 rounded-lg bg-surface-3 border border-surface-border text-text-main focus:outline-none focus:ring-2 focus:ring-brand"
                        >
                          <option value="auto">Auto (configured chat provider, then local Ollama)</option>
                          <option value="ollama">Ollama (local)</option>
                          <option value="openai">OpenAI (cloud embeddings)</option>
						  <option value="openrouter">OpenRouter (cloud embeddings)</option>
						  <option value="gemini">Gemini (cloud embeddings)</option>
                          <option value="disabled">Disabled</option>
                        </select>
                      </div>
                      {semanticProvider !== 'disabled' && (
                        <div>
                          <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">Embedding model</label>
                          <TextInput
                            value={semanticModel}
                            onChange={(e) => {
                              setSemanticModel(e.target.value);
                              setSemanticSaveStatus('idle');
                            }}
                            placeholder="nomic-embed-text"
                            className="w-full bg-surface-3"
                          />
                        </div>
                      )}
                    </div>

                    {semanticProvider === 'ollama' && (
                      <div>
                        <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">Ollama server URL</label>
                        <TextInput
                          value={semanticBaseURL}
                          onChange={(e) => {
                            setSemanticBaseURL(e.target.value);
                            setSemanticSaveStatus('idle');
                          }}
                          placeholder="http://localhost:11434"
                          className="w-full bg-surface-3"
                        />
                      </div>
                    )}

                    {(semanticProvider === 'openai' || semanticProvider === 'openrouter' || semanticProvider === 'gemini') && (
                      <div className="space-y-3 rounded-lg border border-surface-border bg-surface-2 p-3">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                          <div>
                            <label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">Embedding dimensions</label>
                            <TextInput type="number" min="1" value={String(semanticDimensions || 512)} onChange={(e) => { setSemanticDimensions(Number(e.target.value) || 0); setSemanticCloudConfirmed(false); setSemanticSaveStatus('idle'); }} className="w-full bg-surface-3" />
                          </div>
                          <div>
							<label className="block text-xs font-medium text-text-subtle uppercase tracking-wider mb-1">{semanticProvider === 'openai' ? 'OpenAI' : semanticProvider === 'openrouter' ? 'OpenRouter' : 'Gemini'} API key</label>
							<TextInput type="password" value={semanticAPIKey} onChange={(e) => { setSemanticAPIKey(e.target.value); setSemanticSaveStatus('idle'); }} placeholder={`Leave blank to reuse the saved ${semanticProvider} chat key`} className="w-full bg-surface-3" />
                          </div>
                        </div>
						<p className="text-xs text-warning">{semanticProvider === 'openai' ? 'OpenAI receives' : semanticProvider === 'openrouter' ? 'OpenRouter and the selected embedding provider receive' : 'Gemini receives'} deterministic semantic document text, but never file paths, internal song IDs, or listening history.</p>
                        {semanticCloudCost ? (
                          <div className="space-y-2 text-sm text-text-subtle">
							<p>
							  Current catalog: {semanticCloudCost.documents.toLocaleString()} semantic documents
							  {semanticCloudCost.pricingKnown
								? `; one-time input estimate: about $${semanticCloudCost.typicalUSD.toFixed(2)}, with a conservative maximum of $${semanticCloudCost.maximumUSD.toFixed(2)}.`
								: `. Estimated input: ${semanticCloudCost.typicalInputTokens.toLocaleString()} tokens (up to ${semanticCloudCost.maximumInputTokens.toLocaleString()}); actual ${semanticProvider} billing applies.`}
							</p>
                            <label className="flex items-start gap-2 cursor-pointer text-text-main">
                              <input type="checkbox" checked={semanticCloudConfirmed} onChange={(e) => setSemanticCloudConfirmed(e.target.checked)} className="mt-1" />
                              <span>I understand this starts a paid {semanticProvider === 'openai' ? 'OpenAI' : semanticProvider === 'openrouter' ? 'OpenRouter' : 'Gemini'} embedding build for this catalog.</span>
                            </label>
                            {semanticCloudCost.confirmed && <p className="text-success text-xs">This current estimate has already been confirmed.</p>}
                          </div>
                        ) : (
                          <p className="text-xs text-text-subtle">Save the {semanticProvider === 'openai' ? 'OpenAI' : semanticProvider === 'openrouter' ? 'OpenRouter' : 'Gemini'} configuration once to calculate the current catalog estimate before confirming it.</p>
                        )}
                      </div>
                    )}

                    <p className="text-xs text-text-subtle">
					  Ollama uses <code>POST /api/embed</code> and never downloads a model automatically. OpenAI, OpenRouter, and Gemini use their dedicated embedding APIs and will not index until the displayed cloud operation is explicitly confirmed.
                    </p>

                    {(semanticStatus?.reason || semanticStatus?.lastError) && (
                      <div className="rounded bg-warning/10 p-2 text-sm text-warning">
                        {semanticStatus.reason || semanticStatus.lastError}
                      </div>
                    )}

                    {semanticStatus && Object.keys(semanticStatus.documentsByStatus).length > 0 && (
                      <div className="text-xs text-text-subtle">
                        Documents: {semanticStatus.documentsByStatus.ready || 0} ready, {semanticStatus.documentsByStatus.pending || 0} pending, {semanticStatus.documentsByStatus.error || 0} errors
                        {semanticStatus.state === 'indexing' && ` (${Math.round(semanticStatus.progress * 100)}% ready)`}
                      </div>
                    )}

                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="primary"
                        onClick={async () => {
                          try {
                            setSemanticSaveStatus('saving');
                            await api.updateSemanticSettings({
                              provider: semanticProvider,
                              model: semanticModel,
							  dimensions: semanticProvider === 'openai' || semanticProvider === 'openrouter' || semanticProvider === 'gemini' ? semanticDimensions : undefined,
                              baseURL: semanticProvider === 'ollama' ? semanticBaseURL : '',
							  apiKey: (semanticProvider === 'openai' || semanticProvider === 'openrouter' || semanticProvider === 'gemini') && semanticAPIKey ? semanticAPIKey : undefined,
							  confirmCloudCost: (semanticProvider === 'openai' || semanticProvider === 'openrouter' || semanticProvider === 'gemini') && semanticCloudConfirmed,
                            });
                            const settings = await api.getSemanticSettings();
                            setSemanticCloudCost(settings.cloudCost || null);
                            setSemanticCloudConfirmed(false);
                            setSemanticAPIKey('');
                            setSemanticSaveStatus('saved');
                            await refreshSemanticStatus();
                            setTimeout(() => setSemanticSaveStatus('idle'), 3000);
                          } catch (e) {
                            console.error('Failed to save semantic index settings:', e);
                            setSemanticSaveStatus('error');
                          }
                        }}
                        disabled={semanticSaveStatus === 'saving'}
                        leftIcon={semanticSaveStatus === 'saving' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                        className="text-sm font-bold"
                      >
                        {semanticSaveStatus === 'saved' ? 'Saved!' : semanticSaveStatus === 'saving' ? 'Saving...' : 'Save Index Settings'}
                      </Button>

                      <Button
                        variant="secondary"
                        onClick={async () => {
                          try {
                            setSemanticActionStatus('working');
                            setSemanticActionMessage('');
                            const result = await api.testSemanticEmbeddingProvider();
                            setSemanticActionStatus(result.success ? 'success' : 'error');
                            setSemanticActionMessage(result.success ? `Connected to ${result.provider} / ${result.model} (${result.dimensions} dimensions).` : result.message || 'Provider test failed.');
                            await refreshSemanticStatus();
                          } catch (e) {
                            setSemanticActionStatus('error');
                            setSemanticActionMessage(e instanceof Error ? e.message : 'Provider test failed.');
                          }
                        }}
                        disabled={semanticActionStatus === 'working' || semanticStatus?.state === 'disabled' || semanticStatus?.state === 'needs_configuration'}
                        leftIcon={semanticActionStatus === 'working' ? <Loader2 size={16} className="animate-spin" /> : undefined}
                        className="text-sm font-bold"
                      >
                        Test Embedding Provider
                      </Button>

                      <Button
                        variant="secondary"
                        onClick={async () => {
                          try {
                            setSemanticActionStatus('working');
                            await api.rebuildSemanticIndex();
                            setSemanticActionStatus('success');
                            setSemanticActionMessage('Semantic reindex started in the background.');
                            await refreshSemanticStatus();
                          } catch (e) {
                            setSemanticActionStatus('error');
                            setSemanticActionMessage(e instanceof Error ? e.message : 'Could not start the semantic reindex.');
                          }
                        }}
                        disabled={semanticActionStatus === 'working' || semanticStatus?.state === 'disabled' || semanticStatus?.state === 'needs_configuration'}
                        leftIcon={<RefreshCw size={16} />}
                        className="text-sm font-bold"
                      >
                        Rebuild Index
                      </Button>

                      <Button
                        variant="secondary"
                        onClick={async () => {
                          try {
                            setSemanticActionStatus('working');
                            await api.retrySemanticErrors();
                            setSemanticActionStatus('success');
                            setSemanticActionMessage('Eligible semantic embedding errors were queued for retry.');
                            await refreshSemanticStatus();
                          } catch (e) {
                            setSemanticActionStatus('error');
                            setSemanticActionMessage(e instanceof Error ? e.message : 'Could not queue semantic retries.');
                          }
                        }}
                        disabled={semanticActionStatus === 'working' || (semanticStatus?.documentsByStatus.error || 0) === 0}
                        className="text-sm font-bold"
                      >
                        Retry Errors
                      </Button>
                    </div>

                    {semanticActionStatus !== 'idle' && semanticActionStatus !== 'working' && semanticActionMessage && (
                      <div className={`text-sm p-2 rounded ${semanticActionStatus === 'success' ? 'bg-success/20 text-success' : 'bg-error/20 text-error'}`}>
                        {semanticActionMessage}
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* Enrichment Source Selection */}
              <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
                <h3 className="text-lg font-bold text-text-main mb-2">Metadata Enrichment Source</h3>
                <p className="text-text-subtle text-sm mb-4">
                  Choose which system to use for automatic metadata enrichment during library scans.
                  AI DJ features will still use the configured AI provider regardless of this setting.
                </p>
                
                <div className="space-y-3">
                  <label className="flex items-start gap-3 p-3 rounded-lg border border-surface-border hover:border-brand/50 cursor-pointer transition-colors">
                    <input
                      type="radio"
                      name="enrichment-source"
                      value="ai"
                      checked={enrichmentSource === 'ai'}
                      onChange={() => {
                        setEnrichmentSource('ai');
                        api.saveLastFMSettings({ enrichmentSource: 'ai' }).catch(console.error);
                      }}
                      className="mt-1 text-brand focus:ring-brand"
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <Sparkles size={16} className="text-brand" />
                        <span className="font-medium text-text-main">AI Enrichment</span>
                        {enrichmentSource === 'ai' && (
                          <span className="text-xs bg-brand/20 text-brand px-2 py-0.5 rounded-full">Active</span>
                        )}
                      </div>
                      <p className="text-xs text-text-subtle mt-1">
                        Uses configured LLM provider for genre, mood, energy, and tempo analysis.
                      </p>
                    </div>
                  </label>

                  <label className={`flex items-start gap-3 p-3 rounded-lg border border-surface-border hover:border-brand/50 cursor-pointer transition-colors ${!lastfmConfigured ? 'opacity-50' : ''}`}>
                    <input
                      type="radio"
                      name="enrichment-source"
                      value="lastfm"
                      checked={enrichmentSource === 'lastfm'}
                      onChange={() => {
                        if (lastfmConfigured) {
                          setEnrichmentSource('lastfm');
                          api.saveLastFMSettings({ enrichmentSource: 'lastfm' }).catch(console.error);
                        }
                      }}
                      disabled={!lastfmConfigured}
                      className="mt-1 text-brand focus:ring-brand"
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <Music size={16} className="text-[#d51007]" />
                        <span className="font-medium text-text-main">Last.FM Enrichment</span>
                        {enrichmentSource === 'lastfm' && (
                          <span className="text-xs bg-brand/20 text-brand px-2 py-0.5 rounded-full">Active</span>
                        )}
                      </div>
                      <p className="text-xs text-text-subtle mt-1">
                        Uses Last.FM's community-sourced tags and metadata. Free, no API costs.
                      </p>
                    </div>
                  </label>

                  <label className={`flex items-start gap-3 p-3 rounded-lg border border-surface-border hover:border-brand/50 cursor-pointer transition-colors ${!lastfmConfigured ? 'opacity-50' : ''}`}>
                    <input
                      type="radio"
                      name="enrichment-source"
                      value="hybrid"
                      checked={enrichmentSource === 'hybrid'}
                      onChange={() => {
                        if (lastfmConfigured) {
                          setEnrichmentSource('hybrid');
                          api.saveLastFMSettings({ enrichmentSource: 'hybrid' }).catch(console.error);
                        }
                      }}
                      disabled={!lastfmConfigured}
                      className="mt-1 text-brand focus:ring-brand"
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <Zap size={16} className="text-warning" />
                        <span className="font-medium text-text-main">Hybrid Mode</span>
                        {enrichmentSource === 'hybrid' && (
                          <span className="text-xs bg-brand/20 text-brand px-2 py-0.5 rounded-full">Active</span>
                        )}
                      </div>
                      <p className="text-xs text-text-subtle mt-1">
                        Uses Last.FM first for free metadata, falls back to AI for missing tracks.
                      </p>
                    </div>
                  </label>
                </div>
              </div>

              {/* Unified AI Enrichment */}
              <div className="bg-surface-1 rounded-lg p-4 border-2 border-brand">
                <div className="flex items-center gap-2 mb-2">
                  <span className="px-2 py-0.5 text-xs font-bold bg-brand text-black rounded">RECOMMENDED</span>
                  <h3 className="text-lg font-bold text-text-main">Unified AI Enrichment</h3>
                </div>
                <p className="text-text-subtle text-sm mb-4">
                  Enrich your entire library with genres, mood, energy, tempo, BPM, and original year detection 
                  in a <strong>single efficient operation</strong>.
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
                      const hasLLMAccess = llmProvider && (llmProvider === 'ollama' ? llmBaseURL : llmApiKey);
                      if (!hasLLMAccess) {
                        alert("Please configure an AI Provider in the section above first.");
                        return;
                      }

                      setIsUnifiedEnriching(true);
                      setUnifiedStatus('Connecting to unified enrichment service...');
                      setUnifiedProgress(null);
					  setEnrichmentStatus({
						isEnriching: true,
						totalSongs: 0,
						processedSongs: 0,
						currentBatch: 0,
						totalBatches: 0,
						message: 'Connecting to AI enrichment service…',
					  });

                      const eventSource = api.enrichAllMetadataStream(forceUnified, (progress) => {
                        setUnifiedStatus(progress.message);
                        updateGlobalEnrichmentStatus(progress, 'AI metadata enrichment is running');
                        
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
						  void refreshLibrary();
						  window.dispatchEvent(new CustomEvent('enrichment_complete', { detail: progress }));
                          setTimeout(() => setUnifiedProgress(null), 5000);
                        }

						if (progress.status === 'batch_complete') {
						  void refreshLibrary();
						}
                        
                        if (progress.status === 'error') {
                          setIsUnifiedEnriching(false);
                          setUnifiedProgress(null);
                          setUnifiedStatus(`Error: ${progress.error || progress.message}`);
                        }
                      });

                      return () => eventSource.close();
                    }}
                    disabled={isUnifiedEnriching || !(llmProvider && (llmProvider === 'ollama' ? llmBaseURL : llmApiKey))}
                    variant={isUnifiedEnriching || !(llmProvider && (llmProvider === 'ollama' ? llmBaseURL : llmApiKey)) ? 'secondary' : 'primary'}
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

              {/* Year Backfill & Remaster Detection */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
                  <h3 className="text-base font-bold text-text-main mb-1">Year Data Backfill</h3>
                  <p className="text-text-subtle text-xs mb-3">
                    Populate missing song year values from album metadata release dates.
                  </p>
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
                    className="self-start text-xs py-2 px-3 font-bold"
                  >
                    📅 Backfill Song Years
                  </Button>
                  {yearBackfillStatus && (
                    <div className={`text-xs mt-2 ${yearBackfillStatus.startsWith('Error') ? 'text-error' : 'text-success'}`}>
                      {yearBackfillStatus}
                    </div>
                  )}
                </div>

                <div className="bg-surface-1 rounded-lg p-4 border border-surface-border">
                  <h3 className="text-base font-bold text-text-main mb-1">Original Year Detection</h3>
                  <p className="text-text-subtle text-xs mb-3">
                    Detect songs with remaster dates vs original release years.
                  </p>
                  <div className="flex gap-2">
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
                      className="text-xs py-2 px-3 font-bold"
                    >
                      🔍 Detect Remasters
                    </Button>
                  </div>
                  {remasterStatus && (
                    <div className={`text-xs mt-2 ${remasterStatus.startsWith('Error') ? 'text-error' : 'text-success'}`}>
                      {remasterStatus}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* TAB 5: APPEARANCE & PERSONALIZATION */}
      {activeTab === 'appearance' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-6 text-brand">
                <Sparkles size={20} />
                <h2 className="text-lg font-bold text-text-main">Personalization & Layout</h2>
            </div>
            
            <div className="space-y-6">
              <div>
                <div className="mb-3">
                    <h3 className="font-medium text-text-main">Home Layout</h3>
                    <p className="text-sm text-text-subtle">Choose the Home screen experience.</p>
                </div>
                <div className="grid gap-3 md:grid-cols-3" role="radiogroup" aria-label="Home layout">
                  {HOME_LAYOUT_OPTIONS.map((option) => {
                    const selected = homeLayoutVariant === option.value;
                    return (
                      <button
                        key={option.value}
                        type="button"
                        role="radio"
                        aria-checked={selected}
                        onClick={() => setHomeLayoutVariant(option.value)}
                        className={`text-left rounded-lg border p-4 transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-2 ${
                          selected
                            ? 'border-brand bg-brand/10 ring-1 ring-brand/50'
                            : 'border-surface-border bg-surface-1 hover:border-white/20 hover:bg-surface-hover'
                        }`}
                      >
                        <div className="flex items-center justify-between gap-3">
                          <span className="font-semibold text-text-main">{option.label}</span>
                          {selected && <span className="text-xs font-semibold uppercase tracking-wide text-brand">Selected</span>}
                        </div>
                        <p className="mt-2 text-sm text-text-subtle">{option.description}</p>
                      </button>
                    );
                  })}
                </div>
              </div>

              <div className="flex items-center justify-between border-t border-surface-hover pt-6">
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
            </div>
          </section>
        </div>
      )}

      {/* TAB 6: SYSTEM & LOGS */}
      {activeTab === 'system' && (
        <div className="space-y-6 animate-in fade-in duration-200">
          {/* Server Info */}
          <section className="bg-surface-2 rounded-xl p-6 border border-surface-3">
            <div className="flex items-center gap-3 mb-4 text-brand">
                <Server size={20} />
                <h2 className="text-lg font-bold text-text-main">System Environment</h2>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
              <div className="p-4 rounded-lg bg-surface-1 border border-surface-border flex items-center justify-between">
                <div>
                  <span className="text-text-subtle uppercase font-bold block mb-1">Architecture</span>
                  <span className="font-mono text-sm text-text-main">{backendAvailable ? 'Native Wails (Go Backend)' : 'Browser Standalone'}</span>
                </div>
                {backendAvailable ? (
                  <span className="p-2 rounded-full bg-success/20 text-success"><Server size={18} /></span>
                ) : (
                  <span className="p-2 rounded-full bg-warning/20 text-warning"><MonitorOff size={18} /></span>
                )}
              </div>

              <div className="p-4 rounded-lg bg-surface-1 border border-surface-border flex items-center justify-between">
                <div>
                  <span className="text-text-subtle uppercase font-bold block mb-1">Active Folders</span>
                  <span className="font-mono text-sm text-text-main">{scanFolders.length} Configured Folder{scanFolders.length !== 1 ? 's' : ''}</span>
                </div>
                <FolderOpen size={18} className="text-brand" />
              </div>
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
                      Clear Logs
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
              <p className="text-[10px] text-surface-slider mt-2">Logs capture AI enhancement progress, Spotify API activity, and internal errors.</p>
          </section>
        </div>
      )}

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
    </Page>
  );
};
