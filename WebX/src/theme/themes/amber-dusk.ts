import type { ThemeDefinitionInput } from '../tokens'

/** Amber Dusk — warm amber with expressive, shifted secondary hues. */
const theme: ThemeDefinitionInput = {
  id: 'amber-dusk',
  name: 'Amber Dusk',
  description: 'Warm amber glow with expressive complementary hues.',
  author: 'WebX',
  seed: '#FFB300',
  variant: 'expressive',
  preferredMode: 'dark',
  typography: {
    body: "'Manrope', system-ui, sans-serif",
    googleFonts: ['Manrope:wght@300..800'],
  },
  shape: { scale: 1.5 },
  effects: { glass: true, ambientBackdrop: true, pureBlack: false, tonalElevation: true },
  builtIn: true,
}

export default theme
