/** Configures master and headphone audio-output devices for DJ playback. */

import React, { useEffect, useState } from 'react';
import { AlertTriangle, Headphones, Lightbulb, Loader2, Speaker } from 'lucide-react';
import { useStore } from '../store';

/**
 * AudioOutputSettings - DJ Mode audio output device configuration
 *
 * Allows the user to select separate audio output devices for:
 * - Main/Live output (speakers/PA system)
 * - Headphone/Cue output (DJ headphones for previewing tracks)
 *
 * Uses the Web Audio API's setSinkId for device routing.
 */
export const AudioOutputSettings: React.FC = () => {
    const { audioSettings, setMainOutputDevice, setHeadphoneOutputDevice } = useStore();
    const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [permissionDenied, setPermissionDenied] = useState(false);

    // Enumerate available audio output devices
    useEffect(() => {
        const mediaDevices = navigator.mediaDevices;
        if (!mediaDevices?.enumerateDevices) {
            setAudioDevices([]);
            setPermissionDenied(true);
            setIsLoading(false);
            return;
        }

        const loadDevices = async () => {
            try {
                setIsLoading(true);
                // Request permission to enumerate devices (requires user gesture in some browsers)
                if (mediaDevices.getUserMedia) {
                    await mediaDevices.getUserMedia({ audio: true }).then(stream => {
                        // Stop the stream immediately - we just needed permission
                        stream.getTracks().forEach(track => track.stop());
                    }).catch(() => {
                        // Permission denied or not available - still try to enumerate
                        console.warn('Could not get audio permission, device list may be limited');
                    });
                }

                const devices = await mediaDevices.enumerateDevices();
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
        mediaDevices.addEventListener?.('devicechange', handleDeviceChange);
        return () => {
            mediaDevices.removeEventListener?.('devicechange', handleDeviceChange);
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
                    <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
                    <span>Loading audio devices...</span>
                </div>
            </div>
        );
    }

    if (permissionDenied) {
        return (
            <div className="pt-4 mt-4 border-t border-surface-hover">
                <div className="flex items-center gap-3 text-amber-400">
                    <AlertTriangle className="w-4 h-4" aria-hidden="true" />
                    <span className="text-sm">Audio device permission required for DJ output routing</span>
                </div>
            </div>
        );
    }

    return (
        <div className="pt-4 mt-4 border-t border-surface-hover space-y-4">
            <div className="flex items-center gap-2 text-brand">
                <Headphones size={16} aria-hidden="true" />
                <h3 className="text-base font-semibold text-text-main">DJ Mode Audio Output</h3>
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
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                        <div className="flex items-center gap-3 min-w-0">
                            <div className="p-2 bg-surface-hover rounded-lg text-text-main shrink-0">
                                <Speaker size={18} aria-hidden="true" />
                            </div>
                            <div className="min-w-0">
                                <h4 className="font-medium text-text-main">Main Output (Live)</h4>
                                <p className="text-xs text-text-subtle">Speakers or PA system for the audience</p>
                            </div>
                        </div>
                        <select
                            aria-label="Main output device"
                            value={audioSettings.mainOutputDevice || ''}
                            onChange={(e) => setMainOutputDevice(e.target.value)}
                            className="w-full sm:w-auto sm:max-w-[250px] max-w-full bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
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
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                        <div className="flex items-center gap-3 min-w-0">
                            <div className="p-2 bg-surface-hover rounded-lg text-text-main shrink-0">
                                <Headphones size={18} aria-hidden="true" />
                            </div>
                            <div className="min-w-0">
                                <h4 className="font-medium text-text-main">Headphone Output (Cue)</h4>
                                <p className="text-xs text-text-subtle">Preview next track while mixing</p>
                            </div>
                        </div>
                        <select
                            aria-label="Headphone output device"
                            value={audioSettings.headphoneOutputDevice || ''}
                            onChange={(e) => setHeadphoneOutputDevice(e.target.value)}
                            className="w-full sm:w-auto sm:max-w-[250px] max-w-full bg-surface-1 border border-surface-border rounded px-3 py-2 text-sm text-text-main focus:border-brand outline-none"
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
                            <AlertTriangle size={14} className="shrink-0" aria-hidden="true" />
                            <span>
                                Main and headphone outputs are the same device.
                                For proper cueing, select different devices.
                            </span>
                        </div>
                    )}

                    {/* Warning if selected device no longer available */}
                    {audioSettings.mainOutputDevice && !isDeviceAvailable(audioSettings.mainOutputDevice) && (
                        <div className="flex items-center gap-2 text-red-400 text-xs bg-red-400/10 p-2 rounded">
                            <AlertTriangle size={14} className="shrink-0" aria-hidden="true" />
                            <span>Main output device no longer available. Please select a new device.</span>
                        </div>
                    )}
                    {audioSettings.headphoneOutputDevice && !isDeviceAvailable(audioSettings.headphoneOutputDevice) && (
                        <div className="flex items-center gap-2 text-red-400 text-xs bg-red-400/10 p-2 rounded">
                            <AlertTriangle size={14} className="shrink-0" aria-hidden="true" />
                            <span>Headphone output device no longer available. Please select a new device.</span>
                        </div>
                    )}

                    <p className="flex items-start gap-2 text-xs text-text-subtle italic">
                        <Lightbulb size={14} className="shrink-0 text-brand mt-0.5" aria-hidden="true" />
                        <span>
                            Tip: Connect a USB audio interface with multiple outputs for professional DJ setups.
                            Changes take effect in DJ Mode when audio is next loaded.
                        </span>
                    </p>
                </div>
            )}
        </div>
    );
};
