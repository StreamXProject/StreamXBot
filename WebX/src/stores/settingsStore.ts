import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { AudioQualityLevel } from '@/audio/AudioQuality'

export type StreamFormat = 'auto' | 'original' | 'flac'
export type LibraryTab = 'liked' | 'playlists' | 'history' | 'albums' | 'artists'
export type StartPage = '/' | '/search' | '/library'
export type DiscordMode = 'gateway' | 'daemon'
export type LyricsProvider = 'auto' | 'betterlyrics' | 'musixmatch' | 'lrclib' | 'kugou'
export type LyricsAnimationStyle = 'apple_music_v2' | 'lyrics_v2_fluid' | 'apple_music' | 'glow' | 'fade' | 'classic'
export type LyricsTextPosition = 'left' | 'center' | 'right'
export type LyricsTextSize = 'sm' | 'md' | 'lg' | 'xl'

export interface SettingsState {
  apiBaseUrl: string
  /** Remembered servers for quick switching */
  knownServers: string[]

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

  compactRows: boolean
  showLyricsButton: boolean
  startPage: StartPage
  defaultLibraryTab: LibraryTab
  railLabels: boolean
  keyboardShortcuts: boolean
  confirmDestructive: boolean
  showQualityBadges: boolean
  /** Interface scale (CSS zoom on <body>), 0.8 – 1.3 */
  uiScale: number

  lyricsProvider: LyricsProvider
  lyricsAnimationStyle: LyricsAnimationStyle
  lyricsTextPosition: LyricsTextPosition
  lyricsGlow: boolean
  lyricsBlur: boolean
  lyricsTextSize: LyricsTextSize
  lyricsLineSpacing: number
  lyricsAutoScroll: boolean
  lyricsSeekOnClick: boolean
  lyricsSyncOffsetMs: number

  eqEnabled: boolean
  eqPreset: string
  eqGains: number[]
  eqPreamp: number

  lastfmEnabled: boolean
  lastfmApiKey: string
  lastfmApiSecret: string
  lastfmSessionKey: string
  lastfmUsername: string
  /** Scrobble once playback passes this fraction of the track (0.5 = 50%) */
  lastfmScrobbleAt: number
  lastfmNowPlaying: boolean

  discordEnabled: boolean
  discordMode: DiscordMode
  discordUserToken: string
  discordClientId: string
  discordDaemonUrl: string
  discordShowArtwork: boolean

  pwaAutoUpdate: boolean

  /** Record listening events for recaps */
  recapsEnabled: boolean

  /** Vibrate on long-press, drag and gestures (where supported) */
  haptics: boolean
  /** Swipe the mini player left/right to skip, up to expand */
  miniPlayerSwipe: boolean

  hasSeenWelcome: boolean

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
  } catch {}
  return DEFAULT_ORIGIN
}

const DEFAULTS = {
  apiBaseUrl: legacyBase(),
  knownServers: [] as string[],
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
  uiScale: 1,
  lyricsProvider: 'auto' as LyricsProvider,
  lyricsAnimationStyle: 'apple_music_v2' as LyricsAnimationStyle,
  lyricsTextPosition: 'left' as LyricsTextPosition,
  lyricsGlow: true,
  lyricsBlur: true,
  lyricsTextSize: 'lg' as LyricsTextSize,
  lyricsLineSpacing: 1.5,
  lyricsAutoScroll: true,
  lyricsSeekOnClick: true,
  lyricsSyncOffsetMs: 0,
  eqEnabled: false,
  eqPreset: 'flat',
  eqGains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] as number[],
  eqPreamp: 0,
  lastfmEnabled: false,
  lastfmApiKey: '',
  lastfmApiSecret: '',
  lastfmSessionKey: '',
  lastfmUsername: '',
  lastfmScrobbleAt: 0.5,
  lastfmNowPlaying: true,
  discordEnabled: false,
  discordMode: 'gateway' as DiscordMode,
  discordUserToken: '',
  discordClientId: '1547543416143876167',
  discordDaemonUrl: 'ws://127.0.0.1:6472',
  discordShowArtwork: true,
  pwaAutoUpdate: true,
  haptics: true,
  recapsEnabled: true,
  miniPlayerSwipe: true,
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
