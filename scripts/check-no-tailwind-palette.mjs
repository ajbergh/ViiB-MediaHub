/** Runs the style ratchet against direct Tailwind palette classes. */

import { checkStyleRatchet } from './style-ratchet.mjs';
await checkStyleRatchet({
 name: 'Palette', baselineFile: 'palette-baseline.json', extensions: ['.ts','.tsx','.js','.jsx','.css'],
 pattern: /\b(?:bg|text|border|ring|stroke|fill|from|to|via|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}(?:\/\d{1,3})?\b/g,
});
