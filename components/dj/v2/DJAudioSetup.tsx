/**
 * ViiB MediaHub - DJ Audio Setup Component
 *
 * Output routing dialog for assigning separate master and headphone/cue devices.
 *
 * @module components/dj/v2/DJAudioSetup
 */

import React, { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { Headphones, RefreshCw, Volume2 } from 'lucide-react';
import { getDJAudioEngine } from '../../../lib/djAudio';

interface DJAudioSetupProps {
  onClose: () => void;
}

type ApplyingTarget = 'main' | 'headphones' | null;

const buttonBase = 'min-h-[32px] px-3 rounded text-[12px] font-bold uppercase tracking-wider transition-colors border';

function getErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

function supportsAudioContextSink(): boolean {
  const AudioContextCtor = typeof window !== 'undefined'
    ? window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    : undefined;
  return !!AudioContextCtor?.prototype && 'setSinkId' in AudioContextCtor.prototype;
}

function supportsMediaElementSink(): boolean {
  return typeof HTMLMediaElement !== 'undefined' && 'setSinkId' in HTMLMediaElement.prototype;
}

export const DJAudioSetup: React.FC<DJAudioSetupProps> = memo(({ onClose }) => {
  const engine = getDJAudioEngine();
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [mainDeviceId, setMainDeviceId] = useState(engine.getMainOutputDeviceId());
  const [headphoneDeviceId, setHeadphoneDeviceId] = useState(engine.getHeadphoneOutputDeviceId());
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState<ApplyingTarget>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [labelsUnlocked, setLabelsUnlocked] = useState(false);

  const hasMediaDevices = typeof navigator !== 'undefined' && !!navigator.mediaDevices?.enumerateDevices;
  const canRouteHeadphones = supportsMediaElementSink();
  const canRouteMain = supportsAudioContextSink() || canRouteHeadphones;

  const loadDevices = useCallback(async () => {
    setError(null);
    if (!hasMediaDevices) {
      setDevices([]);
      setError('This browser does not expose media device enumeration.');
      return;
    }

    setLoading(true);
    try {
      const allDevices = await navigator.mediaDevices.enumerateDevices();
      const outputs = allDevices.filter(device => device.kind === 'audiooutput');
      setDevices(outputs);
      setLabelsUnlocked(outputs.some(device => !!device.label));
      if (outputs.length === 0) {
        setMessage('No audio output devices were reported. The system default output can still be used.');
      }
    } catch (err) {
      setDevices([]);
      setError(`Could not enumerate audio devices: ${getErrorMessage(err)}`);
    } finally {
      setLoading(false);
    }
  }, [hasMediaDevices]);

  useEffect(() => {
    loadDevices();
  }, [loadDevices]);

  useEffect(() => {
    if (!navigator.mediaDevices?.addEventListener) return;
    const onDeviceChange = () => {
      loadDevices();
    };
    navigator.mediaDevices.addEventListener('devicechange', onDeviceChange);
    return () => navigator.mediaDevices.removeEventListener('devicechange', onDeviceChange);
  }, [loadDevices]);

  const outputOptions = useMemo(() => {
    const defaultDevice = devices.find(device => device.deviceId === 'default');
    const defaultLabel = defaultDevice?.label ? `System default (${defaultDevice.label})` : 'System default';
    const unique = devices.filter(device => device.deviceId && device.deviceId !== 'default');
    return [
      { id: '', label: defaultLabel },
      ...unique.map((device, index) => ({
        id: device.deviceId,
        label: device.label || `Audio output ${index + 1}`,
      })),
    ];
  }, [devices]);

  const requestDeviceLabels = useCallback(async () => {
    setError(null);
    setMessage(null);
    if (!navigator.mediaDevices?.getUserMedia) {
      setError('This browser cannot request audio permission to reveal device names.');
      return;
    }

    setLoading(true);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      stream.getTracks().forEach(track => track.stop());
      setMessage('Audio permission granted. Device names refreshed.');
      await loadDevices();
    } catch (err) {
      setError(`Could not request audio permission: ${getErrorMessage(err)}`);
    } finally {
      setLoading(false);
    }
  }, [loadDevices]);

  const applyMainOutput = useCallback(async () => {
    setError(null);
    setMessage(null);
    setApplying('main');
    try {
      await engine.setMainOutputDevice(mainDeviceId);
      setMessage(engine.initialized
        ? 'Master output device applied.'
        : 'Master output device saved and will apply when DJ audio initializes.');
    } catch (err) {
      setError(`Could not apply master output: ${getErrorMessage(err)}`);
    } finally {
      setApplying(null);
    }
  }, [engine, mainDeviceId]);

  const applyHeadphoneOutput = useCallback(async () => {
    setError(null);
    setMessage(null);
    setApplying('headphones');
    try {
      await engine.setHeadphoneOutputDevice(headphoneDeviceId);
      setMessage(engine.initialized
        ? 'Headphone output device applied.'
        : 'Headphone output device saved and will apply when DJ audio initializes.');
    } catch (err) {
      setError(`Could not apply headphone output: ${getErrorMessage(err)}`);
    } finally {
      setApplying(null);
    }
  }, [engine, headphoneDeviceId]);

  return (
    <div className='fixed inset-0 z-[100] bg-black/60 flex items-center justify-center' onClick={onClose}>
      <div
        className='bg-[var(--dj-surface-2)] border border-[var(--dj-border-light)] rounded-lg w-[620px] max-h-[82vh] flex flex-col shadow-2xl'
        onClick={e => e.stopPropagation()}
        role='dialog'
        aria-modal='true'
        aria-label='Audio setup'
      >
        <div className='flex items-center justify-between px-4 py-3 border-b border-[var(--dj-border-light)]'>
          <div className='flex items-center gap-2'>
            <Volume2 size={16} className='text-[var(--dj-info)]' />
            <span className='text-sm font-bold text-[var(--dj-text-primary)]'>Audio Setup</span>
            <span className='text-[12px] bg-[color-mix(in_srgb,var(--dj-info)_20%,transparent)] text-[var(--dj-info)] px-1.5 py-0.5 rounded'>
              {outputOptions.length} output option(s)
            </span>
          </div>
          <button onClick={onClose} className='text-[var(--dj-text-secondary)] hover:text-[var(--dj-text-secondary)] text-lg' aria-label='Close audio setup'>
            x
          </button>
        </div>

        <div className='flex-1 overflow-y-auto p-4 space-y-4'>
          {(!canRouteMain || !canRouteHeadphones) && (
            <div className='rounded border border-[color-mix(in_srgb,var(--dj-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--dj-warning)_10%,transparent)] px-3 py-2 text-[12px] text-amber-200'>
              {!canRouteMain && <div>This runtime does not advertise master output routing support; Apply will report the exact engine result.</div>}
              {!canRouteHeadphones && <div>This runtime does not advertise headphone output routing support; Apply will report the exact engine result.</div>}
            </div>
          )}

          {error && (
            <div className='rounded border border-[color-mix(in_srgb,var(--dj-danger)_30%,transparent)] bg-[color-mix(in_srgb,var(--dj-danger)_10%,transparent)] px-3 py-2 text-[12px] text-[var(--dj-danger)]'>
              {error}
            </div>
          )}

          {message && (
            <div className='rounded border border-[color-mix(in_srgb,var(--dj-play)_30%,transparent)] bg-[color-mix(in_srgb,var(--dj-play)_10%,transparent)] px-3 py-2 text-[12px] text-[var(--dj-play-hover)]'>
              {message}
            </div>
          )}

          {!labelsUnlocked && hasMediaDevices && (
            <div className='rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-1)] px-3 py-2 flex items-center justify-between gap-3'>
              <div>
                <div className='text-[12px] font-bold text-[var(--dj-text-secondary)] uppercase tracking-wider'>Device names hidden</div>
                <div className='text-[12px] text-[var(--dj-text-secondary)] mt-0.5'>
                  Some browsers hide output names until audio permission is granted.
                </div>
              </div>
              <button
                onClick={requestDeviceLabels}
                disabled={loading}
                className={`${buttonBase} bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] hover:bg-[var(--dj-border)] disabled:opacity-50`}
              >
                Show Names
              </button>
            </div>
          )}

          <section className='rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-1)] p-3'>
            <div className='flex items-center gap-2 mb-3'>
              <Volume2 size={15} className='text-[var(--dj-info)]' />
              <div>
                <h3 className='text-sm font-bold text-[var(--dj-text-primary)] leading-tight'>Master Output</h3>
                <p className='text-[12px] text-[var(--dj-text-secondary)]'>Main speaker output for the live mix.</p>
              </div>
            </div>
            <div className='flex items-center gap-2'>
              <select
                value={mainDeviceId}
                onChange={e => setMainDeviceId(e.target.value)}
                disabled={applying !== null}
                className='flex-1 bg-[var(--dj-bg)] border border-[var(--dj-border-light)] rounded px-3 py-2 text-xs text-[var(--dj-text-primary)] disabled:opacity-50 focus:outline-none focus:ring-1 focus:ring-[var(--dj-info)]'
                aria-label='Master output device'
              >
                {outputOptions.map(option => (
                  <option key={option.id || 'default'} value={option.id}>{option.label}</option>
                ))}
              </select>
              <button
                onClick={applyMainOutput}
                disabled={applying !== null}
                className={`${buttonBase} bg-[color-mix(in_srgb,var(--dj-info)_20%,transparent)] text-[var(--dj-info)] border-[color-mix(in_srgb,var(--dj-info)_30%,transparent)] hover:bg-[color-mix(in_srgb,var(--dj-info)_30%,transparent)] disabled:opacity-50`}
              >
                {applying === 'main' ? 'Applying' : 'Apply'}
              </button>
            </div>
          </section>

          <section className='rounded border border-[var(--dj-border-light)] bg-[var(--dj-surface-1)] p-3'>
            <div className='flex items-center gap-2 mb-3'>
              <Headphones size={15} className='text-[var(--dj-hotcue-hover)]' />
              <div>
                <h3 className='text-sm font-bold text-[var(--dj-text-primary)] leading-tight'>Headphone / Cue Output</h3>
                <p className='text-[12px] text-[var(--dj-text-secondary)]'>Separate monitor output for deck cues and cue/master blend.</p>
              </div>
            </div>
            <div className='flex items-center gap-2'>
              <select
                value={headphoneDeviceId}
                onChange={e => setHeadphoneDeviceId(e.target.value)}
                disabled={applying !== null}
                className='flex-1 bg-[var(--dj-bg)] border border-[var(--dj-border-light)] rounded px-3 py-2 text-xs text-[var(--dj-text-primary)] disabled:opacity-50 focus:outline-none focus:ring-1 focus:ring-[var(--dj-hotcue)]'
                aria-label='Headphone output device'
              >
                {outputOptions.map(option => (
                  <option key={option.id || 'default'} value={option.id}>{option.label}</option>
                ))}
              </select>
              <button
                onClick={applyHeadphoneOutput}
                disabled={applying !== null}
                className={`${buttonBase} bg-[color-mix(in_srgb,var(--dj-hotcue)_20%,transparent)] text-orange-300 border-[color-mix(in_srgb,var(--dj-hotcue)_30%,transparent)] hover:bg-[color-mix(in_srgb,var(--dj-hotcue)_30%,transparent)] disabled:opacity-50`}
              >
                {applying === 'headphones' ? 'Applying' : 'Apply'}
              </button>
            </div>
          </section>
        </div>

        <div className='flex items-center justify-between px-4 py-3 border-t border-[var(--dj-border-light)]'>
          <button
            onClick={loadDevices}
            disabled={loading}
            className={`${buttonBase} bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] hover:bg-[var(--dj-border)] hover:text-[var(--dj-text-primary)] disabled:opacity-50 flex items-center gap-1.5`}
          >
            <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
            Refresh
          </button>
          <button
            onClick={onClose}
            className={`${buttonBase} bg-[var(--dj-border-light)] text-white border-[var(--dj-border-hover)] hover:bg-[var(--dj-border-hover)]`}
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
});

DJAudioSetup.displayName = 'DJAudioSetup';

export default DJAudioSetup;
