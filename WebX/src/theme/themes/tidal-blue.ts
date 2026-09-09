import type { ThemeDefinitionInput } from '../tokens'

/** Tidal — cool, deep blue with a hint of cyan tertiary. */
const theme: ThemeDefinitionInput = {
  id: 'tidal-blue',
  name: 'Tidal',
  description: 'Cool oceanic blues with a bright cyan counterpoint.',
  author: 'WebX',
  seed: '#0A84FF',
  tertiarySeed: '#30D5C8',
  variant: 'tonalSpot',
  preferredMode: 'dark',
  typography: {
    body: "'Manrope', system-ui, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 1.25 },
  effects: { glass: true, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
