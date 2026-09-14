import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { X, Pause, Play, ChevronLeft, ChevronRight, Share2, ImageDown, RotateCcw, Link2, Check } from 'lucide-react'
import { AmbientBackdrop } from '@/components/player/AmbientBackdrop'
import { IconButton, Button } from '@/components/md3'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { toast } from '@/stores/uiStore'
import { buildRecapStory } from '../story/buildRecapStory'
import { shareRecap } from '../api'
import { downloadShareCard } from '../shareCard'
import type { RecapScene, RecapSnapshot } from '../types'
import { Scene } from './scenes'

/**
 * Story-style recap player. One timeline drives every scene (scenes never own timers):
 * segmented progress on top, tap zones left/right, swipe up/down, keyboard, pause on hold.
 */
export const RecapPlayer: React.FC<{ snapshot: RecapSnapshot & { demo?: boolean; playable?: boolean } }> = ({ snapshot }) => {
  const isDemo = Boolean(snapshot.demo)
  const soundtrackEnabled = !isDemo || snapshot.playable !== false
  const navigate = useNavigate()
  const story = useMemo(() => buildRecapStory(snapshot), [snapshot])
  const scenes = story.scenes
  const [index, setIndex] = useState(0)
  const [started, setStarted] = useState(false)
  const [paused, setPaused] = useState(false)
  const [progress, setProgress] = useState(0) // 0..1 within the current scene
  const [shareUrl, setShareUrl] = useState<string | null>(null)
  const [busy, setBusy] = useState<'share' | 'image' | null>(null)
  const raf = useRef<number | null>(null)
  const sceneStart = useRef(0)
  const elapsedBeforePause = useRef(0)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const isPlaying = usePlayerStore((s) => s.isPlaying)
  const togglePlay = usePlayerStore((s) => s.togglePlay)

  const scene: RecapScene = scenes[Math.min(index, scenes.length - 1)]!
  const isLast = index >= scenes.length - 1

  const close = useCallback(() => navigate({ to: '/recaps' }), [navigate])

  const goTo = useCallback(
    (i: number) => {
      const next = Math.max(0, Math.min(scenes.length - 1, i))
      setIndex(next)
      setProgress(0)
      elapsedBeforePause.current = 0
      sceneStart.current = performance.now()
    },
    [scenes.length]
  )
  const next = useCallback(() => {
    if (isLast) setPaused(true)
    else goTo(index + 1)
  }, [index, isLast, goTo])
  const prev = useCallback(() => goTo(index - 1), [index, goTo])

  /* ---------- timeline ---------- */
  useEffect(() => {
    if (!started || paused) return
    sceneStart.current = performance.now() - elapsedBeforePause.current
    const tick = (now: number) => {
      const elapsed = now - sceneStart.current
      const p = Math.min(1, elapsed / scene.durationMs)
      setProgress(p)
      if (p >= 1) {
        if (isLast) {
          setPaused(true)
          return
        }
        goTo(index + 1)
        return
      }
      raf.current = requestAnimationFrame(tick)
    }
    raf.current = requestAnimationFrame(tick)
    return () => {
      if (raf.current) cancelAnimationFrame(raf.current)
      elapsedBeforePause.current = performance.now() - sceneStart.current
    }
  }, [started, paused, index, scene.durationMs, isLast, goTo])

  /* preload the next scene's artwork only */
  useEffect(() => {
    const url = scenes[index + 1]?.artwork
    if (url) {
      const img = new Image()
      img.decoding = 'async'
      img.src = url
    }
  }, [index, scenes])

  /* ---------- start (also kicks off the soundtrack after a user gesture) ---------- */
  const start = async () => {
    setStarted(true)
    sceneStart.current = performance.now()
    const tracks = soundtrackEnabled ? story.soundtrack.tracks.map((t) => t.track) : []
    if (tracks.length) {
      try {
        await playTrackWithQueue(tracks, 0, { type: 'custom', id: `recap:${snapshot.period}`, title: `${snapshot.label} recap` })
      } catch {
        /* the recap still plays without music */
      }
    }
  }

  /* ---------- keyboard ---------- */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
      if (e.key === 'ArrowRight' || e.key === 'Enter') {
        e.preventDefault()
        if (started) next()
        else void start()
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault()
        prev()
      } else if (e.key === ' ') {
        e.preventDefault()
        if (started) setPaused((p) => !p)
      } else if (e.key === 'Escape') close()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [started, next, prev, close])

  /* ---------- touch: swipe up/down, hold to pause ---------- */
  const touch = useRef<{ x: number; y: number; t: number; id: number } | null>(null)
  const holdTimer = useRef<number | null>(null)
  const held = useRef(false)
  const onPointerDown = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest('button,a')) return
    touch.current = { x: e.clientX, y: e.clientY, t: Date.now(), id: e.pointerId }
    held.current = false
    holdTimer.current = window.setTimeout(() => {
      held.current = true
      setPaused(true)
    }, 250)
  }
  const onPointerUp = (e: React.PointerEvent) => {
    const t = touch.current
    touch.current = null
    if (holdTimer.current) window.clearTimeout(holdTimer.current)
    if (!t || t.id !== e.pointerId) return
    if (held.current) {
      setPaused(false)
      return
    }
    if (!started) {
      void start()
      return
    }
    const dx = e.clientX - t.x
    const dy = e.clientY - t.y
    if (Math.abs(dy) > 60 && Math.abs(dy) > Math.abs(dx)) {
      if (dy < 0) next()
      else prev()
      return
    }
    if (Math.abs(dx) > 60) {
      if (dx < 0) next()
      else prev()
      return
    }
    // tap zones
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const xRatio = (e.clientX - rect.left) / rect.width
    if (xRatio < 0.3) prev()
    else next()
  }

  /* ---------- share ---------- */
  const doShare = async () => {
    if (isDemo) {
      toast('Sample recap — share links are created for real recaps only')
      return
    }
    setBusy('share')
    try {
      const { url } = await shareRecap(snapshot.type, snapshot.period)
      setShareUrl(url)
      if (navigator.share) {
        await navigator.share({ title: `My ${snapshot.label} in WebX`, url }).catch(() => {})
      } else {
        await navigator.clipboard.writeText(url)
        toast('Share link copied')
      }
    } catch (e) {
      toast(`Couldn't create share link: ${(e as Error).message}`, { variant: 'error' })
    } finally {
      setBusy(null)
    }
  }
  const doImage = async () => {
    setBusy('image')
    try {
      await downloadShareCard(snapshot)
    } catch (e) {
      toast(`Couldn't render image: ${(e as Error).message}`, { variant: 'error' })
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="recap fixed inset-0 z-50 bg-surface text-on-surface select-none overflow-hidden" role="dialog" aria-label={`${snapshot.label} recap`}>
      <AmbientBackdrop src={scene.artwork ?? null} enabled />

      {/* story canvas: full viewport on phones, centred 9:16 card on wide screens */}
      <div className="relative z-10 h-full w-full flex items-center justify-center md:p-6">
        <div
          className="recap-canvas relative h-full w-full md:h-[min(100%,920px)] md:max-w-[520px] md:rounded-3xl md:overflow-hidden md:shadow-md3-3 md:border md:border-outline-variant/30 flex flex-col"
          onPointerDown={onPointerDown}
          onPointerUp={onPointerUp}
          onPointerCancel={() => { touch.current = null; if (holdTimer.current) window.clearTimeout(holdTimer.current); if (held.current) setPaused(false) }}
          style={{ paddingTop: 'env(safe-area-inset-top, 0px)', paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
        >
          {/* segmented progress */}
          <div className="flex gap-1 px-3 pt-3 shrink-0" aria-hidden>
            {scenes.map((sc, i) => (
              <div key={sc.id} className="h-[3px] flex-1 rounded-full bg-on-surface/20 overflow-hidden">
                <div className="h-full bg-on-surface rounded-full origin-left" style={{ transform: `scaleX(${i < index ? 1 : i === index ? progress : 0})` }} />
              </div>
            ))}
          </div>

          {/* top bar */}
          <div className="flex items-center justify-between px-2 pt-1 shrink-0">
            <div className="px-2 flex items-center gap-2 min-w-0">
              <p className="type-label-md text-on-surface-variant truncate">{snapshot.label}</p>
              {isDemo && <span className="shrink-0 h-5 px-2 rounded-full bg-tertiary-container text-on-tertiary-container type-label-sm inline-flex items-center">Sample</span>}
            </div>
            <div className="flex items-center">
              {started && (
                <IconButton label={paused ? 'Resume' : 'Pause'} size="md" onClick={() => setPaused((p) => !p)}>
                  {paused ? <Play className="fill-current" /> : <Pause className="fill-current" />}
                </IconButton>
              )}
              <IconButton label="Close" size="md" onClick={close}>
                <X />
              </IconButton>
            </div>
          </div>

          {/* scene */}
          <div className="relative flex-1 min-h-0 px-6 sm:px-8 pb-6 flex flex-col">
            {!started ? (
              <div className="flex-1 flex flex-col items-center justify-center text-center gap-6 recap-enter-fade">
                <p className="type-label-lg text-primary uppercase tracking-[0.2em]">{snapshot.label}</p>
                <h1 className="type-display-sm sm:type-display-md text-on-surface">Your recap is ready</h1>
                <p className="type-body-lg text-on-surface-variant max-w-xs">{snapshot.stats.totalPlays > 0 ? `${snapshot.stats.totalMinutes.toLocaleString()} minutes, ${snapshot.stats.uniqueArtists} artists. Tap to start.` : 'Nothing was played in this period yet.'}</p>
                <Button size="lg" icon={<Play className="fill-current" />} onClick={() => void start()}>{isDemo ? 'Play sample recap' : 'Start recap'}</Button>
                {isDemo && <p className="type-body-sm text-on-surface-variant/70 max-w-xs">Made-up numbers on your recent tracks — just to show how a recap looks and moves.</p>}
                {soundtrackEnabled && story.soundtrack.tracks.length > 0 && <p className="type-body-sm text-on-surface-variant/70">Plays your top tracks while you watch</p>}
              </div>
            ) : (
              <Scene key={scene.id} scene={scene} progress={progress} />
            )}

            {/* outro actions */}
            {started && scene.type === 'outro' && (
              <div className="shrink-0 pt-4 flex flex-wrap items-center justify-center gap-2 recap-enter-slideUp" style={{ animationDelay: '600ms' }}>
                {snapshot.stats.totalPlays > 0 && (
                  <>
                    <Button variant="filled" icon={shareUrl ? <Check /> : <Share2 />} loading={busy === 'share'} onClick={() => void doShare()}>{shareUrl ? 'Link ready' : 'Share'}</Button>
                    <Button variant="tonal" icon={<ImageDown />} loading={busy === 'image'} onClick={() => void doImage()}>Save image</Button>
                  </>
                )}
                <Button variant="text" icon={<RotateCcw />} onClick={() => { goTo(0); setPaused(false) }}>Replay</Button>
              </div>
            )}
            {shareUrl && scene.type === 'outro' && (
              <button onClick={() => void navigator.clipboard.writeText(shareUrl).then(() => toast('Link copied'))} className="mt-3 mx-auto inline-flex items-center gap-2 h-9 px-3 rounded-full glass border border-outline-variant/40 type-label-md text-on-surface-variant max-w-full">
                <Link2 className="size-4 shrink-0" /> <span className="truncate">{shareUrl.replace(/^https?:\/\//, '')}</span>
              </button>
            )}
          </div>

          {/* desktop arrows */}
          {started && (
            <>
              <button onClick={prev} disabled={index === 0} aria-label="Previous scene" className="hidden md:flex absolute left-2 top-1/2 -translate-y-1/2 size-10 items-center justify-center rounded-full glass text-on-surface-variant state-layer disabled:opacity-0">
                <ChevronLeft />
              </button>
              <button onClick={next} disabled={isLast} aria-label="Next scene" className="hidden md:flex absolute right-2 top-1/2 -translate-y-1/2 size-10 items-center justify-center rounded-full glass text-on-surface-variant state-layer disabled:opacity-0">
                <ChevronRight />
              </button>
            </>
          )}

          {/* soundtrack chip */}
          {started && soundtrackEnabled && story.soundtrack.tracks.length > 0 && (
            <button onClick={togglePlay} className="absolute bottom-3 left-3 inline-flex items-center gap-2 h-8 px-3 rounded-full glass border border-outline-variant/40 type-label-md text-on-surface-variant" style={{ marginBottom: 'env(safe-area-inset-bottom, 0px)' }}>
              {isPlaying ? <Pause className="size-3.5 fill-current" /> : <Play className="size-3.5 fill-current" />} Soundtrack
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
