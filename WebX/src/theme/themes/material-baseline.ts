import type { ThemeDefinitionInput } from '../tokens'

/** Material 3 baseline — the reference purple scheme with Roboto Flex. */
const theme: ThemeDefinitionInput = {
  id: 'material-baseline',
  name: 'Material Baseline',
  description: 'The reference Material 3 scheme. Roboto Flex, standard shapes.',
  author: 'Material Design',
  seed: '#6750A4',
  variant: 'tonalSpot',
  preferredMode: 'system',
  typography: {
    body: "'Roboto Flex', Roboto, system-ui, sans-serif",
    googleFonts: ['Roboto+Flex:wght@300..800'],
  },
  shape: { scale: 1 },
  effects: { glass: false, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
