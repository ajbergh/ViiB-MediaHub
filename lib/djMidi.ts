/**
 * ViiB MediaHub - DJ MIDI Controller Mapping
 * 
 * Web MIDI API integration for DJ controller support.
 * Provides MIDI input detection, message routing, and configurable
 * controller-to-action mapping with MIDI learn mode.
 * 
 * Architecture:
 * - DJMidiService: Singleton managing MIDI access and message routing
 * - MidiMapping: Configuration mapping MIDI messages → DJ actions
 * - MIDI Learn: Interactive mapping via "press button on controller" workflow
 * 
 * @module lib/djMidi
 */

// ============================================================================
// Types
// ============================================================================

import type { DeckId } from '../slices/djMixerSlice';
import type { PadMode } from './djPadModes';
import type { MidiLEDMessage } from './djMidiFeedback';
import { isMixtrackGo } from './mixtrackGoProfile';
import { controllerProfileForName } from './djControllerProfiles';

export type MidiMessageType = 'noteon' | 'noteoff' | 'cc' | 'pitchbend';

// MixTrack Go Mode note on zero-based channels 4 (A) and 5 (B).
export const MIXTRACK_PAD_MODE_NOTE_OR_CC = 0x00;

export interface MidiMessage {
  type: MidiMessageType;
  channel: number;    // 0-15
  note: number;       // 0-127 (note number or CC number)
  velocity: number;   // 0-127
  deviceId: string;   // MIDI input device ID
  deviceName: string; // Human-readable device name
}

/** An action that can be triggered by MIDI */
export type MidiAction =
  | 'headphoneVolume' | 'library.browse' | 'library.open'
  | 'deckA.filter' | 'deckB.filter' | 'deckA.jogTouch' | 'deckB.jogTouch'
  | 'deckA.stemAcapella' | 'deckB.stemAcapella' | 'deckA.stemInstrumental' | 'deckB.stemInstrumental'
  | 'deckA.load' | 'deckB.load'
  | 'deckA.padMode' | 'deckB.padMode'
  | 'deckA.performancePad1' | 'deckA.performancePad2' | 'deckA.performancePad3' | 'deckA.performancePad4'
  | 'deckB.performancePad1' | 'deckB.performancePad2' | 'deckB.performancePad3' | 'deckB.performancePad4'
  // Transport
  | 'deckA.play' | 'deckB.play'
  | 'deckA.cue' | 'deckB.cue'
  | 'deckA.sync' | 'deckB.sync'
  // Volume / Faders
  | 'deckA.volume' | 'deckB.volume'
  | 'crossfader'
  | 'masterVolume'
  // EQ
  | 'deckA.eqHigh' | 'deckB.eqHigh'
  | 'deckA.eqMid' | 'deckB.eqMid'
  | 'deckA.eqLow' | 'deckB.eqLow'
  // Tempo
  | 'deckA.tempo' | 'deckB.tempo'
  // Jog / Scratch
  | 'deckA.jogWheel' | 'deckB.jogWheel'
  // Hot Cues
  | 'deckA.hotCue1' | 'deckA.hotCue2' | 'deckA.hotCue3' | 'deckA.hotCue4'
  | 'deckB.hotCue1' | 'deckB.hotCue2' | 'deckB.hotCue3' | 'deckB.hotCue4'
  // FX
  | 'deckA.fxWet' | 'deckB.fxWet'
  // Sampler
  | 'sampler.pad1' | 'sampler.pad2' | 'sampler.pad3' | 'sampler.pad4'
  | 'sampler.pad5' | 'sampler.pad6' | 'sampler.pad7' | 'sampler.pad8'
  // Loop
  | 'deckA.loopIn' | 'deckA.loopOut' | 'deckA.loopToggle'
  | 'deckB.loopIn' | 'deckB.loopOut' | 'deckB.loopToggle'
  // Headphone Cue
  | 'deckA.headphoneCue' | 'deckB.headphoneCue';

/** How to interpret the MIDI value for this mapping */
export type MidiValueMode = 
  | 'toggle'     // Note on = toggle action (buttons)
  | 'momentary'  // Note on = activate, note off = deactivate
  | 'absolute'   // CC value 0-127 = 0.0-1.0 (faders/knobs)
  | 'relative'   // CC value: 1-63 = increment, 65-127 = decrement (encoders)
  | 'trigger';   // Note on = trigger once (pads)

export interface MidiMapping {
  id: string;             // Unique mapping ID
  deviceId: string;       // MIDI device ID (or '*' for any device)
  type: MidiMessageType;  // noteon, cc, etc.
  channel: number;        // MIDI channel 0-15 (-1 for any)
  note: number;           // Note/CC number 0-127
  action: MidiAction;     // DJ action to trigger
  valueMode: MidiValueMode;
  label?: string;         // Human-readable label
  deck?: DeckId;          // Source deck for a mode-specific mapping
  padMode?: PadMode;      // Omitted for controls available in every mode
  ccLSB?: number;         // Optional paired-CC low byte for 14-bit absolute values
  relativeEncoding?: 'offset-binary'; // Omitted: signed two's-complement MIDI encoder
  invert?: boolean;      // Reverse an absolute fader/knob direction
  tempoRange?: number;   // Built-in tempo range as a fraction (0.06 = ±6%)
  loopBeats?: number;    // Dedicated beat-loop button, independent of pad mode
  builtIn?: boolean;      // Generated preset; user overrides are stored separately
}

export interface MidiDevice {
  id: string;
  name: string;
  manufacturer: string;
  connected: boolean;
}

// ============================================================================
// MIDI Service
// ============================================================================

type MidiLearnCallback = (msg: MidiMessage) => void;
type MidiActionHandler = (action: MidiAction, value: number, type: 'press' | 'release' | 'value', mapping: MidiMapping) => void;

// Universal identity request, using the controller's requested device ID (00).
const MIXTRACK_CONNECTION_SYSEX = [0xF0, 0x7E, 0x00, 0x06, 0x01, 0xF7];

function midiPortName(name: string | null): string {
  return (name || '').toLowerCase().replace(/midi\s*(?:in|out)(?:put)?/g, '').replace(/[^a-z0-9]/g, '');
}

class DJMidiService {
  private access: MIDIAccess | null = null;
  private devices: Map<string, MidiDevice> = new Map();
  private mappings: MidiMapping[] = [];
  private actionHandler: MidiActionHandler | null = null;
  private learnCallback: MidiLearnCallback | null = null;
  private learnAction: MidiAction | undefined;
  private enabled = false;
  private listeners: Set<() => void> = new Set();
  private outputConnections = new Map<MIDIOutput, { input: MIDIInput; protocol: 'mixtrack-go'; ready: Promise<void> }>();
  private initializing: Promise<boolean> | null = null;
  private generation = 0;
  private padModeProvider: ((deck: DeckId) => PadMode) | null = null;
  private padModeState: Record<DeckId, PadMode> = { A: 0, B: 0 };
  private feedbackProvider: ((deviceId: string) => readonly MidiLEDMessage[]) | null = null;
  private feedbackLastSent = new Map<MIDIOutput, Map<string, number>>();
  private disabledDefaults = new Set<string>();
  private heldMappings = new Map<string, { msg: MidiMessage; mappings: MidiMapping[]; released: boolean }>();
  private ccValues = new Map<string, number>();

  // ========================================================================
  // Initialization
  // ========================================================================

  async init(): Promise<boolean> {
    if (this.enabled) return true;
    if (this.initializing) return this.initializing;
    const pending = this.initialize(this.generation);
    this.initializing = pending;
    try {
      return await pending;
    } finally {
      if (this.initializing === pending) this.initializing = null;
    }
  }

  private async initialize(generation: number): Promise<boolean> {
    if (!navigator.requestMIDIAccess) {
      console.warn('[DJMidi] Web MIDI API not available');
      return false;
    }

    try {
      this.loadMappings();
      let access: MIDIAccess;
      try {
        access = await navigator.requestMIDIAccess({ sysex: true });
      } catch (err) {
        // Preserve working inbound mappings if SysEx permission is unavailable.
        console.warn('[DJMidi] SysEx access unavailable; enabling input only:', err);
        access = await navigator.requestMIDIAccess({ sysex: false });
      }
      if (generation !== this.generation) return false;
      this.access = access;
      
      // Set up device detection
      this.access.onstatechange = (e) => {
        this.handleDeviceChange(e);
      };
      
      // Enumerate existing devices
      this.scanDevices();
      this.enabled = true;
      this.notifyListeners();
      await this.connectControllerOutputs();
      if (generation !== this.generation) return false;
      
      console.log(`[DJMidi] Initialized with ${this.devices.size} device(s)`);
      return true;
    } catch (err) {
      console.error('[DJMidi] Failed to initialize:', err);
      return false;
    }
  }

  private scanDevices(): void {
    if (!this.access) return;
    
    this.devices.clear();
    
    for (const [id, input] of this.access.inputs) {
      this.devices.set(id, {
        id,
        name: input.name || 'Unknown Device',
        manufacturer: input.manufacturer || 'Unknown',
        connected: input.state === 'connected',
      });
      
      // Attach message handler
      input.onmidimessage = (e) => this.handleMidiMessage(e, id, input.name || 'Unknown');
    }
    this.migrateLegacyPadMappings();
  }

  private migrateLegacyPadMappings(): void {
    for (const mapping of this.mappings) {
      if (mapping.padMode !== undefined || mapping.channel < 4 || mapping.channel > 5
        || mapping.note < 0x14 || mapping.note > 0x17 || mapping.type !== 'noteon') continue;
      const device = this.devices.get(mapping.deviceId);
      if (!device || !isMixtrackGo(device.name)) continue;
      const mode = /^deck[AB]\.hotCue[1-4]$/.test(mapping.action) ? 0 : /^sampler\.pad[1-8]$/.test(mapping.action) ? 2 : undefined;
      if (mode === undefined) continue;
      mapping.deck = mapping.channel === 4 ? 'A' : 'B';
      mapping.padMode = mode;
    }
  }

  private handleDeviceChange(e: MIDIConnectionEvent): void {
    const port = e.port;
    if (!port) return;
    if (port.type === 'output') {
      void this.connectControllerOutputs();
      return;
    }
    
    if (port.state === 'connected') {
      this.devices.set(port.id, {
        id: port.id,
        name: port.name || 'Unknown Device',
        manufacturer: port.manufacturer || 'Unknown',
        connected: true,
      });
      
      const input = this.access?.inputs.get(port.id);
      if (input) input.onmidimessage = (e) => this.handleMidiMessage(e, port.id, port.name || 'Unknown');
      
      console.log(`[DJMidi] Device connected: ${port.name}`);
    } else {
      this.releaseHeldMappings(mapping => mapping.deviceId === port.id);
      for (const [key, held] of this.heldMappings) if (held.msg.deviceId === port.id) this.heldMappings.delete(key);
      const device = this.devices.get(port.id);
      if (device) {
        device.connected = false;
      }
      console.log(`[DJMidi] Device disconnected: ${port.name}`);
    }
    
    this.notifyListeners();
    void this.connectControllerOutputs();
  }

  /** Open the controller output before sending; opening itself emits state events. */
  private async connectControllerOutputs(): Promise<void> {
    const access = this.access;
    if (!access?.sysexEnabled) return;

    for (const [output, connection] of this.outputConnections) {
      if (output.state !== 'connected' || connection.input.state !== 'connected') {
        this.outputConnections.delete(output);
        this.feedbackLastSent.delete(output);
      }
    }

    const inputs = Array.from(access.inputs.values()).filter(input => input.state === 'connected');
    const outputs = Array.from(access.outputs.values()).filter(output => output.state === 'connected');
    for (const output of outputs) {
      const name = midiPortName(output.name);
      const profile = controllerProfileForName(output.name || '');
      if (output.state !== 'connected' || profile?.outputProtocol !== 'mixtrack-go' || this.outputConnections.has(output)) continue;
      // Port IDs are direction-specific. Require one matching name/manufacturer,
      // never choose an arbitrary output or broadcast to unrelated hardware.
      const matches = inputs.filter(input => midiPortName(input.name) === name
        && (!input.manufacturer || !output.manufacturer
          || input.manufacturer.trim().toLowerCase() === output.manufacturer.trim().toLowerCase()));
      if (matches.length !== 1) continue;
      if (outputs.filter(candidate => midiPortName(candidate.name) === name
        && candidate.manufacturer === output.manufacturer).length !== 1) continue;

      if (controllerProfileForName(matches[0].name || '')?.id !== profile.id) continue;
      const connection = { input: matches[0], protocol: 'mixtrack-go' as const, ready: Promise.resolve() };
      this.outputConnections.set(output, connection);
      connection.ready = (async () => {
        try {
          await output.open();
          if (this.access !== access || this.outputConnections.get(output) !== connection
            || output.state !== 'connected' || connection.input.state !== 'connected') return;
          // No timestamp: queue these complete raw SysEx bytes immediately.
          output.send(new Uint8Array(MIXTRACK_CONNECTION_SYSEX));
          // Initialize even when the mapping panel connects before DJ actions mount.
          // Do this directly: waiting on connection.ready here would deadlock.
          this.writePadModeLEDs(output, 'A');
          this.writePadModeLEDs(output, 'B');
          this.writeControlLEDs(output);
          console.log(`[DJMidi] Connection SysEx sent to ${output.name}`);
        } catch (err) {
          // Keep this attempt recorded until disconnect to prevent retry loops
          // from output.open() state events. Input handling remains active.
          console.warn(`[DJMidi] Could not send connection SysEx to ${output.name}:`, err);
        }
      })();
    }
    await Promise.all(Array.from(this.outputConnections.values(), connection => connection.ready));
  }

  // ========================================================================
  // Message Handling
  // ========================================================================

  private handleMidiMessage(e: MIDIMessageEvent, deviceId: string, deviceName: string): void {
    if (!e.data || e.data.length < 2) return;
    
    const status = e.data[0];
    const data1 = e.data[1];
    const data2 = e.data.length > 2 ? e.data[2] : 0;
    
    const channel = status & 0x0f;
    const msgType = status & 0xf0;
    
    let type: MidiMessageType;
    let note = data1;
    let velocity = data2;
    
    switch (msgType) {
      case 0x90: // Note On
        type = velocity > 0 ? 'noteon' : 'noteoff';
        break;
      case 0x80: // Note Off
        type = 'noteoff';
        velocity = 0;
        break;
      case 0xb0: // Control Change
        type = 'cc';
        velocity = data2;
        break;
      case 0xe0: // Pitch Bend
        type = 'pitchbend';
        velocity = (data2 << 7) | data1; // 14-bit value
        break;
      default:
        return; // Ignore other message types
    }
    
    const msg: MidiMessage = { type, channel, note, velocity, deviceId, deviceName };
    if (type === 'cc') this.ccValues.set(`${deviceId}:${channel}:${note}`, velocity);
    
    // MIDI Learn mode - send to learn callback
    if (this.learnCallback) {
      if (msg.type === 'noteoff' || msg.velocity === 0) { this.routeMessage(msg); return; }
      // Keep the hardware Mode button usable while learning a scoped pad.
      if (!this.learnAction?.endsWith('.padMode') && this.getMappings().some(mapping => mapping.action.endsWith('.padMode') && this.matchesMapping(msg, mapping))) {
        this.routeMessage(msg);
        return;
      }
      this.learnCallback(msg);
      return;
    }
    
    // Normal mode - route to mappings
    this.routeMessage(msg);
  }

  private routeMessage(msg: MidiMessage): void {
    if (!this.actionHandler) return;
    const key = `${msg.deviceId}:${msg.channel}:${msg.type === 'noteoff' ? 'noteon' : msg.type}:${msg.note}`;
    const down = msg.type !== 'noteoff' && msg.velocity > 0;
    const held = this.heldMappings.get(key);
    if (held) {
      if (!down) {
        this.heldMappings.delete(key);
        if (!held.released) for (const mapping of held.mappings) this.dispatchMapping(msg, mapping);
      }
      return;
    }
    let candidates = this.getMappings().filter(mapping => this.matchesMapping(msg, mapping));
    candidates = candidates.filter(mapping => mapping.padMode === undefined
      || (mapping.deck !== undefined && mapping.padMode === this.getPadMode(mapping.deck)));
    // Explicit bank overrides win over unscoped legacy bindings and defaults.
    const custom = candidates.filter(mapping => !mapping.builtIn);
    const scoped = custom.filter(mapping => mapping.padMode !== undefined);
    if (scoped.length) candidates = scoped;
    else if (custom.length) candidates = custom;
    else if (candidates.some(mapping => mapping.padMode !== undefined)) candidates = candidates.filter(mapping => mapping.padMode !== undefined);
    const button = msg.type === 'noteon' || msg.type === 'noteoff'
      || (msg.type === 'cc' && candidates.some(mapping => ['toggle', 'momentary', 'trigger'].includes(mapping.valueMode)));
    if (button && down && candidates.length) this.heldMappings.set(key, { msg, mappings: candidates.map(mapping => ({ ...mapping, deviceId: msg.deviceId })), released: false });
    for (const mapping of candidates) this.dispatchMapping(msg, mapping);
  }

  private dispatchMapping(msg: MidiMessage, source: MidiMapping): void {
    if (!this.actionHandler) return;
    const mapping = { ...source, deviceId: msg.deviceId };

      // Performance buttons need both edges even if an older mapping used
      // toggle/trigger. CC > 0 is down; CC 0 and Note Off are release.
      if (/^deck[AB]\.(padMode|performancePad[1-4]|cue|sync|jogTouch)$/.test(mapping.action)
        || (mapping.padMode !== undefined && mapping.action.startsWith('sampler.pad'))) {
        if (msg.type !== 'noteon' && msg.type !== 'noteoff' && msg.type !== 'cc') return;
        const down = msg.type !== 'noteoff' && msg.velocity > 0;
        // Pass the actual input ID for wildcard mappings and output feedback.
        this.actionHandler(mapping.action, msg.velocity / 127, down ? 'press' : 'release', { ...mapping, deviceId: msg.deviceId });
        return;
      }
      
      // Determine value and trigger type
      let value = 0;
      let triggerType: 'press' | 'release' | 'value' = 'value';
      
      switch (mapping.valueMode) {
        case 'toggle':
          if ((msg.type === 'noteon' || msg.type === 'cc') && msg.velocity > 0) {
            triggerType = 'press';
            value = 1;
          } else return;
          break;
          
        case 'momentary':
          triggerType = msg.type !== 'noteoff' && msg.velocity > 0 ? 'press' : 'release';
          value = triggerType === 'press' ? 1 : 0;
          break;
          
        case 'trigger':
          if ((msg.type === 'noteon' || msg.type === 'cc') && msg.velocity > 0) {
            triggerType = 'press';
            value = msg.velocity / 127;
          } else return;
          break;
          
        case 'absolute':
          if (mapping.ccLSB !== undefined && !this.ccValues.has(`${msg.deviceId}:${msg.channel}:${mapping.note}`)) return;
          triggerType = 'value';
          value = mapping.ccLSB === undefined ? msg.velocity / 127
            : (((this.ccValues.get(`${msg.deviceId}:${msg.channel}:${mapping.note}`) ?? 0) << 7)
              | (this.ccValues.get(`${msg.deviceId}:${msg.channel}:${mapping.ccLSB}`) ?? 0)) / 16383;
          if (mapping.invert) value = 1 - value;
          break;
          
        case 'relative': {
          triggerType = 'value';
          // Relative encoding: 1-63 = positive, 65-127 = negative
          const raw = msg.velocity;
          value = mapping.relativeEncoding === 'offset-binary' ? (raw - 64) / 63
            : raw < 64 ? raw / 63 : -(128 - raw) / 63;
          break;
        }
      }
      
      this.actionHandler(mapping.action, value, triggerType, mapping);
  }

  private releaseHeldMappings(matches: (mapping: MidiMapping) => boolean): void {
    for (const held of this.heldMappings.values()) {
      if (held.released || !held.mappings.some(matches)) continue;
      held.released = true;
      for (const mapping of held.mappings) this.dispatchMapping({ ...held.msg, type: held.msg.type === 'cc' ? 'cc' : 'noteoff', velocity: 0 }, mapping);
    }
  }

  private matchesMapping(msg: MidiMessage, mapping: MidiMapping): boolean {
    if (mapping.deviceId !== '*' && mapping.deviceId !== msg.deviceId) return false;
    if (mapping.channel !== -1 && mapping.channel !== msg.channel) return false;
    if (mapping.note !== msg.note && !(msg.type === 'cc' && mapping.ccLSB === msg.note)) return false;
    
    // Type matching with flexibility
    if (mapping.type === 'noteon' || mapping.type === 'noteoff') {
      if (msg.type !== 'noteon' && msg.type !== 'noteoff') return false;
    } else if (mapping.type !== msg.type) {
      return false;
    }
    
    return true;
  }

  // ========================================================================
  // Public API
  // ========================================================================

  /** Let reconnect feedback read the current software-owned modes. */
  setPadModeProvider(provider: ((deck: DeckId) => PadMode) | null): void {
    this.padModeProvider = provider;
    if (!provider) return;
    for (const { input } of this.outputConnections.values()) {
      this.sendPadModeFeedback(input.id, 'A', provider('A'));
      this.sendPadModeFeedback(input.id, 'B', provider('B'));
    }
  }

  sendPadModeFeedback(deviceId: string, deck: DeckId, mode: PadMode): void {
    if (this.padModeState[deck] !== mode) this.releaseHeldMappings(mapping => mapping.deck === deck && mapping.padMode !== undefined);
    this.padModeState[deck] = mode;
    this.notifyListeners();
    for (const [output, connection] of this.outputConnections) {
      if (connection.input.id !== deviceId) continue;
      void connection.ready.then(() => {
        if (this.outputConnections.get(output) !== connection || output.state !== 'connected'
          || connection.input.state !== 'connected' || output.connection !== 'open') return;
        this.writePadModeLEDs(output, deck);
      });
    }
  }

  private writePadModeLEDs(output: MIDIOutput, deck: DeckId): void {
    if (this.outputConnections.get(output)?.protocol !== 'mixtrack-go') return;
    // Protocol reference: josefveiga/Numark-Mixtrack-Go-mapping-for-Mixxx,
    // Numark-Mixtrack-Go.scripts.js, led.mode_* and bright/off values.
    // Deck A/B: channel 5/6; notes 01/02/03/04: cue/loops/sampler/stems.
    const mode = this.padModeProvider?.(deck) ?? this.padModeState[deck];
    const status = deck === 'A' ? 0x94 : 0x95;
    try {
      // Explicitly clear every inactive indicator, then light the selected mode.
      for (let note = 1; note <= 4; note++) {
        if (note !== mode + 1) output.send(new Uint8Array([status, note, 0x00]));
      }
      output.send(new Uint8Array([status, mode + 1, 0x7F]));
    } catch (err) {
      console.warn('[DJMidi] Pad mode LED update failed:', err);
    }
  }

  /** Supply authoritative playback/pad state; unchanged values are not resent. */
  setFeedbackProvider(provider: ((deviceId: string) => readonly MidiLEDMessage[]) | null): void {
    this.feedbackProvider = provider;
    if (!provider) {
      for (const [output, cache] of this.feedbackLastSent) {
        const connection = this.outputConnections.get(output);
        if (output.state !== 'connected' || output.connection !== 'open' || connection?.input.state !== 'connected') continue;
        try {
          for (const key of cache.keys()) {
            const [status, note] = key.split(':').map(Number);
            output.send(new Uint8Array([status, note, 0]));
          }
        } catch (err) { console.warn('[DJMidi] Could not clear controller LEDs:', err); }
      }
      this.feedbackLastSent.clear();
      return;
    }
    this.refreshFeedback();
  }

  refreshFeedback(): void {
    for (const [output, connection] of this.outputConnections) {
      void connection.ready.then(() => {
        if (this.outputConnections.get(output) !== connection || output.state !== 'connected'
          || connection.input.state !== 'connected' || output.connection !== 'open') return;
        this.writeControlLEDs(output);
      });
    }
  }

  private writeControlLEDs(output: MIDIOutput): void {
    if (!this.feedbackProvider || this.outputConnections.get(output)?.protocol !== 'mixtrack-go') return;
    const cache = this.feedbackLastSent.get(output) ?? new Map<string, number>();
    this.feedbackLastSent.set(output, cache);
    try {
      for (const [status, note, value] of this.feedbackProvider(this.outputConnections.get(output)!.input.id)) {
        const key = `${status}:${note}`;
        if (cache.get(key) === value) continue;
        output.send(new Uint8Array([status, note, value]));
        cache.set(key, value); // Failed sends remain eligible for the next refresh.
      }
    } catch (err) {
      console.warn('[DJMidi] Controller LED update failed:', err);
    }
  }

  setActionHandler(handler: MidiActionHandler | null): void {
    if (this.actionHandler && this.actionHandler !== handler) this.releaseHeldMappings(() => true);
    this.actionHandler = handler;
    if (!handler) this.heldMappings.clear();
  }

  /** Start MIDI learn mode - next MIDI message will be passed to callback */
  startLearn(callback: MidiLearnCallback, action?: MidiAction): void {
    this.releaseHeldMappings(() => true);
    this.learnCallback = callback;
    this.learnAction = action;
  }

  /** Cancel MIDI learn mode */
  cancelLearn(): void {
    this.learnCallback = null;
    this.learnAction = undefined;
  }

  isLearning(): boolean {
    return this.learnCallback !== null;
  }

  getDevices(): MidiDevice[] {
    return Array.from(this.devices.values());
  }

  getMappings(): MidiMapping[] {
    const defaults = this.getDefaultMappings().filter(mapping => !this.disabledDefaults.has(mapping.id)
      && !this.mappings.some(custom => (custom.deviceId === '*' || custom.deviceId === mapping.deviceId)
        && (custom.channel === -1 || custom.channel === mapping.channel) && custom.type === mapping.type && custom.note === mapping.note
        && (custom.padMode === undefined || (custom.deck === mapping.deck && custom.padMode === mapping.padMode))));
    return [...this.mappings, ...defaults];
  }

  private getDefaultMappings(): MidiMapping[] {
    return this.getDevices().filter(device => device.connected).flatMap(device => controllerProfileForName(device.name)?.mappings(device.id) ?? []);
  }

  getPadMode(deck: DeckId): PadMode {
    return this.padModeProvider?.(deck) ?? this.padModeState[deck];
  }

  restoreDefaultMappings(): void {
    this.releaseHeldMappings(() => true);
    this.disabledDefaults.clear();
    this.mappings = [];
    this.saveMappings();
    this.notifyListeners();
  }

  addMapping(mapping: MidiMapping): void {
    // Remove existing mapping for the same action
    this.mappings = this.mappings.filter(m => m.id !== mapping.id && !(m.deviceId === mapping.deviceId
      && m.deck === mapping.deck && m.padMode === mapping.padMode
      && (m.action === mapping.action || (m.type === mapping.type && m.channel === mapping.channel && m.note === mapping.note))));
    this.mappings.push({ ...mapping, builtIn: false });
    this.notifyListeners();
  }

  removeMapping(mappingId: string): void {
    this.releaseHeldMappings(mapping => mapping.id === mappingId);
    if (this.getDefaultMappings().some(mapping => mapping.id === mappingId)) this.disabledDefaults.add(mappingId);
    this.mappings = this.mappings.filter(m => m.id !== mappingId);
    this.notifyListeners();
  }

  clearMappings(): void {
    this.releaseHeldMappings(() => true);
    for (const mapping of this.getDefaultMappings()) this.disabledDefaults.add(mapping.id);
    this.mappings = [];
    this.notifyListeners();
  }

  isEnabled(): boolean {
    return this.enabled;
  }

  /** Subscribe to state changes (devices, mappings) */
  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notifyListeners(): void {
    this.listeners.forEach(l => l());
  }

  /** Save mappings to localStorage */
  saveMappings(): void {
    try {
      localStorage.setItem('viib-dj-midi-mappings', JSON.stringify(this.mappings));
      localStorage.setItem('viib-dj-midi-disabled-defaults', JSON.stringify([...this.disabledDefaults]));
    } catch { /* ignore */ }
  }

  /** Load mappings from localStorage */
  loadMappings(): void {
    try {
      const saved = localStorage.getItem('viib-dj-midi-mappings');
      if (saved) {
        const parsed: unknown = JSON.parse(saved);
        if (Array.isArray(parsed)) this.mappings = parsed.filter(mapping => mapping && typeof mapping.id === 'string' && typeof mapping.action === 'string');
      }
      const disabled = JSON.parse(localStorage.getItem('viib-dj-midi-disabled-defaults') || '[]');
      if (Array.isArray(disabled)) this.disabledDefaults = new Set(disabled.filter(id => typeof id === 'string'));
      this.migrateLegacyPadMappings();
    } catch { /* ignore */ }
  }

  destroy(): void {
    this.releaseHeldMappings(() => true);
    this.heldMappings.clear();
    this.ccValues.clear();
    this.setFeedbackProvider(null);
    this.generation++;
    this.initializing = null;
    this.padModeProvider = null;
    this.padModeState = { A: 0, B: 0 };
    this.feedbackProvider = null;
    this.feedbackLastSent.clear();
    this.learnCallback = null;
    this.actionHandler = null;
    this.mappings = [];
    this.devices.clear();
    this.listeners.clear();
    
    if (this.access) {
      this.access.onstatechange = null;
      for (const [, input] of this.access.inputs) {
        input.onmidimessage = null;
      }
    }
    for (const [output, connection] of this.outputConnections) {
      void connection.ready.then(() => output.close()).catch(err => console.warn('[DJMidi] Could not close output:', err));
    }
    this.outputConnections.clear();
    this.access = null;
    this.enabled = false;
  }
}

// Singleton
let midiServiceInstance: DJMidiService | null = null;

export function getDJMidiService(): DJMidiService {
  if (!midiServiceInstance) {
    midiServiceInstance = new DJMidiService();
  }
  return midiServiceInstance;
}

// ============================================================================
// Predefined Actions List (for UI)
// ============================================================================

export const MIDI_ACTION_CATEGORIES: { label: string; actions: { action: MidiAction; label: string; defaultMode: MidiValueMode }[] }[] = [
  {
    label: 'Controller',
    actions: [
      { action: 'headphoneVolume', label: 'Headphone volume', defaultMode: 'absolute' },
      { action: 'library.browse', label: 'Browse library', defaultMode: 'relative' },
      { action: 'library.open', label: 'Open library/search', defaultMode: 'toggle' },
      { action: 'deckA.load', label: 'Load selected track to A', defaultMode: 'toggle' },
      { action: 'deckB.load', label: 'Load selected track to B', defaultMode: 'toggle' },
      { action: 'deckA.filter', label: 'Deck A filter', defaultMode: 'absolute' },
      { action: 'deckB.filter', label: 'Deck B filter', defaultMode: 'absolute' },
      { action: 'deckA.jogTouch', label: 'Deck A jog touch', defaultMode: 'momentary' },
      { action: 'deckB.jogTouch', label: 'Deck B jog touch', defaultMode: 'momentary' },
      { action: 'deckA.stemAcapella', label: 'Deck A acapella', defaultMode: 'toggle' },
      { action: 'deckB.stemAcapella', label: 'Deck B acapella', defaultMode: 'toggle' },
      { action: 'deckA.stemInstrumental', label: 'Deck A instrumental', defaultMode: 'toggle' },
      { action: 'deckB.stemInstrumental', label: 'Deck B instrumental', defaultMode: 'toggle' },
    ],
  },
  {
    label: 'Pad Modes',
    actions: [
      { action: 'deckA.padMode', label: 'Deck A Pad Mode', defaultMode: 'momentary' },
      { action: 'deckA.performancePad1', label: 'Deck A Performance Pad 1', defaultMode: 'momentary' },
      { action: 'deckA.performancePad2', label: 'Deck A Performance Pad 2', defaultMode: 'momentary' },
      { action: 'deckA.performancePad3', label: 'Deck A Performance Pad 3', defaultMode: 'momentary' },
      { action: 'deckA.performancePad4', label: 'Deck A Performance Pad 4', defaultMode: 'momentary' },
      { action: 'deckB.padMode', label: 'Deck B Pad Mode', defaultMode: 'momentary' },
      { action: 'deckB.performancePad1', label: 'Deck B Performance Pad 1', defaultMode: 'momentary' },
      { action: 'deckB.performancePad2', label: 'Deck B Performance Pad 2', defaultMode: 'momentary' },
      { action: 'deckB.performancePad3', label: 'Deck B Performance Pad 3', defaultMode: 'momentary' },
      { action: 'deckB.performancePad4', label: 'Deck B Performance Pad 4', defaultMode: 'momentary' },
    ],
  },
  {
    label: 'Transport',
    actions: [
      { action: 'deckA.play', label: 'Deck A Play/Pause', defaultMode: 'toggle' },
      { action: 'deckB.play', label: 'Deck B Play/Pause', defaultMode: 'toggle' },
      { action: 'deckA.cue', label: 'Deck A Cue', defaultMode: 'momentary' },
      { action: 'deckB.cue', label: 'Deck B Cue', defaultMode: 'momentary' },
      { action: 'deckA.sync', label: 'Deck A Sync', defaultMode: 'toggle' },
      { action: 'deckB.sync', label: 'Deck B Sync', defaultMode: 'toggle' },
    ],
  },
  {
    label: 'Jog Wheels',
    actions: [
      { action: 'deckA.jogWheel', label: 'Deck A Jog Wheel', defaultMode: 'relative' },
      { action: 'deckB.jogWheel', label: 'Deck B Jog Wheel', defaultMode: 'relative' },
    ],
  },
  {
    label: 'Faders & Knobs',
    actions: [
      { action: 'deckA.volume', label: 'Deck A Volume', defaultMode: 'absolute' },
      { action: 'deckB.volume', label: 'Deck B Volume', defaultMode: 'absolute' },
      { action: 'crossfader', label: 'Crossfader', defaultMode: 'absolute' },
      { action: 'masterVolume', label: 'Master Volume', defaultMode: 'absolute' },
      { action: 'deckA.tempo', label: 'Deck A Tempo', defaultMode: 'absolute' },
      { action: 'deckB.tempo', label: 'Deck B Tempo', defaultMode: 'absolute' },
    ],
  },
  {
    label: 'EQ',
    actions: [
      { action: 'deckA.eqHigh', label: 'Deck A EQ High', defaultMode: 'absolute' },
      { action: 'deckA.eqMid', label: 'Deck A EQ Mid', defaultMode: 'absolute' },
      { action: 'deckA.eqLow', label: 'Deck A EQ Low', defaultMode: 'absolute' },
      { action: 'deckB.eqHigh', label: 'Deck B EQ High', defaultMode: 'absolute' },
      { action: 'deckB.eqMid', label: 'Deck B EQ Mid', defaultMode: 'absolute' },
      { action: 'deckB.eqLow', label: 'Deck B EQ Low', defaultMode: 'absolute' },
    ],
  },
  {
    label: 'Hot Cues',
    actions: [
      { action: 'deckA.hotCue1', label: 'Deck A Hot Cue 1', defaultMode: 'trigger' },
      { action: 'deckA.hotCue2', label: 'Deck A Hot Cue 2', defaultMode: 'trigger' },
      { action: 'deckA.hotCue3', label: 'Deck A Hot Cue 3', defaultMode: 'trigger' },
      { action: 'deckA.hotCue4', label: 'Deck A Hot Cue 4', defaultMode: 'trigger' },
      { action: 'deckB.hotCue1', label: 'Deck B Hot Cue 1', defaultMode: 'trigger' },
      { action: 'deckB.hotCue2', label: 'Deck B Hot Cue 2', defaultMode: 'trigger' },
      { action: 'deckB.hotCue3', label: 'Deck B Hot Cue 3', defaultMode: 'trigger' },
      { action: 'deckB.hotCue4', label: 'Deck B Hot Cue 4', defaultMode: 'trigger' },
    ],
  },
  {
    label: 'Sampler',
    actions: [
      { action: 'sampler.pad1', label: 'Sampler Pad 1', defaultMode: 'trigger' },
      { action: 'sampler.pad2', label: 'Sampler Pad 2', defaultMode: 'trigger' },
      { action: 'sampler.pad3', label: 'Sampler Pad 3', defaultMode: 'trigger' },
      { action: 'sampler.pad4', label: 'Sampler Pad 4', defaultMode: 'trigger' },
      { action: 'sampler.pad5', label: 'Sampler Pad 5', defaultMode: 'trigger' },
      { action: 'sampler.pad6', label: 'Sampler Pad 6', defaultMode: 'trigger' },
      { action: 'sampler.pad7', label: 'Sampler Pad 7', defaultMode: 'trigger' },
      { action: 'sampler.pad8', label: 'Sampler Pad 8', defaultMode: 'trigger' },
    ],
  },
  {
    label: 'Loop',
    actions: [
      { action: 'deckA.loopIn', label: 'Deck A Loop In', defaultMode: 'trigger' },
      { action: 'deckA.loopOut', label: 'Deck A Loop Out', defaultMode: 'trigger' },
      { action: 'deckA.loopToggle', label: 'Deck A Loop On/Off', defaultMode: 'toggle' },
      { action: 'deckB.loopIn', label: 'Deck B Loop In', defaultMode: 'trigger' },
      { action: 'deckB.loopOut', label: 'Deck B Loop Out', defaultMode: 'trigger' },
      { action: 'deckB.loopToggle', label: 'Deck B Loop On/Off', defaultMode: 'toggle' },
    ],
  },
  {
    label: 'Headphones',
    actions: [
      { action: 'deckA.headphoneCue', label: 'Deck A Headphone Cue', defaultMode: 'toggle' },
      { action: 'deckB.headphoneCue', label: 'Deck B Headphone Cue', defaultMode: 'toggle' },
    ],
  },
  {
    label: 'FX',
    actions: [
      { action: 'deckA.fxWet', label: 'Deck A FX Wet/Dry', defaultMode: 'absolute' },
      { action: 'deckB.fxWet', label: 'Deck B FX Wet/Dry', defaultMode: 'absolute' },
    ],
  },
];

export { DJMidiService };
