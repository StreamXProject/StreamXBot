import { create } from 'zustand'
import type { Track } from '@/schemas/track'
import type { RepeatMode } from '@/audio/AudioState'
import { QueueManager } from '@/audio/QueueManager'
import { audioEngine } from '@/audio/AudioEngine'
import { usePlayerStore, readPersistedPlayback } from './playerStore'
import { useSettingsStore } from './settingsStore'

export interface QueueContext {
  type: 'album' | 'artist' | 'playlist' | 'mix' | 'search' | 'library' | 'browse' | 'radio' | 'custom'
  id?: string
  title: string
  href?: string
}

interface QueueStoreState {
  queue: Track[]
  currentIndex: number
  isShuffle: boolean
  repeatMode: RepeatMode
  context: QueueContext | null

  playTrackWithQueue: (tracks: Track[], startIndex?: number, context?: QueueContext) => Promise<void>
  playTrack: (track: Track, context?: QueueContext) => Promise<void>
  jumpTo: (index: number) => Promise<void>
  nextTrack: () => Promise<void>
  previousTrack: () => Promise<void>
  toggleShuffle: () => void
  setRepeatMode: (mode: RepeatMode) => void
  cycleRepeatMode: () => void
  addToQueue: (track: Track | Track[]) => void
  playNext: (track: Track | Track[]) => void
  removeFromQueue: (index: number) => void
  reorderQueue: (fromIndex: number, toIndex: number) => void
  clearQueue: () => void
  clearUpcoming: () => void
  restoreSession: () => Promise<void>
}

const queueManager = new QueueManager()
const QUEUE_KEY = 'webx.queue'

function persistQueue(state: Pick<QueueStoreState, 'queue' | 'currentIndex' | 'isShuffle' | 'repeatMode' | 'context'>) {
  try {
    const slim = state.queue.slice(0, 500)
    localStorage.setItem(QUEUE_KEY, JSON.stringify({ ...state, queue: slim }))
  } catch {
    /* ignore quota */
  }
}

export const useQueueStore = create<QueueStoreState>((set, get) => {
  const sync = (extra: Partial<QueueStoreState> = {}) => {
    const next = {
      queue: queueManager.getQueue(),
      currentIndex: queueManager.getCurrentIndex(),
      isShuffle: queueManager.getIsShuffle(),
      repeatMode: queueManager.getRepeatMode(),
      ...extra,
    }
    set(next)
    const s = get()
    persistQueue({ queue: s.queue, currentIndex: s.currentIndex, isShuffle: s.isShuffle, repeatMode: s.repeatMode, context: s.context })
    prefetchUpcoming()
  }

  const prefetchUpcoming = () => {
    if (!useSettingsStore.getState().prefetchNext) return
    const q = queueManager.getQueue()
    const i = queueManager.getCurrentIndex()
    const next = queueManager.getRepeatMode() === 'one' ? null : q[i + 1] ?? (queueManager.getRepeatMode() === 'all' ? q[0] : null)
    audioEngine.prefetchTrack(next && next.id !== q[i]?.id ? next : null)
  }

  return {
    queue: [],
    currentIndex: -1,
    isShuffle: false,
    repeatMode: 'off',
    context: null,

    playTrackWithQueue: async (tracks, startIndex = 0, context) => {
      if (tracks.length === 0) return
      queueManager.setQueue(tracks, startIndex)
      sync({ context: context ?? null })
      const current = queueManager.getCurrentTrack()
      if (current) await usePlayerStore.getState().playTrack(current)
    },

    playTrack: async (track, context) => get().playTrackWithQueue([track], 0, context),

    jumpTo: async (index) => {
      const t = queueManager.jumpToIndex(index)
      if (!t) return
      sync()
      await usePlayerStore.getState().playTrack(t)
    },

    nextTrack: async () => {
      const next = queueManager.next()
      if (next) {
        sync()
        await usePlayerStore.getState().playTrack(next)
      } else {
        audioEngine.pause()
        audioEngine.seek(0)
      }
    },

    previousTrack: async () => {
      if (audioEngine.getProgress().currentTime > 3) {
        audioEngine.seek(0)
        return
      }
      const prev = queueManager.previous()
      if (prev) {
        sync()
        await usePlayerStore.getState().playTrack(prev)
      }
    },

    toggleShuffle: () => {
      queueManager.toggleShuffle()
      sync()
    },
    setRepeatMode: (mode) => {
      queueManager.setRepeatMode(mode)
      sync()
    },
    cycleRepeatMode: () => {
      const modes: RepeatMode[] = ['off', 'all', 'one']
      get().setRepeatMode(modes[(modes.indexOf(get().repeatMode) + 1) % modes.length])
    },

    addToQueue: (t) => {
      for (const track of Array.isArray(t) ? t : [t]) queueManager.appendTrack(track)
      sync()
    },
    playNext: (t) => {
      const arr = Array.isArray(t) ? [...t].reverse() : [t]
      for (const track of arr) queueManager.addTrackNext(track)
      sync()
    },
    removeFromQueue: (index) => {
      queueManager.removeTrack(index)
      sync()
    },
    reorderQueue: (from, to) => {
      queueManager.reorder(from, to)
      sync()
    },
    clearQueue: () => {
      queueManager.clear()
      audioEngine.stop()
      sync({ context: null })
    },
    clearUpcoming: () => {
      const i = queueManager.getCurrentIndex()
      const q = queueManager.getQueue()
      for (let k = q.length - 1; k > i; k--) queueManager.removeTrack(k)
      sync()
    },

    restoreSession: async () => {
      if (!useSettingsStore.getState().resumeOnLaunch) return
      try {
        const raw = localStorage.getItem(QUEUE_KEY)
        if (!raw) return
        const saved = JSON.parse(raw) as { queue: Track[]; currentIndex: number; isShuffle: boolean; repeatMode: RepeatMode; context: QueueContext | null }
        if (!Array.isArray(saved.queue) || saved.queue.length === 0) return
        queueManager.setQueue(saved.queue, Math.max(0, saved.currentIndex))
        queueManager.setRepeatMode(saved.repeatMode ?? 'off')
        if (saved.isShuffle) queueManager.toggleShuffle()
        sync({ context: saved.context ?? null })
        const current = queueManager.getCurrentTrack()
        if (current) {
          const pos = readPersistedPlayback()
          const startAt = pos && pos.trackId === current.id ? pos.position : 0
          await usePlayerStore.getState().playTrack(current, startAt, false)
        }
      } catch {
        /* ignore corrupt state */
      }
    },
  }
})

audioEngine.onTrackEnd(() => {
  const p = usePlayerStore.getState()
  if (p.sleepAfterTrack) {
    usePlayerStore.setState({ sleepAfterTrack: false })
    return
  }
  void useQueueStore.getState().nextTrack()
})

audioEngine.setMediaSessionHandlers({
  next: () => void useQueueStore.getState().nextTrack(),
  previous: () => void useQueueStore.getState().previousTrack(),
})
