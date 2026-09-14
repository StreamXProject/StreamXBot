/**
 * Writes a resolved theme to the document as `--md-sys-*` CSS custom properties.
 * Everything visual in the app reads from these variables, so switching a theme
 * is a single style recalculation — no React re-render required.
 */
import { SHAPE_BASE, kebab, type ColorScheme, type ResolvedTheme } from './tokens'
import { hexToRgbTriplet } from './scheme'

const loadedFonts = new Set<string>()

export function ensureGoogleFonts(families?: string[]): void {
  if (!families || typeof document === 'undefined') return
  const missing = families.filter((f) => !loadedFonts.has(f))
  if (missing.length === 0) return
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = `https://fonts.googleapis.com/css2?${missing.map((f) => `family=${f}`).join('&')}&display=swap`
  document.head.appendChild(link)
  missing.forEach((f) => loadedFonts.add(f))
}

export interface ApplyOptions {
  mode: 'light' | 'dark'
  /** Runtime override of the resolved seed-derived scheme (e.g. dynamic color) */
  colorsOverride?: ColorScheme
  reducedMotion?: boolean
}

export function applyTheme(theme: ResolvedTheme, opts: ApplyOptions): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const def = theme.definition
  const colors = opts.colorsOverride ?? (opts.mode === 'dark' ? theme.dark : theme.light)

  const style = root.style
  // Color roles
  for (const [role, hex] of Object.entries(colors)) {
    style.setProperty(`--md-sys-color-${kebab(role)}`, hex)
    // rgb triplets for alpha compositing (state layers, scrims)
    style.setProperty(`--md-sys-color-${kebab(role)}-rgb`, hexToRgbTriplet(hex))
  }

  // Shape
  for (const [token, px] of Object.entries(SHAPE_BASE)) {
    style.setProperty(`--md-sys-shape-corner-${kebab(token)}`, `${Math.round(px * def.shape.scale)}px`)
  }
  style.setProperty('--md-sys-shape-corner-full', '9999px')

  // Typography
  style.setProperty('--md-ref-typeface-plain', def.typography.body)
  style.setProperty('--md-ref-typeface-brand', def.typography.headline ?? def.typography.body)
  ensureGoogleFonts(def.typography.googleFonts)

  // Effects
  root.dataset.theme = def.id
  root.dataset.mode = opts.mode
  root.dataset.glass = String(def.effects.glass)
  root.dataset.tonal = String(def.effects.tonalElevation)
  root.dataset.pureBlack = String(def.effects.pureBlack && opts.mode === 'dark')
  root.classList.toggle('dark', opts.mode === 'dark')
  root.style.colorScheme = opts.mode
  if (opts.reducedMotion !== undefined) root.dataset.reducedMotion = String(opts.reducedMotion)

  // PWA / browser chrome color (Android status + navigation bar, iOS standalone status bar)
  const chromeColor = colors.surfaceContainer ?? colors.surface
  const themeMetas = document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')
  if (themeMetas.length > 0) {
    themeMetas.forEach((m) => m.setAttribute('content', chromeColor))
  } else {
    const meta = document.createElement('meta')
    meta.name = 'theme-color'
    meta.setAttribute('content', chromeColor)
    document.head.appendChild(meta)
  }

  // Edge-to-edge Android draws the gesture pill over the document background — keep it themed too
  root.style.backgroundColor = chromeColor
  if (document.body) document.body.style.backgroundColor = colors.surface
}
