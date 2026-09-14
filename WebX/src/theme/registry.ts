/**
 * Theme registry — auto-discovers built-in themes from ./themes/*.ts and
 * merges user themes persisted in localStorage.
 */
import { ThemeDefinitionSchema, type ThemeDefinition, type ThemeDefinitionInput } from './tokens'

const USER_THEMES_KEY = 'webx.themes.user'

const modules = import.meta.glob<{ default: ThemeDefinitionInput }>('./themes/*.ts', { eager: true })

const builtIns: ThemeDefinition[] = Object.values(modules)
  .map((m) => ThemeDefinitionSchema.parse({ ...m.default, builtIn: true }))
  // Keep WebX first, rest alphabetical
  .sort((a, b) => (a.id === 'webx-crimson' ? -1 : b.id === 'webx-crimson' ? 1 : a.name.localeCompare(b.name)))

export const DEFAULT_THEME_ID = 'webx-crimson'

export function getBuiltInThemes(): ThemeDefinition[] {
  return builtIns
}

export function loadUserThemes(): ThemeDefinition[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = localStorage.getItem(USER_THEMES_KEY)
    if (!raw) return []
    const arr = JSON.parse(raw)
    if (!Array.isArray(arr)) return []
    return arr
      .map((t) => {
        const r = ThemeDefinitionSchema.safeParse({ ...t, builtIn: false })
        return r.success ? r.data : null
      })
      .filter((t): t is ThemeDefinition => t !== null)
  } catch {
    return []
  }
}

export function saveUserThemes(themes: ThemeDefinition[]): void {
  try {
    localStorage.setItem(USER_THEMES_KEY, JSON.stringify(themes.filter((t) => !t.builtIn)))
  } catch {}
}

/** Parse a theme JSON document (string or object). Throws a readable error. */
export function parseThemeJson(input: string | unknown): ThemeDefinition {
  const obj = typeof input === 'string' ? JSON.parse(input) : input
  const result = ThemeDefinitionSchema.safeParse({ ...(obj as object), builtIn: false })
  if (!result.success) {
    const first = result.error.issues[0]
    throw new Error(`Invalid theme: ${first.path.join('.') || 'root'} — ${first.message}`)
  }
  return result.data
}

export function exportThemeJson(theme: ThemeDefinition): string {
  const { builtIn: _b, ...rest } = theme
  return JSON.stringify(rest, null, 2)
}

export function findTheme(id: string, all: ThemeDefinition[]): ThemeDefinition | undefined {
  return all.find((t) => t.id === id)
}
