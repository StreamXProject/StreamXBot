import type { ThemeDefinitionInput } from '../tokens'

/** Graphite — monochrome, pure-black OLED surfaces, sharp corners. */
const theme: ThemeDefinitionInput = {
  id: 'graphite-oled',
  name: 'Graphite OLED',
  description: 'Pure black, monochrome accents, crisp edges. Built for OLED.',
  author: 'WebX',
  seed: '#9E9E9E',
  variant: 'monochrome',
  contrast: 0.5,
  preferredMode: 'dark',
  typography: {
    body: "'Manrope', system-ui, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 0.5 },
  effects: { glass: false, ambientBackdrop: false, pureBlack: true, tonalElevation: false },
  builtIn: true,
}

export default theme
