import type { ThemeDefinitionInput } from '../tokens'

/** Paper — a light theme with a soft plum accent. */
const theme: ThemeDefinitionInput = {
  id: 'paper',
  name: 'Paper',
  description: 'Bright, warm light surfaces with a soft plum accent.',
  author: 'WebX',
  seed: '#8E4585',
  variant: 'tonalSpot',
  preferredMode: 'light',
  typography: {
    body: "'Manrope', system-ui, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 1 },
  effects: { glass: true, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
