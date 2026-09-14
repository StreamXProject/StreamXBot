import { create } from 'zustand'
import type { Track } from '@/schemas/track'
import type { PlaybackStatus } from '@/audio/AudioState'
import { audioEngine } from '@/audio/AudioEngine'
import { getStreamUrl, resolveStreamFormat, warmTrack } from '@/api/stream'
import { useSettingsStore } from './settingsStore'

interface PlayerStoreState {
  currentTrack: Track | null
  isPlaying: boolean
  isBuffering: boolean
  status: PlaybackStatus
  volume: number
  isMuted: boolean
  playbackRate: number
  error: string | null
  /** Sleep timer: epoch ms when playback should pause, or null */
  sleepAt: number | null
  sleepAfterTrack: boolean

  playTrack: (track: Track, startAt?: number, autoPlay?: boolean) => Promise<void>
  togglePlay: () => void
  pause: () => void
  resume: () => void
  stop: () => void
  setVolume: (volume: number) => void
  adjustVolume: (delta: number) => void
  toggleMute: () => void
  setPlaybackRate: (rate: number) => void
  setSleepTimer: (minutes: number | null) => void
  setSleepAfterTrack: (on: boolean) => void
  syncWithEngine: (state: ReturnType<typeof audioEngine.getState>) => void
}

audioEngine.setStreamResolver({
  url: (track) => getStreamUrl(track),
  needsTranscode: (track) => resolveStreamFormat(track) !== undefined && !track.stream_url,
  warm: warmTrack,
})

const PLAYBACK_KEY = 'webx.playback'

export interface PersistedPlayback {
  trackId: string | null
  position: number
  updatedAt: number
}

export const usePlayerStore = create<PlayerStoreState>((set, get) => ({
  currentTrack: null,
  isPlaying: false,
  isBuffering: false,
  status: 'idle',
  volume: useSettingsStore.getState().volume,
  isMuted: false,
  playbackRate: useSettingsStore.getState().playbackRate,
  error: null,
  sleepAt: null,
  sleepAfterTrack: false,

  playTrack: async (track, startAt = 0, autoPlay = true) => {
    set({ currentTrack: track, isBuffering: autoPlay, error: null })
    await audioEngine.loadTrack(track, autoPlay, startAt)
  },
  togglePlay: () => audioEngine.togglePlay(),
  pause: () => audioEngine.pause(),
  resume: () => void audioEngine.play(),
  stop: () => audioEngine.stop(),

  setVolume: (vol) => {
    const v = Math.max(0, Math.min(1, vol))
    audioEngine.setVolume(v)
    useSettingsStore.getState().set('volume', v)
    set({ volume: v, isMuted: v === 0 })
  },
  adjustVolume: (delta) => get().setVolume(get().volume + delta),
  toggleMute: () => {
    audioEngine.toggleMute()
    set({ isMuted: audioEngine.getState().isMuted })
  },
  setPlaybackRate: (rate) => {
    audioEngine.setPlaybackRate(rate)
    useSettingsStore.getState().set('playbackRate', rate)
    set({ playbackRate: rate })
  },
  setSleepTimer: (minutes) => set({ sleepAt: minutes ? Date.now() + minutes * 60_000 : null, sleepAfterTrack: false }),
  setSleepAfterTrack: (on) => set({ sleepAfterTrack: on, sleepAt: null }),

  syncWithEngine: (s) =>
    set({
      currentTrack: s.currentTrack,
      isPlaying: s.isPlaying,
      isBuffering: s.isBuffering,
      status: s.status,
      volume: s.volume,
      isMuted: s.isMuted,
      playbackRate: s.playbackRate,
      error: s.error,
    }),
}))

audioEngine.subscribeState((state) => usePlayerStore.getState().syncWithEngine(state))

if (typeof window !== 'undefined') {
  audioEngine.setVolume(useSettingsStore.getState().volume)
  audioEngine.setPlaybackRate(useSettingsStore.getState().playbackRate)

  setInterval(() => {
    const s = usePlayerStore.getState()
    if (s.sleepAt && Date.now() >= s.sleepAt && s.isPlaying) {
      audioEngine.pause()
      set_sleep_null()
    }
  }, 1000)

  // Persist position every few seconds for resume-on-launch
  let lastSaved = 0
  audioEngine.subscribeProgress((p) => {
    const now = Date.now()
    if (now - lastSaved < 4000) return
    lastSaved = now
    const track = usePlayerStore.getState().currentTrack
    try {
      localStorage.setItem(PLAYBACK_KEY, JSON.stringify({ trackId: track?.id ?? null, position: p.currentTime, updatedAt: now } satisfies PersistedPlayback))
    } catch {}
  })
}

function set_sleep_null() {
  usePlayerStore.setState({ sleepAt: null })
}

export function readPersistedPlayback(): PersistedPlayback | null {
  try {
    const raw = localStorage.getItem(PLAYBACK_KEY)
    return raw ? (JSON.parse(raw) as PersistedPlayback) : null
  } catch {
    return null
  }
}
