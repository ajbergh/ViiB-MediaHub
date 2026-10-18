import { describe, expect, it } from 'vitest';
import { controllerProfileForName, DJ_CONTROLLER_PROFILES } from './djControllerProfiles';

describe('documented controller profiles', () => {
  it.each([
    ['Numark DJ2Go MIDI 1', 'numark-dj2go'], ['CMD MICRO', 'behringer-cmd-micro'],
    ['Hercules DJControl Compact', 'hercules-djcontrol-compact'], ['DDJ-400', 'pioneer-ddj-400'],
  ])('recognizes %s with device-scoped, stable mappings', (name, id) => {
    const profile = controllerProfileForName(name)!;
    expect(profile.id).toBe(id);
    expect(profile.experimental).toBe(true);
    expect(profile.outputProtocol).toBeUndefined();
    const mappings = profile.mappings('input-a');
    expect(mappings.length).toBeGreaterThan(15);
    expect(new Set(mappings.map(mapping => mapping.id)).size).toBe(mappings.length);
    expect(mappings).toEqual(profile.mappings('input-a'));
    expect(mappings.every(mapping => mapping.deviceId === 'input-a' && mapping.builtIn)).toBe(true);
    expect(profile.mappings('input-b').some(mapping => mappings.some(existing => existing.id === mapping.id))).toBe(false);
    const messages = mappings.map(mapping => `${mapping.type}:${mapping.channel}:${mapping.note}`);
    expect(new Set(messages).size).toBe(messages.length);
    for (const mapping of mappings) {
      expect(mapping.channel).toBeGreaterThanOrEqual(0); expect(mapping.channel).toBeLessThan(16);
      expect(mapping.note).toBeGreaterThanOrEqual(0); expect(mapping.note).toBeLessThan(128);
    }
  });
  it.each(['Numark DJ2GO2 Touch', 'DDJ-FLX4', 'DDJ-4000', 'CMD', 'DJControl', 'Generic MIDI Keyboard'])('does not guess a related/ambiguous model: %s', name => {
    expect(controllerProfileForName(name)).toBeUndefined();
  });
  it('declares output capability only for the existing MixTrack Go protocol', () => {
    expect(DJ_CONTROLLER_PROFILES.filter(profile => profile.outputProtocol).map(profile => profile.id)).toEqual(['mixtrack-go']);
  });
});
