/**
 * ViiB MediaHub - DJ MIDI Mapping Component
 * 
 * UI for configuring MIDI controller mappings.
 * Features MIDI learn mode, device detection, and mapping management.
 * 
 * @module components/dj/v2/DJMidiMapping
 */

import React, { useState, useEffect, useCallback, memo } from 'react';
import { 
  getDJMidiService, 
  MIDI_ACTION_CATEGORIES,
  type MidiDevice,
  type MidiMapping,
  type MidiMessage,
  type MidiAction,
  type MidiValueMode,
} from '../../../lib/djMidi';

// ============================================================================
// MIDI Mapping Panel
// ============================================================================

export const DJMidiMapping: React.FC<{ onClose: () => void }> = memo(({ onClose }) => {
  const midi = getDJMidiService();
  
  const [initialized, setInitialized] = useState(midi.isEnabled());
  const [devices, setDevices] = useState<MidiDevice[]>(midi.getDevices());
  const [mappings, setMappings] = useState<MidiMapping[]>(midi.getMappings());
  const [learning, setLearning] = useState(false);
  const [learningAction, setLearningAction] = useState<MidiAction | null>(null);
  const [lastMessage, setLastMessage] = useState<MidiMessage | null>(null);
  const [selectedCategory, setSelectedCategory] = useState(0);
  
  // Subscribe to MIDI service changes
  useEffect(() => {
    const unsub = midi.subscribe(() => {
      setDevices(midi.getDevices());
      setMappings(midi.getMappings());
    });
    return unsub;
  }, [midi]);
  
  // Initialize MIDI
  const handleInit = useCallback(async () => {
    const ok = await midi.init();
    if (ok) {
      midi.loadMappings();
      setInitialized(true);
      setDevices(midi.getDevices());
      setMappings(midi.getMappings());
    }
  }, [midi]);
  
  // Start MIDI learn for a specific action
  const handleStartLearn = useCallback((action: MidiAction) => {
    setLearningAction(action);
    setLearning(true);
    setLastMessage(null);
    
    midi.startLearn((msg: MidiMessage) => {
      setLastMessage(msg);
      
      // Find default mode for this action
      let defaultMode: MidiValueMode = 'toggle';
      for (const cat of MIDI_ACTION_CATEGORIES) {
        const found = cat.actions.find(a => a.action === action);
        if (found) {
          defaultMode = found.defaultMode;
          break;
        }
      }
      
      // Create mapping
      const mapping: MidiMapping = {
        id: `${action}-${msg.deviceId}-${msg.channel}-${msg.note}`,
        deviceId: msg.deviceId,
        type: msg.type === 'noteoff' ? 'noteon' : msg.type,
        channel: msg.channel,
        note: msg.note,
        action,
        valueMode: defaultMode,
        label: `${msg.deviceName} Ch${msg.channel + 1} ${msg.type === 'cc' ? 'CC' : 'Note'}${msg.note}`,
      };
      
      midi.addMapping(mapping);
      midi.saveMappings();
      midi.cancelLearn();
      setLearning(false);
      setLearningAction(null);
    });
  }, [midi]);
  
  const handleCancelLearn = useCallback(() => {
    midi.cancelLearn();
    setLearning(false);
    setLearningAction(null);
  }, [midi]);
  
  const handleRemoveMapping = useCallback((mappingId: string) => {
    midi.removeMapping(mappingId);
    midi.saveMappings();
  }, [midi]);
  
  const handleClearAll = useCallback(() => {
    midi.clearMappings();
    midi.saveMappings();
  }, [midi]);
  
  // Get mapping for a specific action
  const getMappingForAction = (action: MidiAction): MidiMapping | undefined => {
    return mappings.find(m => m.action === action);
  };
  
  const category = MIDI_ACTION_CATEGORIES[selectedCategory];
  
  return (
    <div className="fixed inset-0 z-[100] bg-black/60 flex items-center justify-center" onClick={onClose}>
      <div 
        className="bg-[#1a1a1a] border border-[#333] rounded-lg w-[600px] max-h-[80vh] flex flex-col shadow-2xl"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-[#333]">
          <div className="flex items-center gap-2">
            <span className="text-sm font-bold text-neutral-200">🎹 MIDI Controller Mapping</span>
            {initialized && (
              <span className="text-[9px] bg-green-600/30 text-green-400 px-1.5 py-0.5 rounded">
                {devices.filter(d => d.connected).length} device(s)
              </span>
            )}
          </div>
          <button onClick={onClose} className="text-neutral-500 hover:text-neutral-300 text-lg">&times;</button>
        </div>
        
        {/* Content */}
        <div className="flex-1 overflow-hidden flex flex-col">
          {!initialized ? (
            /* Init prompt */
            <div className="flex flex-col items-center justify-center p-8 gap-4">
              <p className="text-neutral-400 text-sm text-center">
                Connect a MIDI controller and enable Web MIDI access to start mapping controls.
              </p>
              <button
                onClick={handleInit}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-md text-sm font-medium transition-colors"
              >
                Enable MIDI
              </button>
              <p className="text-[10px] text-neutral-600 text-center">
                Your browser will ask for permission to access MIDI devices.
              </p>
            </div>
          ) : (
            <>
              {/* Device list */}
              <div className="px-4 py-2 bg-[#161616] border-b border-[#2a2a2a]">
                <div className="text-[9px] text-neutral-500 uppercase tracking-wider mb-1">Connected Devices</div>
                {devices.length === 0 ? (
                  <div className="text-[10px] text-neutral-600">No MIDI devices detected</div>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {devices.map(d => (
                      <div 
                        key={d.id} 
                        className={`text-[10px] px-2 py-1 rounded ${
                          d.connected 
                            ? 'bg-green-600/15 text-green-400 border border-green-500/30' 
                            : 'bg-neutral-800 text-neutral-600 border border-neutral-700'
                        }`}
                      >
                        {d.name}
                      </div>
                    ))}
                  </div>
                )}
              </div>
              
              {/* MIDI Learn overlay */}
              {learning && (
                <div className="px-4 py-3 bg-yellow-600/10 border-b border-yellow-500/30 flex items-center justify-between">
                  <div>
                    <div className="text-sm font-bold text-yellow-400 animate-pulse">
                      MIDI Learn Active
                    </div>
                    <div className="text-[10px] text-yellow-400/70">
                      Move a control or press a button on your MIDI controller for: <strong>{learningAction}</strong>
                    </div>
                    {lastMessage && (
                      <div className="text-[9px] text-neutral-500 mt-1">
                        Last: {lastMessage.deviceName} Ch{lastMessage.channel + 1} {lastMessage.type} #{lastMessage.note} vel={lastMessage.velocity}
                      </div>
                    )}
                  </div>
                  <button 
                    onClick={handleCancelLearn}
                    className="px-2 py-1 text-[10px] bg-neutral-700 hover:bg-neutral-600 text-neutral-300 rounded"
                  >
                    Cancel
                  </button>
                </div>
              )}
              
              {/* Category tabs + mapping table */}
              <div className="flex-1 overflow-hidden flex">
                {/* Category sidebar */}
                <div className="w-32 bg-[#141414] border-r border-[#2a2a2a] overflow-y-auto">
                  {MIDI_ACTION_CATEGORIES.map((cat, i) => (
                    <button
                      key={cat.label}
                      onClick={() => setSelectedCategory(i)}
                      className={`w-full text-left px-3 py-2 text-[10px] font-medium transition-colors ${
                        selectedCategory === i
                          ? 'bg-blue-600/20 text-blue-400 border-l-2 border-blue-500'
                          : 'text-neutral-500 hover:text-neutral-300 hover:bg-[#1a1a1a] border-l-2 border-transparent'
                      }`}
                    >
                      {cat.label}
                    </button>
                  ))}
                </div>
                
                {/* Mapping list */}
                <div className="flex-1 overflow-y-auto">
                  <table className="w-full">
                    <thead className="sticky top-0 bg-[#1a1a1a]">
                      <tr className="text-[9px] text-neutral-500 uppercase tracking-wider">
                        <th className="text-left px-3 py-2">Action</th>
                        <th className="text-left px-3 py-2">Mapping</th>
                        <th className="text-center px-3 py-2 w-20">Mode</th>
                        <th className="text-center px-3 py-2 w-16"></th>
                      </tr>
                    </thead>
                    <tbody>
                      {category?.actions.map(({ action, label }) => {
                        const mapping = getMappingForAction(action);
                        return (
                          <tr key={action} className="border-t border-[#222] hover:bg-[#1e1e1e]">
                            <td className="px-3 py-1.5 text-[11px] text-neutral-300">{label}</td>
                            <td className="px-3 py-1.5">
                              {mapping ? (
                                <span className="text-[10px] text-cyan-400 font-mono bg-cyan-600/10 px-1.5 py-0.5 rounded">
                                  {mapping.label || `Ch${mapping.channel + 1} ${mapping.type === 'cc' ? 'CC' : 'N'}${mapping.note}`}
                                </span>
                              ) : (
                                <span className="text-[10px] text-neutral-600 italic">Not mapped</span>
                              )}
                            </td>
                            <td className="px-3 py-1.5 text-center">
                              {mapping && (
                                <span className="text-[9px] text-neutral-500 bg-neutral-800 px-1 py-0.5 rounded">
                                  {mapping.valueMode}
                                </span>
                              )}
                            </td>
                            <td className="px-3 py-1.5 text-center">
                              <div className="flex gap-1 justify-center">
                                <button
                                  onClick={() => handleStartLearn(action)}
                                  disabled={learning}
                                  className={`text-[9px] px-1.5 py-0.5 rounded transition-colors ${
                                    learningAction === action
                                      ? 'bg-yellow-600/30 text-yellow-400 animate-pulse'
                                      : 'bg-neutral-800 text-neutral-400 hover:bg-neutral-700 hover:text-neutral-200'
                                  } ${learning && learningAction !== action ? 'opacity-30 cursor-not-allowed' : ''}`}
                                >
                                  Learn
                                </button>
                                {mapping && (
                                  <button
                                    onClick={() => handleRemoveMapping(mapping.id)}
                                    className="text-[9px] px-1 py-0.5 rounded bg-red-900/20 text-red-400/70 hover:bg-red-900/40 hover:text-red-300"
                                  >
                                    ×
                                  </button>
                                )}
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            </>
          )}
        </div>
        
        {/* Footer */}
        {initialized && (
          <div className="flex items-center justify-between px-4 py-2 border-t border-[#333]">
            <span className="text-[9px] text-neutral-600">
              {mappings.length} mapping(s) configured
            </span>
            <button
              onClick={handleClearAll}
              className="text-[9px] text-red-400/60 hover:text-red-400 transition-colors"
            >
              Clear All Mappings
            </button>
          </div>
        )}
      </div>
    </div>
  );
});
DJMidiMapping.displayName = 'DJMidiMapping';

export default DJMidiMapping;
