import { checkStyleRatchet } from './style-ratchet.mjs';
await checkStyleRatchet({
 name: 'Raw colors', baselineFile: 'raw-color-baseline.json', extensions: ['.ts','.tsx','.css'],
 pattern: /#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b/g,
});
