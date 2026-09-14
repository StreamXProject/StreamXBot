import { create } from 'zustand'
import { subscribeWithSelector } from 'zustand/middleware'
import {
  DEFAULT_THEME_ID,
  exportThemeJson,
  findTheme,
  getBuiltInThemes,
  loadUserThemes,
  parseThemeJson,
  saveUserThemes,
} from './registry'
import { resolveTheme, seedFromImage, schemeToColors } from './scheme'
import { applyTheme } from './apply'
import { ThemeDefinitionSchema, type ColorScheme, type ThemeDefinition, type ThemeMode } from './tokens'
import { Hct, argbFromHex, SchemeTonalSpot, SchemeVibrant } from '@material/material-color-utilities'

const KEYS = {
  active: 'webx.theme.active',
  mode: 'webx.theme.mode',
  dynamic: 'webx.theme.dynamic',
  motion: 'webx.theme.motion',
} as const

export type MotionPref = 'system' | 'full' | 'reduced'

interface ThemeStoreState {
  themes: ThemeDefinition[]
  activeThemeId: string
  mode: ThemeMode
  /** Derive colors from the current track artwork ("Material You") */
  dynamicColor: boolean
  dynamicSeed: string | null
  motion: MotionPref

  // derived (kept in state for cheap selectors)
  resolvedMode: 'light' | 'dark'
  activeTheme: ThemeDefinition

  setTheme: (id: string) => void
  setMode: (mode: ThemeMode) => void
  setDynamicColor: (on: boolean) => void
  setDynamicSeedFromImage: (url: string | null) => Promise<void>
  setMotion: (m: MotionPref) => void

  addUserTheme: (theme: ThemeDefinition) => void
  updateUserTheme: (theme: ThemeDefinition) => void
  removeUserTheme: (id: string) => void
  importThemeJson: (json: string) => ThemeDefinition
  exportThemeJson: (id: string) => string
  duplicateTheme: (id: string) => ThemeDefinition | null
}

const systemPrefersDark = () =>
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches

const systemReducedMotion = () =>
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

function resolveMode(mode: ThemeMode, theme: ThemeDefinition): 'light' | 'dark' {
  if (mode === 'light' || mode === 'dark') return mode
  // 'system': follow OS, but honour theme preference if OS has no preference signal
  if (typeof window === 'undefined') return theme.preferredMode === 'light' ? 'light' : 'dark'
  return systemPrefersDark() ? 'dark' : 'light'
}

function readInitial() {
  const builtIns = getBuiltInThemes()
  const user = loadUserThemes()
  const themes = [...builtIns, ...user]
  let activeThemeId = DEFAULT_THEME_ID
  let mode: ThemeMode = 'dark'
  let dynamicColor = false
  let motion: MotionPref = 'system'
  try {
    activeThemeId = localStorage.getItem(KEYS.active) || DEFAULT_THEME_ID
    const m = localStorage.getItem(KEYS.mode) as ThemeMode | null
    if (m === 'light' || m === 'dark' || m === 'system') mode = m
    else mode = findTheme(activeThemeId, themes)?.preferredMode ?? 'dark'
    dynamicColor = localStorage.getItem(KEYS.dynamic) === 'true'
    const mo = localStorage.getItem(KEYS.motion) as MotionPref | null
    if (mo === 'system' || mo === 'full' || mo === 'reduced') motion = mo
  } catch {}
  const activeTheme = findTheme(activeThemeId, themes) ?? themes[0]
  return { themes, activeThemeId: activeTheme.id, mode, dynamicColor, motion, activeTheme }
}

export const useThemeStore = create<ThemeStoreState>()(
  subscribeWithSelector((set, get) => {
    const init = readInitial()
    return {
      ...init,
      dynamicSeed: null,
      resolvedMode: resolveMode(init.mode, init.activeTheme),

      setTheme: (id) => {
        const theme = findTheme(id, get().themes)
        if (!theme) return
        localStorage.setItem(KEYS.active, id)
        set({ activeThemeId: id, activeTheme: theme, resolvedMode: resolveMode(get().mode, theme) })
      },
      setMode: (mode) => {
        localStorage.setItem(KEYS.mode, mode)
        set({ mode, resolvedMode: resolveMode(mode, get().activeTheme) })
      },
      setDynamicColor: (on) => {
        localStorage.setItem(KEYS.dynamic, String(on))
        set({ dynamicColor: on })
      },
      setDynamicSeedFromImage: async (url) => {
        if (!url) {
          set({ dynamicSeed: null })
          return
        }
        const seed = await seedFromImage(url)
        set({ dynamicSeed: seed })
      },
      setMotion: (m) => {
        localStorage.setItem(KEYS.motion, m)
        set({ motion: m })
      },

      addUserTheme: (theme) => {
        const clean = { ...theme, builtIn: false }
        const themes = [...get().themes.filter((t) => t.id !== clean.id), clean]
        saveUserThemes(themes)
        set({ themes })
      },
      updateUserTheme: (theme) => {
        const themes = get().themes.map((t) => (t.id === theme.id && !t.builtIn ? { ...theme, builtIn: false } : t))
        saveUserThemes(themes)
        const active = themes.find((t) => t.id === get().activeThemeId) ?? themes[0]
        set({ themes, activeTheme: active })
      },
      removeUserTheme: (id) => {
        const themes = get().themes.filter((t) => t.id !== id || t.builtIn)
        saveUserThemes(themes)
        set({ themes })
        if (get().activeThemeId === id) get().setTheme(DEFAULT_THEME_ID)
      },
      importThemeJson: (json) => {
        const theme = parseThemeJson(json)
        // avoid clobbering built-ins
        if (getBuiltInThemes().some((b) => b.id === theme.id)) theme.id = `${theme.id}-custom`
        get().addUserTheme(theme)
        return theme
      },
      exportThemeJson: (id) => {
        const theme = findTheme(id, get().themes)
        if (!theme) throw new Error('Theme not found')
        return exportThemeJson(theme)
      },
      duplicateTheme: (id) => {
        const src = findTheme(id, get().themes)
        if (!src) return null
        const base = src.id.replace(/-copy(-\d+)?$/, '')
        let n = 1
        let newId = `${base}-copy`
        while (get().themes.some((t) => t.id === newId)) newId = `${base}-copy-${++n}`
        const copy = ThemeDefinitionSchema.parse({
          ...src,
          id: newId,
          name: `${src.name} (copy)`,
          author: undefined,
          builtIn: false,
        })
        get().addUserTheme(copy)
        return copy
      },
    }
  })
)

function dynamicColors(seed: string, isDark: boolean, contrast: number, vibrant: boolean): ColorScheme {
  const hct = Hct.fromInt(argbFromHex(seed))
  const scheme = vibrant ? new SchemeVibrant(hct, isDark, contrast) : new SchemeTonalSpot(hct, isDark, contrast)
  return schemeToColors(scheme)
}

function applyFromState() {
  const s = useThemeStore.getState()
  const resolved = resolveTheme(s.activeTheme)
  const isDark = s.resolvedMode === 'dark'
  let colorsOverride: ColorScheme | undefined
  if (s.dynamicColor && s.dynamicSeed) {
    colorsOverride = dynamicColors(s.dynamicSeed, isDark, s.activeTheme.contrast, s.activeTheme.variant === 'vibrant')
    if (s.activeTheme.effects.pureBlack && isDark) {
      colorsOverride = { ...colorsOverride, ...resolved.dark, primary: colorsOverride.primary, onPrimary: colorsOverride.onPrimary, primaryContainer: colorsOverride.primaryContainer, onPrimaryContainer: colorsOverride.onPrimaryContainer, secondaryContainer: colorsOverride.secondaryContainer, onSecondaryContainer: colorsOverride.onSecondaryContainer, tertiary: colorsOverride.tertiary }
    }
  }
  const reducedMotion = s.motion === 'reduced' || (s.motion === 'system' && systemReducedMotion())
  applyTheme(resolved, { mode: s.resolvedMode, colorsOverride, reducedMotion })
}

if (typeof window !== 'undefined') {
  applyFromState()
  useThemeStore.subscribe(
    (s) => [s.activeTheme, s.resolvedMode, s.dynamicColor, s.dynamicSeed, s.motion] as const,
    () => applyFromState(),
    { equalityFn: (a, b) => a.every((v, i) => v === b[i]) }
  )
  const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
  mq?.addEventListener?.('change', () => {
    const s = useThemeStore.getState()
    if (s.mode === 'system') {
      useThemeStore.setState({ resolvedMode: resolveMode('system', s.activeTheme) })
    }
  })
  window.matchMedia?.('(prefers-reduced-motion: reduce)')?.addEventListener?.('change', applyFromState)
}

/** Resolved colors for previews (theme cards etc.) */
export function previewColors(theme: ThemeDefinition, mode: 'light' | 'dark'): ColorScheme {
  const r = resolveTheme(theme)
  return mode === 'dark' ? r.dark : r.light
}
