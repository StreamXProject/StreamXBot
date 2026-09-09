import type { ThemeDefinitionInput } from '../tokens'

/** Moss — muted greens, low contrast, calm. */
const theme: ThemeDefinitionInput = {
  id: 'moss',
  name: 'Moss',
  description: 'Muted forest greens and warm neutrals. Easy on the eyes.',
  author: 'WebX',
  seed: '#4C8C4A',
  variant: 'neutral',
  preferredMode: 'dark',
  typography: {
    body: "'Manrope', system-ui, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 0.75 },
  effects: { glass: false, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
