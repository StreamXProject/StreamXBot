import type { Track } from '@/schemas/track'
import type { AudioPlaybackState, ProgressState } from './AudioState'

type StateListener = (state: AudioPlaybackState) => void
type ProgressListener = (progress: ProgressState) => void
type TrackEndListener = () => void
type ErrorListener = (message: string, track: Track | null) => void

export interface StreamResolver {
  /** Build the playable URL for a track */
  url: (track: Track) => string
  /** Whether this track will be transcoded server-side (warm before play) */
  needsTranscode: (track: Track) => boolean
  /** Ask the server to prepare/cache the file */
  warm: (trackId: string, opts?: { force?: boolean }) => Promise<{ ok: boolean; ready: boolean }>
}

export interface MediaSessionHandlers {
  next: () => void
  previous: () => void
}

/**
 * Single <audio> element wrapper. UI never touches the element directly.
 *
 * Performance notes:
 *  - Progress is emitted at most every 250 ms while playing (not per frame);
 *    scrubbers interpolate visually via CSS transitions.
 *  - A second, hidden <audio> pre-buffers the next track.
 */
export class AudioEngine {
  private static instance: AudioEngine | null = null
  private audio: HTMLAudioElement
  private prefetch: HTMLAudioElement | null = null
  private prefetchTrackId: string | null = null

  private stateListeners = new Set<StateListener>()
  private progressListeners = new Set<ProgressListener>()
  private trackEndListeners = new Set<TrackEndListener>()
  private errorListeners = new Set<ErrorListener>()

  private currentTrack: Track | null = null
  private isBuffering = false
  private lastError: string | null = null
  private loadSeq = 0
  private progressTimer: number | null = null
  private pollTimer: number | null = null
  private resolver: StreamResolver | null = null
  private sessionHandlers: MediaSessionHandlers | null = null

  private constructor() {
    if (typeof window !== 'undefined' && typeof Audio !== 'undefined') {
      this.audio = new Audio()
      this.audio.setAttribute('playsinline', 'true')
      this.audio.setAttribute('webkit-playsinline', 'true')
      if (typeof document !== 'undefined') {
        const attach = () => {
          if (document.body && !document.body.contains(this.audio)) {
            this.audio.style.position = 'fixed'
            this.audio.style.width = '0'
            this.audio.style.height = '0'
            this.audio.style.opacity = '0'
            this.audio.style.pointerEvents = 'none'
            this.audio.style.zIndex = '-1'
            document.body.appendChild(this.audio)
          }
        }
        if (document.readyState === 'loading') {
          document.addEventListener('DOMContentLoaded', attach, { once: true })
        } else {
          attach()
        }
      }
    } else {
      this.audio = {
        preload: 'auto',
        volume: 0.8,
        paused: true,
        muted: false,
        currentTime: 0,
        duration: 0,
        playbackRate: 1,
        buffered: { length: 0, end: () => 0 },
        play: async () => {},
        pause: () => {},
        load: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        removeAttribute: () => {},
        setAttribute: () => {},
      } as unknown as HTMLAudioElement
    }
    this.audio.preload = 'auto'
    this.audio.volume = 0.8
    this.setupAudioListeners()
  }

  public static getInstance(): AudioEngine {
    if (!AudioEngine.instance) AudioEngine.instance = new AudioEngine()
    return AudioEngine.instance
  }

  public setStreamResolver(resolver: StreamResolver) {
    this.resolver = resolver
  }

  public setMediaSessionHandlers(h: MediaSessionHandlers) {
    this.sessionHandlers = h
    this.bindMediaSessionActions()
  }

  /* ---------------------------------------------------------------- */

  private setupAudioListeners(): void {
    const a = this.audio
    a.addEventListener('play', () => {
      this.emitState()
      this.startProgressLoop()
      this.setSessionState('playing')
    })
    a.addEventListener('pause', () => {
      this.emitState()
      this.stopProgressLoop()
      this.emitProgress()
      this.setSessionState('paused')
    })
    a.addEventListener('waiting', () => {
      this.isBuffering = true
      this.emitState()
    })
    a.addEventListener('canplay', () => {
      this.isBuffering = false
      this.emitState()
    })
    a.addEventListener('playing', () => {
      this.isBuffering = false
      this.lastError = null
      this.emitState()
    })
    a.addEventListener('loadedmetadata', () => this.emitProgress())
    a.addEventListener('durationchange', () => this.emitProgress())
    a.addEventListener('seeked', () => this.emitProgress())
    a.addEventListener('ratechange', () => this.emitState())
    a.addEventListener('volumechange', () => this.emitState())
    a.addEventListener('ended', () => {
      this.stopProgressLoop()
      this.emitProgress()
      this.emitState()
      this.trackEndListeners.forEach((l) => l())
    })
    a.addEventListener('error', () => {
      this.isBuffering = false
      this.stopProgressLoop()
      const code = a.error?.code
      const msg =
        code === MediaError.MEDIA_ERR_NETWORK
          ? 'Network error while streaming'
          : code === MediaError.MEDIA_ERR_DECODE
            ? 'This format cannot be decoded by your browser'
            : code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED
              ? 'Stream unavailable or unsupported'
              : 'Playback error'
      this.lastError = msg
      this.emitState()
      this.errorListeners.forEach((l) => l(msg, this.currentTrack))
    })
  }

  /* ---------------------------------------------------------------- */

  public subscribeState(listener: StateListener): () => void {
    this.stateListeners.add(listener)
    listener(this.getState())
    return () => this.stateListeners.delete(listener)
  }

  public subscribeProgress(listener: ProgressListener): () => void {
    this.progressListeners.add(listener)
    listener(this.getProgress())
    return () => this.progressListeners.delete(listener)
  }

  public onTrackEnd(listener: TrackEndListener): () => void {
    this.trackEndListeners.add(listener)
    return () => this.trackEndListeners.delete(listener)
  }

  public onError(listener: ErrorListener): () => void {
    this.errorListeners.add(listener)
    return () => this.errorListeners.delete(listener)
  }

  /* ---------------------------------------------------------------- */

  public async loadTrack(track: Track, autoPlay = true, startAt = 0): Promise<void> {
    const seq = ++this.loadSeq
    this.clearPoll()
    this.currentTrack = track
    this.isBuffering = true
    this.lastError = null
    this.emitState()
    this.setupMediaSession(track)

    const url = this.resolver ? this.resolver.url(track) : track.stream_url || ''
    if (!url) {
      this.isBuffering = false
      this.lastError = 'No stream URL for this track'
      this.emitState()
      return
    }

    const apply = async () => {
      if (seq !== this.loadSeq) return
      // Reuse the prefetch element's buffer if it already holds this track
      if (this.prefetch && this.prefetchTrackId === track.id && this.prefetch.src === url && this.prefetch.readyState >= 2) {
        // Swapping elements is more expensive than letting the browser use its HTTP cache; just set src.
      }
      const currentSrc = this.audio.src || ''
      const targetSrc = typeof window !== 'undefined' && url ? new URL(url, window.location.href).href : url
      if (currentSrc !== targetSrc) {
        this.audio.src = url
        try {
          this.audio.currentTime = 0
        } catch {
          /* ignore */
        }
      }
      if (startAt > 0) {
        const seekOnce = () => {
          this.audio.removeEventListener('loadedmetadata', seekOnce)
          try {
            this.audio.currentTime = startAt
          } catch {
            /* ignore */
          }
        }
        this.audio.addEventListener('loadedmetadata', seekOnce)
      }
      this.emitProgress()
      if (autoPlay) {
        try {
          await this.audio.play()
        } catch (err) {
          if (seq !== this.loadSeq) return
          const e = err as Error
          if (e?.name !== 'AbortError') {
            console.warn('Autoplay failed:', e)
            this.isBuffering = false
            this.emitState()
          }
        }
      } else {
        this.isBuffering = false
        this.emitState()
      }
    }

    if (this.resolver?.needsTranscode(track)) {
      // Transcoded streams must be ready on the server before we request them
      try {
        const res = await this.resolver.warm(track.id)
        if (seq !== this.loadSeq) return
        if (res.ready) return apply()
        this.pollTimer = window.setInterval(async () => {
          if (seq !== this.loadSeq) return this.clearPoll()
          const again = await this.resolver!.warm(track.id, { force: true })
          if (again.ready && seq === this.loadSeq) {
            this.clearPoll()
            apply()
          }
        }, 1500)
      } catch {
        return apply()
      }
      return
    }

    await apply()
    // Warm in the background so subsequent seeks/replays are instant
    this.resolver?.warm(track.id).catch(() => {})
  }

  /** Pre-buffer the upcoming track so the transition is near-gapless */
  public prefetchTrack(track: Track | null): void {
    if (typeof Audio === 'undefined') return
    if (!track || !this.resolver) {
      this.clearPrefetch()
      return
    }
    if (this.prefetchTrackId === track.id) return
    this.resolver.warm(track.id).catch(() => {})
    if (this.resolver.needsTranscode(track)) return

    // On mobile / iOS, avoid creating concurrent <audio> decoders which can stall the primary audio stream
    const isMobile = typeof navigator !== 'undefined' && /iPhone|iPad|iPod|Android/i.test(navigator.userAgent)
    if (isMobile) {
      return
    }

    try {
      if (!this.prefetch) {
        this.prefetch = new Audio()
        this.prefetch.preload = 'auto'
        this.prefetch.muted = true
        this.prefetch.setAttribute('playsinline', 'true')
        this.prefetch.setAttribute('webkit-playsinline', 'true')
      }
      this.prefetch.src = this.resolver.url(track)
      this.prefetch.load()
      this.prefetchTrackId = track.id
    } catch {
      /* ignore */
    }
  }

  private clearPrefetch() {
    if (this.prefetch) {
      this.prefetch.removeAttribute('src')
      this.prefetch.load()
    }
    this.prefetchTrackId = null
  }

  private clearPoll() {
    if (this.pollTimer !== null) {
      clearInterval(this.pollTimer)
      this.pollTimer = null
    }
  }

  public async play(): Promise<void> {
    if (!this.audio.src && this.currentTrack) return this.loadTrack(this.currentTrack, true)
    try {
      await this.audio.play()
    } catch (err) {
      if ((err as Error)?.name !== 'AbortError') console.warn('Resume failed:', err)
    }
  }

  public pause(): void {
    this.audio.pause()
  }

  public togglePlay(): void {
    if (this.audio.paused) void this.play()
    else this.pause()
  }

  public stop(): void {
    this.loadSeq++
    this.clearPoll()
    this.audio.pause()
    this.audio.removeAttribute('src')
    this.audio.load()
    this.currentTrack = null
    this.isBuffering = false
    this.emitState()
    this.emitProgress()
    this.setSessionState('none')
  }

  public seek(seconds: number): void {
    const d = this.audio.duration || this.currentTrack?.duration_sec || 0
    if (!Number.isFinite(seconds) || !d) return
    this.audio.currentTime = Math.max(0, Math.min(seconds, d))
    this.emitProgress()
  }

  public seekPercent(percent: number): void {
    const d = this.audio.duration || this.currentTrack?.duration_sec || 0
    if (d) this.seek((Math.max(0, Math.min(percent, 100)) / 100) * d)
  }

  public seekBy(deltaSeconds: number): void {
    this.seek((this.audio.currentTime || 0) + deltaSeconds)
  }

  public setVolume(volume: number): void {
    const v = Math.max(0, Math.min(volume, 1))
    this.audio.volume = v
    if (v > 0 && this.audio.muted) this.audio.muted = false
    this.emitState()
  }

  public toggleMute(): void {
    this.audio.muted = !this.audio.muted
    this.emitState()
  }

  public setPlaybackRate(rate: number): void {
    this.audio.playbackRate = Math.max(0.5, Math.min(2, rate))
    this.emitState()
  }

  /* ---------------------------------------------------------------- */

  public getState(): AudioPlaybackState {
    return {
      status: this.lastError
        ? 'error'
        : this.isBuffering
          ? 'loading'
          : !this.audio.paused
            ? 'playing'
            : this.currentTrack
              ? 'paused'
              : 'idle',
      currentTrack: this.currentTrack,
      isPlaying: !this.audio.paused && !this.audio.ended,
      isBuffering: this.isBuffering,
      volume: this.audio.volume,
      isMuted: this.audio.muted,
      playbackRate: this.audio.playbackRate,
      error: this.lastError,
    }
  }

  public getProgress(): ProgressState {
    const currentTime = this.audio.currentTime || 0
    const duration = (Number.isFinite(this.audio.duration) && this.audio.duration) || this.currentTrack?.duration_sec || 0
    let buffered = 0
    try {
      const b = this.audio.buffered
      for (let i = 0; i < b.length; i++) {
        if (b.start(i) <= currentTime && b.end(i) >= currentTime) buffered = b.end(i)
      }
      if (!buffered && b.length) buffered = b.end(b.length - 1)
    } catch {
      buffered = 0
    }
    return { currentTime, duration, buffered, progressPercent: duration > 0 ? (currentTime / duration) * 100 : 0 }
  }

  private startProgressLoop(): void {
    if (this.progressTimer !== null) return
    this.progressTimer = window.setInterval(() => {
      if (this.audio.paused) return this.stopProgressLoop()
      this.emitProgress()
    }, 250)
  }

  private stopProgressLoop(): void {
    if (this.progressTimer !== null) {
      clearInterval(this.progressTimer)
      this.progressTimer = null
    }
  }

  private emitState(): void {
    const s = this.getState()
    this.stateListeners.forEach((l) => l(s))
  }

  private emitProgress(): void {
    const p = this.getProgress()
    this.progressListeners.forEach((l) => l(p))
    this.updatePositionState(p)
  }

  /* ---------------------------------------------------------------- */
  /* Media Session                                                     */

  private setupMediaSession(track: Track): void {
    try {
      if ('mediaSession' in navigator && typeof MediaMetadata !== 'undefined') {
        navigator.mediaSession.metadata = new MediaMetadata({
          title: track.title,
          artist: track.artist,
          album: track.album || 'WebX',
          artwork: track.cover_url ? [{ src: track.cover_url, sizes: '512x512', type: 'image/jpeg' }] : [],
        })
      }
    } catch {
      /* ignore */
    }
  }

  private bindMediaSessionActions() {
    if (typeof navigator === 'undefined' || !('mediaSession' in navigator)) return
    const ms = navigator.mediaSession
    const bind = (action: MediaSessionAction, handler: MediaSessionActionHandler | null) => {
      try {
        ms.setActionHandler(action, handler)
      } catch {
        /* unsupported action */
      }
    }
    bind('play', () => void this.play())
    bind('pause', () => this.pause())
    bind('stop', () => this.pause())
    bind('previoustrack', () => this.sessionHandlers?.previous())
    bind('nexttrack', () => this.sessionHandlers?.next())
    bind('seekbackward', (d) => this.seekBy(-(d.seekOffset || 10)))
    bind('seekforward', (d) => this.seekBy(d.seekOffset || 10))
    bind('seekto', (d) => {
      if (typeof d.seekTime === 'number') this.seek(d.seekTime)
    })
  }

  private setSessionState(state: MediaSessionPlaybackState) {
    try {
      if ('mediaSession' in navigator) navigator.mediaSession.playbackState = state
    } catch {
      /* ignore */
    }
  }

  private updatePositionState(p: ProgressState) {
    try {
      if ('mediaSession' in navigator && 'setPositionState' in navigator.mediaSession && p.duration > 0 && Number.isFinite(p.duration)) {
        navigator.mediaSession.setPositionState({
          duration: p.duration,
          playbackRate: this.audio.playbackRate || 1,
          position: Math.min(p.currentTime, p.duration),
        })
      }
    } catch {
      /* ignore */
    }
  }
}

export const audioEngine = AudioEngine.getInstance()
