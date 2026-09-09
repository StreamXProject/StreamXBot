import type { ThemeDefinitionInput } from '../tokens'

/**
 * WebX — the default identity. Crimson seed, deep tinted surfaces,
 * Manrope for a confident, slightly technical voice.
 */
const theme: ThemeDefinitionInput = {
  id: 'webx-crimson',
  name: 'WebX',
  description: 'Signature crimson accent on deep, red-tinted surfaces.',
  author: 'WebX',
  seed: '#FA2D48',
  variant: 'vibrant',
  preferredMode: 'dark',
  typography: {
    body: "'Manrope', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 1 },
  effects: { glass: true, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
