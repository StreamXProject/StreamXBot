/**
 * Scheme generation: seed color → full MD3 color scheme using the official
 * Material Color Utilities (HCT color space, tonal palettes, contrast curves).
 */
import {
  Hct,
  argbFromHex,
  hexFromArgb,
  DynamicScheme,
  SchemeTonalSpot,
  SchemeVibrant,
  SchemeExpressive,
  SchemeNeutral,
  SchemeMonochrome,
  SchemeFidelity,
  SchemeContent,
  SchemeRainbow,
  SchemeFruitSalad,
  TonalPalette,
  sourceColorFromImage,
} from '@material/material-color-utilities'
import { COLOR_ROLES, type ColorScheme, type ResolvedTheme, type SchemeVariant, type ThemeDefinition } from './tokens'

const VARIANT_CTORS: Record<SchemeVariant, new (hct: Hct, isDark: boolean, contrast: number) => DynamicScheme> = {
  tonalSpot: SchemeTonalSpot,
  vibrant: SchemeVibrant,
  expressive: SchemeExpressive,
  neutral: SchemeNeutral,
  monochrome: SchemeMonochrome,
  fidelity: SchemeFidelity,
  content: SchemeContent,
  rainbow: SchemeRainbow,
  fruitSalad: SchemeFruitSalad,
}

function buildScheme(def: ThemeDefinition, isDark: boolean): DynamicScheme {
  const Ctor = VARIANT_CTORS[def.variant] ?? SchemeTonalSpot
  const seedHct = Hct.fromInt(argbFromHex(def.seed))
  let scheme: DynamicScheme = new Ctor(seedHct, isDark, def.contrast)

  if (def.secondarySeed || def.tertiarySeed) {
    scheme = new DynamicScheme({
      sourceColorHct: seedHct,
      variant: scheme.variant,
      isDark,
      contrastLevel: def.contrast,
      primaryPalette: scheme.primaryPalette,
      secondaryPalette: def.secondarySeed
        ? TonalPalette.fromInt(argbFromHex(def.secondarySeed))
        : scheme.secondaryPalette,
      tertiaryPalette: def.tertiarySeed
        ? TonalPalette.fromInt(argbFromHex(def.tertiarySeed))
        : scheme.tertiaryPalette,
      neutralPalette: scheme.neutralPalette,
      neutralVariantPalette: scheme.neutralVariantPalette,
      errorPalette: scheme.errorPalette,
    })
  }
  return scheme
}

export function schemeToColors(scheme: DynamicScheme): ColorScheme {
  const out = {} as ColorScheme
  for (const role of COLOR_ROLES) {
    const argb = (scheme as unknown as Record<string, number>)[role]
    out[role] = hexFromArgb(argb).toUpperCase()
  }
  return out
}

function applyOverrides(colors: ColorScheme, overrides?: Record<string, string>): ColorScheme {
  if (!overrides) return colors
  const next = { ...colors }
  for (const [k, v] of Object.entries(overrides)) {
    if (k in next) (next as Record<string, string>)[k] = v.toUpperCase()
  }
  return next
}

function pureBlack(colors: ColorScheme): ColorScheme {
  return {
    ...colors,
    background: '#000000',
    surface: '#000000',
    surfaceDim: '#000000',
    surfaceContainerLowest: '#000000',
    surfaceContainerLow: '#0A0A0A',
    surfaceContainer: '#111111',
    surfaceContainerHigh: '#1A1A1A',
    surfaceContainerHighest: '#242424',
    surfaceBright: '#2C2C2C',
  }
}

const cache = new Map<string, ResolvedTheme>()

export function resolveTheme(def: ThemeDefinition): ResolvedTheme {
  const key = JSON.stringify(def)
  const hit = cache.get(key)
  if (hit) return hit

  let dark = schemeToColors(buildScheme(def, true))
  let light = schemeToColors(buildScheme(def, false))
  if (def.effects.pureBlack) dark = pureBlack(dark)
  dark = applyOverrides(dark, def.overrides?.dark)
  light = applyOverrides(light, def.overrides?.light)

  const resolved = { definition: def, dark, light }
  if (cache.size > 50) cache.clear()
  cache.set(key, resolved)
  return resolved
}

/** Extract a seed color from an image URL (used for "Material You" from artwork) */
export async function seedFromImage(url: string): Promise<string | null> {
  if (typeof window === 'undefined' || !url) return null
  try {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.decoding = 'async'
    const loaded = new Promise<void>((res, rej) => {
      img.onload = () => res()
      img.onerror = () => rej(new Error('image failed'))
    })
    img.src = url
    await loaded
    const argb = await sourceColorFromImage(img)
    return hexFromArgb(argb).toUpperCase()
  } catch {
    return null
  }
}

export function isValidHex(v: string): boolean {
  return /^#([0-9a-fA-F]{6})$/.test(v)
}

/** Simple relative luminance helper (0..1) */
export function luminance(hex: string): number {
  const n = parseInt(hex.slice(1, 7), 16)
  const r = (n >> 16) & 255
  const g = (n >> 8) & 255
  const b = n & 255
  const f = (c: number) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}

export function hexToRgbTriplet(hex: string): string {
  const n = parseInt(hex.slice(1, 7), 16)
  return `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`
}
