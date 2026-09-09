import type { Track } from '@/schemas/track'

export type PlaybackStatus = 'idle' | 'loading' | 'playing' | 'paused' | 'error'

export type RepeatMode = 'off' | 'all' | 'one'

export interface AudioPlaybackState {
  status: PlaybackStatus
  currentTrack: Track | null
  isPlaying: boolean
  isBuffering: boolean
  volume: number
  isMuted: boolean
  playbackRate: number
  error: string | null
}

export interface ProgressState {
  currentTime: number
  duration: number
  buffered: number
  progressPercent: number
}
