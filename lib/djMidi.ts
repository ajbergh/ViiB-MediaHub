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

export type MidiMessageType = 'noteon' | 'noteoff' | 'cc' | 'pitchbend';

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
type MidiActionHandler = (action: MidiAction, value: number, type: 'press' | 'release' | 'value') => void;

class DJMidiService {
  private access: MIDIAccess | null = null;
  private devices: Map<string, MidiDevice> = new Map();
  private mappings: MidiMapping[] = [];
  private actionHandler: MidiActionHandler | null = null;
  private learnCallback: MidiLearnCallback | null = null;
  private enabled = false;
  private listeners: Set<() => void> = new Set();

  // ========================================================================
  // Initialization
  // ========================================================================

  async init(): Promise<boolean> {
    if (!navigator.requestMIDIAccess) {
      console.warn('[DJMidi] Web MIDI API not available');
      return false;
    }

    try {
      this.access = await navigator.requestMIDIAccess({ sysex: false });
      
      // Set up device detection
      this.access.onstatechange = (e) => {
        this.handleDeviceChange(e);
      };
      
      // Enumerate existing devices
      this.scanDevices();
      this.enabled = true;
      this.notifyListeners();
      
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
  }

  private handleDeviceChange(e: MIDIConnectionEvent): void {
    const port = e.port;
    if (!port || port.type !== 'input') return;
    
    if (port.state === 'connected') {
      this.devices.set(port.id, {
        id: port.id,
        name: port.name || 'Unknown Device',
        manufacturer: port.manufacturer || 'Unknown',
        connected: true,
      });
      
      if (port instanceof MIDIInput) {
        port.onmidimessage = (e) => this.handleMidiMessage(e, port.id, port.name || 'Unknown');
      }
      
      console.log(`[DJMidi] Device connected: ${port.name}`);
    } else {
      const device = this.devices.get(port.id);
      if (device) {
        device.connected = false;
      }
      console.log(`[DJMidi] Device disconnected: ${port.name}`);
    }
    
    this.notifyListeners();
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
    
    // MIDI Learn mode - send to learn callback
    if (this.learnCallback) {
      this.learnCallback(msg);
      return;
    }
    
    // Normal mode - route to mappings
    this.routeMessage(msg);
  }

  private routeMessage(msg: MidiMessage): void {
    if (!this.actionHandler) return;
    
    for (const mapping of this.mappings) {
      if (!this.matchesMapping(msg, mapping)) continue;
      
      // Determine value and trigger type
      let value = 0;
      let triggerType: 'press' | 'release' | 'value' = 'value';
      
      switch (mapping.valueMode) {
        case 'toggle':
          if (msg.type === 'noteon') {
            triggerType = 'press';
            value = 1;
          } else continue;
          break;
          
        case 'momentary':
          triggerType = msg.type === 'noteon' ? 'press' : 'release';
          value = msg.type === 'noteon' ? 1 : 0;
          break;
          
        case 'trigger':
          if (msg.type === 'noteon' && msg.velocity > 0) {
            triggerType = 'press';
            value = msg.velocity / 127;
          } else continue;
          break;
          
        case 'absolute':
          triggerType = 'value';
          value = msg.velocity / 127;
          break;
          
        case 'relative': {
          triggerType = 'value';
          // Relative encoding: 1-63 = positive, 65-127 = negative
          const raw = msg.velocity;
          value = raw < 64 ? raw / 63 : -(128 - raw) / 63;
          break;
        }
      }
      
      this.actionHandler(mapping.action, value, triggerType);
    }
  }

  private matchesMapping(msg: MidiMessage, mapping: MidiMapping): boolean {
    if (mapping.deviceId !== '*' && mapping.deviceId !== msg.deviceId) return false;
    if (mapping.channel !== -1 && mapping.channel !== msg.channel) return false;
    if (mapping.note !== msg.note) return false;
    
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

  setActionHandler(handler: MidiActionHandler): void {
    this.actionHandler = handler;
  }

  /** Start MIDI learn mode - next MIDI message will be passed to callback */
  startLearn(callback: MidiLearnCallback): void {
    this.learnCallback = callback;
  }

  /** Cancel MIDI learn mode */
  cancelLearn(): void {
    this.learnCallback = null;
  }

  isLearning(): boolean {
    return this.learnCallback !== null;
  }

  getDevices(): MidiDevice[] {
    return Array.from(this.devices.values());
  }

  getMappings(): MidiMapping[] {
    return [...this.mappings];
  }

  addMapping(mapping: MidiMapping): void {
    // Remove existing mapping for the same action
    this.mappings = this.mappings.filter(m => m.id !== mapping.id);
    this.mappings.push(mapping);
    this.notifyListeners();
  }

  removeMapping(mappingId: string): void {
    this.mappings = this.mappings.filter(m => m.id !== mappingId);
    this.notifyListeners();
  }

  clearMappings(): void {
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
    } catch { /* ignore */ }
  }

  /** Load mappings from localStorage */
  loadMappings(): void {
    try {
      const saved = localStorage.getItem('viib-dj-midi-mappings');
      if (saved) {
        this.mappings = JSON.parse(saved);
      }
    } catch { /* ignore */ }
  }

  destroy(): void {
    this.learnCallback = null;
    this.actionHandler = null;
    this.mappings = [];
    this.devices.clear();
    this.listeners.clear();
    
    if (this.access) {
      for (const [, input] of this.access.inputs) {
        input.onmidimessage = null;
      }
    }
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
