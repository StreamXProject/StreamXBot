/**
 * Listening event recorder — feeds the recap engine.
 *
 * One event per "track session": starts when a track begins playing, accumulates actual
 * played milliseconds from the engine's progress ticks (so seeking/pausing don't inflate it)
 * and is flushed when the track changes, ends, or the page is hidden/closed.
 * Events carry a client-generated id so re-sends after a flaky network never double-count.
 */
import { audioEngine } from '@/audio/AudioEngine'
import { useSettingsStore } from '@/stores/settingsStore'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { postListeningEvents, type ListeningEventPayload } from '@/features/recap/api'

const QUEUE_KEY = 'webx.listening.queue'
const MIN_STORE_MS = 5_000 // shorter than this is noise, not even a skip
const FLUSH_INTERVAL = 20_000

interface Session { id: string; trackId: string; startedAt: number; durationMs: number; playedMs: number; lastTime: number | null; ended: boolean }

let started = false
let session: Session | null = null
let flushTimer: number | null = null

const uid = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`)
const sessionId = uid()

function loadQueue(): ListeningEventPayload[] {
  try {
    return JSON.parse(localStorage.getItem(QUEUE_KEY) || '[]') as ListeningEventPayload[]
  } catch {
    return []
  }
}
function saveQueue(q: ListeningEventPayload[]) {
  try {
    localStorage.setItem(QUEUE_KEY, JSON.stringify(q.slice(-500)))
  } catch {}
}

function canRecord(): boolean {
  const auth = useAuthStore.getState()
  return useSettingsStore.getState().recapsEnabled && sessionKind(auth.token, auth.user) === 'user'
}

function finalize(skippedByUser: boolean) {
  const s = session
  session = null
  if (!s || s.playedMs < MIN_STORE_MS || !canRecord()) return
  const completed = s.ended || (s.durationMs > 0 && s.playedMs >= 0.8 * s.durationMs)
  const ev: ListeningEventPayload = {
    id: s.id,
    track_id: s.trackId,
    played_at: Date.now() / 1000,
    started_at: s.startedAt / 1000,
    played_ms: Math.round(s.playedMs),
    duration_ms: Math.round(s.durationMs),
    completed,
    skipped: !completed && skippedByUser && s.playedMs < 30_000,
    source: 'server',
    session_id: sessionId,
  }
  const q = loadQueue()
  q.push(ev)
  saveQueue(q)
  scheduleFlush(1500)
}

let flushing = false
export async function flushListeningEvents(): Promise<void> {
  if (flushing) return
  const q = loadQueue()
  if (!q.length || !canRecord()) return
  flushing = true
  try {
    await postListeningEvents(q.slice(0, 200))
    saveQueue(loadQueue().filter((e) => !q.slice(0, 200).some((s) => s.id === e.id)))
  } catch (err) {
    const status = (err as { status?: number })?.status
    if (status === 404 || status === 405) {
      saveQueue([])
    }
  } finally {
    flushing = false
  }
}

function scheduleFlush(delay = FLUSH_INTERVAL) {
  if (flushTimer) return
  flushTimer = window.setTimeout(() => {
    flushTimer = null
    void flushListeningEvents()
  }, delay)
}

/** Idempotent — wire to the audio engine once at startup. */
export function startListeningRecorder(): void {
  if (started || typeof window === 'undefined') return
  started = true

  audioEngine.subscribeState((st) => {
    const t = st.currentTrack
    if (!t) {
      if (session) finalize(true)
      return
    }
    if (!session || session.trackId !== t.id) {
      if (session) finalize(!session.ended)
      session = { id: uid(), trackId: t.id, startedAt: Date.now(), durationMs: (t.duration_sec || 0) * 1000, playedMs: 0, lastTime: null, ended: false }
    }
  })

  audioEngine.subscribeProgress((p) => {
    const s = session
    if (!s) return
    if (p.duration > 0) s.durationMs = p.duration * 1000
    const st = audioEngine.getState()
    if (st.isPlaying && s.lastTime !== null) {
      const delta = p.currentTime - s.lastTime
      // only count forward, natural progress (ignores seeks / rate jumps)
      if (delta > 0 && delta < 2.5) s.playedMs += delta * 1000
    }
    s.lastTime = p.currentTime
  })

  audioEngine.onTrackEnd(() => {
    if (session) session.ended = true
  })

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      // don't finalise (the track may keep playing in the background) — just persist what we have
      if (session && session.playedMs >= MIN_STORE_MS) void flushListeningEvents()
    }
  })
  window.addEventListener('pagehide', () => {
    if (session) finalize(false)
    // best effort — anything that doesn't make it stays queued in localStorage for next launch
    if (loadQueue().length && canRecord()) void flushListeningEvents()
  })

  scheduleFlush()
  window.setInterval(() => void flushListeningEvents(), FLUSH_INTERVAL)
}
