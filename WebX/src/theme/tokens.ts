/**
 * WebX Theme Engine — MD3 token model
 *
 * A theme is a small declarative object. The engine expands it into the full
 * Material Design 3 color scheme (light + dark), typography, shape and motion
 * tokens and writes them to CSS custom properties (`--md-sys-*`).
 *
 * Themes are modular: drop a file in `src/theme/themes/` that default-exports a
 * `ThemeDefinition` and it is auto-registered (see registry.ts). Users can also
 * import/export themes as JSON at runtime (validated with zod).
 */
import { z } from 'zod'

export const COLOR_ROLES = [
  'primary',
  'onPrimary',
  'primaryContainer',
  'onPrimaryContainer',
  'primaryFixed',
  'primaryFixedDim',
  'onPrimaryFixed',
  'onPrimaryFixedVariant',
  'inversePrimary',
  'secondary',
  'onSecondary',
  'secondaryContainer',
  'onSecondaryContainer',
  'secondaryFixed',
  'secondaryFixedDim',
  'onSecondaryFixed',
  'onSecondaryFixedVariant',
  'tertiary',
  'onTertiary',
  'tertiaryContainer',
  'onTertiaryContainer',
  'tertiaryFixed',
  'tertiaryFixedDim',
  'onTertiaryFixed',
  'onTertiaryFixedVariant',
  'error',
  'onError',
  'errorContainer',
  'onErrorContainer',
  'background',
  'onBackground',
  'surface',
  'onSurface',
  'surfaceVariant',
  'onSurfaceVariant',
  'surfaceDim',
  'surfaceBright',
  'surfaceContainerLowest',
  'surfaceContainerLow',
  'surfaceContainer',
  'surfaceContainerHigh',
  'surfaceContainerHighest',
  'surfaceTint',
  'inverseSurface',
  'inverseOnSurface',
  'outline',
  'outlineVariant',
  'shadow',
  'scrim',
] as const

export type ColorRole = (typeof COLOR_ROLES)[number]
export type ColorScheme = Record<ColorRole, string>

export type SchemeVariant =
  | 'tonalSpot'
  | 'vibrant'
  | 'expressive'
  | 'neutral'
  | 'monochrome'
  | 'fidelity'
  | 'content'
  | 'rainbow'
  | 'fruitSalad'

export type ThemeMode = 'light' | 'dark' | 'system'

export const HEX = /^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/

const HexSchema = z.string().regex(HEX, 'Expected a hex color like #FA2D48')

export const ThemeTypographySchema = z.object({
  /** Font stack for body / UI text */
  body: z.string().min(1),
  /** Font stack for headlines and display text (defaults to body) */
  headline: z.string().min(1).optional(),
  /** Google Fonts family list to load at runtime, e.g. ["Manrope:wght@300..800"] */
  googleFonts: z.array(z.string()).optional(),
})

export const ThemeShapeSchema = z.object({
  /** Multiplier applied to the MD3 shape scale. 0 = sharp, 1 = MD3 default, 1.5 = extra round */
  scale: z.number().min(0).max(2).default(1),
})

export const ThemeEffectsSchema = z.object({
  /** Frosted-glass surfaces (backdrop blur) on app bars & sheets */
  glass: z.boolean().default(true),
  /** Ambient artwork backdrop in the full player */
  ambientBackdrop: z.boolean().default(true),
  /** Pure #000 surfaces in dark mode (OLED) */
  pureBlack: z.boolean().default(false),
  /** Show tonal elevation (surface tint) on elevated surfaces */
  tonalElevation: z.boolean().default(true),
})

export const ThemeDefinitionSchema = z.object({
  /** Unique id, kebab-case */
  id: z.string().min(1).regex(/^[a-z0-9-]+$/),
  name: z.string().min(1),
  description: z.string().optional(),
  author: z.string().optional(),
  version: z.number().int().positive().default(1),
  /** Seed color from which the tonal palettes are derived */
  seed: HexSchema,
  /** Optional additional key colors */
  secondarySeed: HexSchema.optional(),
  tertiarySeed: HexSchema.optional(),
  variant: z
    .enum(['tonalSpot', 'vibrant', 'expressive', 'neutral', 'monochrome', 'fidelity', 'content', 'rainbow', 'fruitSalad'])
    .default('tonalSpot'),
  /** -1 (reduced) … 0 (standard) … 1 (high contrast) */
  contrast: z.number().min(-1).max(1).default(0),
  /** Which mode the theme is designed for. 'system' follows the OS */
  preferredMode: z.enum(['light', 'dark', 'system']).default('dark'),
  typography: ThemeTypographySchema,
  shape: ThemeShapeSchema.default({ scale: 1 }),
  effects: ThemeEffectsSchema.default({
    glass: true,
    ambientBackdrop: true,
    pureBlack: false,
    tonalElevation: true,
  }),
  /**
   * Hard overrides applied after scheme generation. Keys are color roles,
   * values hex. `dark` and `light` are applied for the respective mode only.
   */
  overrides: z
    .object({
      dark: z.record(z.string(), HexSchema).optional(),
      light: z.record(z.string(), HexSchema).optional(),
    })
    .optional(),
  /** Marks themes shipped with the app; user themes are `false` */
  builtIn: z.boolean().default(false),
})

export type ThemeDefinition = z.infer<typeof ThemeDefinitionSchema>
export type ThemeDefinitionInput = z.input<typeof ThemeDefinitionSchema>

/** Fully resolved runtime theme: both schemes computed, ready to apply */
export interface ResolvedTheme {
  definition: ThemeDefinition
  light: ColorScheme
  dark: ColorScheme
}

/** MD3 shape scale in px at scale = 1 */
export const SHAPE_BASE = {
  none: 0,
  extraSmall: 4,
  small: 8,
  medium: 12,
  large: 16,
  largeIncreased: 20,
  extraLarge: 28,
  extraLargeIncreased: 32,
  extraExtraLarge: 48,
} as const

export type ShapeToken = keyof typeof SHAPE_BASE

export const kebab = (s: string) => s.replace(/[A-Z]/g, (m) => '-' + m.toLowerCase())
