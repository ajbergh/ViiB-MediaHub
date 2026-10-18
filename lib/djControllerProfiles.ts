/** Controller recognition is conservative: related models do not share presets implicitly. */
import type { MidiMapping } from './djMidi';
import { isMixtrackGo, mixtrackGoMappings } from './mixtrackGoProfile';
import { experimentalMidiMappings, type ExperimentalControllerId } from './experimentalMidiProfiles';

export interface DJControllerProfile {
  id: string;
  label: string;
  experimental: boolean;
  outputProtocol?: 'mixtrack-go';
  matches: (name: string) => boolean;
  mappings: (deviceId: string) => MidiMapping[];
}
const normalize = (name: string) => name.toLowerCase().replace(/[^a-z0-9]/g, '');
const experimental = (id: ExperimentalControllerId, label: string, pattern: RegExp): DJControllerProfile => ({
  id, label, experimental: true, matches: name => pattern.test(normalize(name)),
  mappings: deviceId => experimentalMidiMappings(id, label, deviceId),
});
export const DJ_CONTROLLER_PROFILES: readonly DJControllerProfile[] = [
  { id: 'mixtrack-go', label: 'Numark MixTrack Go', experimental: false, outputProtocol: 'mixtrack-go', matches: isMixtrackGo, mappings: mixtrackGoMappings },
  experimental('numark-dj2go', 'Numark DJ2GO', /^(?:numark)?dj2go(?:midi\d+)?$/),
  experimental('behringer-cmd-micro', 'Behringer CMD Micro', /^(?:behringer)?cmdmicro(?:midi\d+)?$/),
  experimental('hercules-djcontrol-compact', 'Hercules DJControl Compact', /^(?:hercules)?djcontrolcompact(?:midi\d+)?$/),
  experimental('pioneer-ddj-400', 'Pioneer DDJ-400', /^(?:pioneer(?:dj)?)?ddj400(?:midi\d+)?$/),
];
export function controllerProfileForName(name: string): DJControllerProfile | undefined {
  return DJ_CONTROLLER_PROFILES.find(profile => profile.matches(name));
}
