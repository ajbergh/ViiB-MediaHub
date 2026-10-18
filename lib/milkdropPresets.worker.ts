/** Keep the extra packs' download, parsing and preparation off the UI thread. */
import extra from 'butterchurn-presets/lib/butterchurnPresetsExtra.min';
import extra2 from 'butterchurn-presets/lib/butterchurnPresetsExtra2.min';
import md1 from 'butterchurn-presets/lib/butterchurnPresetsMD1.min';

self.postMessage({ ...md1.getPresets(), ...extra2.getPresets(), ...extra.getPresets() });
