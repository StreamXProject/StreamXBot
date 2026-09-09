import { create } from 'zustand'
import { audioEngine } from '@/audio/AudioEngine'
import type { ProgressState } from '@/audio/AudioState'

interface ProgressStoreState extends ProgressState {
  seek: (seconds: number) => void
  seekPercent: (percent: number) => void
  seekBy: (delta: number) => void
  setProgress: (progress: ProgressState) => void
}

/**
 * Isolated high-frequency store. Only components that truly need the time
 * subscribe here; everything else stays untouched by playback ticks.
 */
export const useProgressStore = create<ProgressStoreState>((set) => ({
  currentTime: 0,
  duration: 0,
  buffered: 0,
  progressPercent: 0,
  seek: (seconds) => audioEngine.seek(seconds),
  seekPercent: (percent) => audioEngine.seekPercent(percent),
  seekBy: (delta) => audioEngine.seekBy(delta),
  setProgress: (progress) => set(progress),
}))

audioEngine.subscribeProgress((p) => useProgressStore.getState().setProgress(p))
