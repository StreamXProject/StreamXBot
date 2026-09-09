import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { AudioQualityLevel } from '@/audio/AudioQuality'

export type StreamFormat = 'auto' | 'original' | 'flac'
export type LibraryTab = 'liked' | 'playlists' | 'history' | 'albums' | 'artists'
export type StartPage = '/' | '/search' | '/library'

export interface SettingsState {
  // Server / endpoints
  apiBaseUrl: string
  /** Remembered servers for quick switching */
  knownServers: string[]
  /** Use bundled demo catalog when the server is unreachable */
  demoMode: boolean

  // Playback
  audioQuality: AudioQualityLevel
  streamFormat: StreamFormat
  normalizeAudio: boolean
  crossfadeSec: number
  prefetchNext: boolean
  resumeOnLaunch: boolean
  playbackRate: number
  volume: number
  /** Skip silence at track boundaries (client-side gapless attempt) */
  gapless: boolean

  // Interface
  compactRows: boolean
  showLyricsButton: boolean
  startPage: StartPage
  defaultLibraryTab: LibraryTab
  railLabels: boolean
  keyboardShortcuts: boolean
  confirmDestructive: boolean
  showQualityBadges: boolean

  // Onboarding
  hasSeenWelcome: boolean

  // actions
  setApiBaseUrl: (url: string) => void
  addKnownServer: (url: string) => void
  removeKnownServer: (url: string) => void
  set: <K extends keyof SettingsState>(key: K, value: SettingsState[K]) => void
  reset: () => void
}

const DEFAULT_ORIGIN = typeof window !== 'undefined' && window.location?.origin?.startsWith('http') ? window.location.origin : 'http://localhost:8000'

function legacyBase(): string {
  try {
    const stored = localStorage.getItem('webx_api_base')
    if (stored && stored !== 'http://localhost:8000') return stored
  } catch {
    /* noop */
  }
  return DEFAULT_ORIGIN
}

const DEFAULTS = {
  apiBaseUrl: legacyBase(),
  knownServers: [] as string[],
  demoMode: false,
  audioQuality: 'lossless' as AudioQualityLevel,
  streamFormat: 'auto' as StreamFormat,
  normalizeAudio: false,
  crossfadeSec: 0,
  prefetchNext: true,
  resumeOnLaunch: true,
  playbackRate: 1,
  volume: 0.8,
  gapless: true,
  compactRows: false,
  showLyricsButton: true,
  startPage: '/' as StartPage,
  defaultLibraryTab: 'liked' as LibraryTab,
  railLabels: true,
  keyboardShortcuts: true,
  confirmDestructive: true,
  showQualityBadges: true,
  hasSeenWelcome: false,
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set, get) => ({
      ...DEFAULTS,
      setApiBaseUrl: (url) => {
        const clean = url.trim().replace(/\/+$/, '')
        set({ apiBaseUrl: clean })
        get().addKnownServer(clean)
      },
      addKnownServer: (url) => {
        const clean = url.trim().replace(/\/+$/, '')
        if (!clean) return
        set({ knownServers: [clean, ...get().knownServers.filter((s) => s !== clean)].slice(0, 6) })
      },
      removeKnownServer: (url) => set({ knownServers: get().knownServers.filter((s) => s !== url) }),
      set: (key, value) => set({ [key]: value } as Partial<SettingsState>),
      reset: () => set({ ...DEFAULTS, apiBaseUrl: get().apiBaseUrl, knownServers: get().knownServers }),
    }),
    {
      name: 'webx.settings',
      version: 2,
      partialize: (s) => {
        const { setApiBaseUrl: _a, addKnownServer: _b, removeKnownServer: _c, set: _d, reset: _e, ...rest } = s
        return rest
      },
    }
  )
)
