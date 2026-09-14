import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Mic2, ExternalLink, Sparkles } from 'lucide-react'
import { useTrackLyrics } from '@/hooks/useQueries'
import { useProgressStore } from '@/stores/progressStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { usePlayerStore } from '@/stores/playerStore'
import { audioEngine } from '@/audio/AudioEngine'
import { cn } from '@/lib/cn'
import type { LyricLine, LyricWordSpan } from '@/schemas/track'
import { estimateLineWords } from '@/api/lyrics'
import { Skeleton } from '@/components/common/Skeleton'

function indexAt(lines: LyricLine[], t: number): number {
  let lo = 0
  let hi = lines.length - 1
  let ans = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (lines[mid].time <= t) {
      ans = mid
      lo = mid + 1
    } else {
      hi = mid - 1
    }
  }
  return ans
}

function useActiveLine(lines: LyricLine[], syncOffsetMs: number) {
  const offsetSec = syncOffsetMs / 1000
  return useProgressStore((s) => (lines.length ? indexAt(lines, s.currentTime + offsetSec + 0.15) : -1))
}

/** High-precision ticking component for the single active lyric line */
const ActiveLineRenderer: React.FC<{
  line: LyricLine
  syncOffsetMs: number
  glow: boolean
  style: string
  position: string
}> = ({ line, syncOffsetMs, glow, style, position }) => {
  const isPlaying = usePlayerStore((p) => p.isPlaying)
  const [time, setTime] = useState(() => (audioEngine.getProgress().currentTime || 0) + syncOffsetMs / 1000)

  useEffect(() => {
    if (!isPlaying) {
      setTime((audioEngine.getProgress().currentTime || 0) + syncOffsetMs / 1000)
      return
    }
    let animId: number
    const tick = () => {
      setTime((audioEngine.getProgress().currentTime || 0) + syncOffsetMs / 1000)
      animId = requestAnimationFrame(tick)
    }
    animId = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(animId)
  }, [isPlaying, syncOffsetMs])

  const words = useMemo(() => {
    return line.spans && line.spans.length > 0 ? line.spans : estimateLineWords(line, line.duration || 3.5)
  }, [line])

  // Apple Music V2: Letter-by-letter karaoke glow sweep (piTube AppleV2LetterLine)
  if (style === 'apple_music_v2') {
    return (
      <span
        className={cn(
          'inline-flex flex-wrap items-baseline',
          position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
        )}
      >
        {words.map((w, wIdx) => {
          const chars = w.text.split('')
          const wDur = w.duration || 0.35
          const charDur = chars.length > 0 ? wDur / chars.length : 0.05

          return (
            <span key={wIdx} className="inline-flex mr-[0.3em] last:mr-0">
              {chars.map((ch, cIdx) => {
                const cStart = w.time + cIdx * charDur
                const cEnd = cStart + charDur
                const cProg = Math.max(0, Math.min(1, (time - cStart) / charDur))
                const isPassed = time >= cEnd
                const isActiveChar = time >= cStart && time < cEnd
                const alpha = isPassed ? 1 : Math.max(0.35, 0.35 + 0.65 * cProg)

                return (
                  <span
                    key={cIdx}
                    style={{
                      opacity: alpha,
                      color: isPassed ? 'var(--color-primary)' : isActiveChar ? 'var(--color-primary)' : 'var(--color-on-surface)',
                      textShadow:
                        glow && (isPassed || isActiveChar)
                          ? '0 0 16px var(--color-primary), 0 0 32px var(--color-primary-container)'
                          : undefined,
                    }}
                    className="transition-colors duration-75 inline-block"
                  >
                    {ch}
                  </span>
                )
              })}
            </span>
          )
        })}
      </span>
    )
  }

  // Lyrics V2 Fluid: Smooth liquid fill sweep across words (piTube LyricsV2FillLine single-layer engine)
  if (style === 'lyrics_v2_fluid') {
    return (
      <span
        className={cn(
          'inline-flex flex-wrap items-baseline font-bold',
          position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
        )}
      >
        {words.map((w, wIdx) => {
          const wDur = w.duration || 0.35
          const wProg = Math.max(0, Math.min(1, (time - w.time) / wDur))
          const isPassed = time >= w.time + wDur
          const isWordActive = time >= w.time && time < w.time + wDur
          const bounce = isWordActive ? Math.sin(wProg * Math.PI) * 2 : 0

          // Single-layer fill: trailing-feather gradient eliminates ghost double-text artifact
          if (isPassed) {
            return (
              <span
                key={wIdx}
                className="font-bold text-primary mr-[0.3em] last:mr-0 inline-block"
                style={{
                  textShadow: glow
                    ? '0 0 16px var(--color-primary), 0 0 32px var(--color-primary-container)'
                    : undefined,
                }}
              >
                {w.text}
              </span>
            )
          }

          if (!isWordActive) {
            return (
              <span
                key={wIdx}
                className="font-bold text-on-surface opacity-35 mr-[0.3em] last:mr-0 inline-block select-none"
              >
                {w.text}
              </span>
            )
          }

          // Active filling word: smooth horizontal feathered gradient sweep (zero ghosting, zero background box)
          const p1 = Math.max(0, Math.round((wProg - 0.1) * 100))
          const p2 = Math.min(100, Math.round((wProg + 0.1) * 100))

          return (
            <span
              key={wIdx}
              className="font-bold mr-[0.3em] last:mr-0 inline-block transition-transform duration-75 select-none"
              style={{
                backgroundImage: `linear-gradient(90deg, var(--color-primary) 0%, var(--color-primary) ${p1}%, color-mix(in srgb, var(--color-on-surface) 35%, transparent) ${p2}%, color-mix(in srgb, var(--color-on-surface) 35%, transparent) 100%)`,
                WebkitBackgroundClip: 'text',
                backgroundClip: 'text',
                WebkitTextFillColor: 'transparent',
                color: 'transparent',
                transform: bounce ? `translateY(-${bounce}px)` : undefined,
              }}
            >
              {w.text}
            </span>
          )
        })}
      </span>
    )
  }

  // Apple Music: Word-level scale & illumination with warm shadow
  if (style === 'apple_music') {
    return (
      <span
        className={cn(
          'inline-flex flex-wrap items-baseline',
          position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
        )}
      >
        {words.map((w, wIdx) => {
          const wDur = w.duration || 0.35
          const isWordActive = time >= w.time && time < w.time + wDur
          const isPassed = time >= w.time + wDur

          return (
            <span
              key={wIdx}
              className={cn(
                'inline-block mr-[0.3em] last:mr-0 transition-all duration-150',
                isWordActive
                  ? 'text-primary scale-105 font-extrabold'
                  : isPassed
                    ? 'text-on-surface opacity-100'
                    : 'text-on-surface-variant opacity-40'
              )}
              style={{
                textShadow:
                  glow && isWordActive
                    ? '0 0 20px var(--color-primary), 0 0 35px var(--color-primary-container)'
                    : undefined,
              }}
            >
              {w.text}
            </span>
          )
        })}
      </span>
    )
  }

  // Fallback: Classic or Glow
  return (
    <span
      className="inline-block text-on-surface font-bold"
      style={{
        textShadow:
          glow || style === 'glow'
            ? '0 0 22px var(--color-primary), 0 0 38px var(--color-primary-container)'
            : undefined,
      }}
    >
      {line.text}
    </span>
  )
}

interface LyricsViewProps {
  trackId: string
  className?: string
  /** Override size if needed */
  size?: 'md' | 'lg'
}

export const LyricsView: React.FC<LyricsViewProps> = ({ trackId, className, size: propSize }) => {
  const settings = useSettingsStore()
  const { data, isLoading, isError } = useTrackLyrics(trackId, settings.lyricsProvider)
  const lines = useMemo(() => data?.lines ?? [], [data])
  const active = useActiveLine(lines, settings.lyricsSyncOffsetMs)

  const containerRef = useRef<HTMLDivElement>(null)
  const [userScrolled, setUserScrolled] = useState(false)
  const scrollTimer = useRef<number | null>(null)

  // Typography & Layout configs from settings
  const position = settings.lyricsTextPosition
  const glow = settings.lyricsGlow
  const blur = settings.lyricsBlur
  const animStyle = settings.lyricsAnimationStyle
  const autoScroll = settings.lyricsAutoScroll
  const seekOnClick = settings.lyricsSeekOnClick
  const spacing = settings.lyricsLineSpacing

  const sizeChoice = propSize ? (propSize === 'lg' ? 'xl' : 'md') : settings.lyricsTextSize
  const fontSizeClass = {
    sm: 'text-lg sm:text-xl',
    md: 'text-xl sm:text-2xl',
    lg: 'text-2xl sm:text-3xl',
    xl: 'text-3xl sm:text-4xl',
  }[sizeChoice]

  const alignClass = {
    left: 'text-left items-start',
    center: 'text-center items-center',
    right: 'text-right items-end',
  }[position]

  // Auto-scroll active line to center/upper third unless user is actively scrolling
  useEffect(() => {
    if (!autoScroll || active < 0 || userScrolled || !containerRef.current) return
    const el = containerRef.current.querySelector<HTMLElement>(`[data-line="${active}"]`)
    if (!el) return
    const c = containerRef.current
    const target = el.offsetTop - c.clientHeight * 0.38 + el.offsetHeight / 2
    c.scrollTo({ top: target, behavior: 'smooth' })
  }, [active, userScrolled, autoScroll])

  // Reset scroll state on track change
  useEffect(() => {
    setUserScrolled(false)
    containerRef.current?.scrollTo({ top: 0 })
  }, [trackId])

  const onScroll = () => {
    setUserScrolled(true)
    if (scrollTimer.current) clearTimeout(scrollTimer.current)
    scrollTimer.current = window.setTimeout(() => setUserScrolled(false), 3500)
  }

  if (isLoading) {
    return (
      <div className={cn('flex flex-col gap-6 p-8 max-w-2xl mx-auto', className)}>
        {[85, 65, 78, 55, 70, 60].map((w, i) => (
          <Skeleton key={i} variant="text" className="h-8 rounded-xl" style={{ width: `${w}%` }} />
        ))}
      </div>
    )
  }

  if (isError || !data || (!data.synced && !data.plain)) {
    return (
      <div className={cn('flex-1 flex flex-col items-center justify-center text-center p-8 text-on-surface-variant', className)}>
        <Mic2 className="size-10 mb-3 opacity-50" />
        <p className="type-title-md text-on-surface font-semibold">No lyrics available</p>
        <p className="type-body-sm mt-1 text-on-surface-variant/80 max-w-sm">
          Lyrics could not be found via {settings.lyricsProvider === 'musixmatch' ? 'Musixmatch' : settings.lyricsProvider === 'lrclib' ? 'LRCLIB' : 'Musixmatch or LRCLIB'}.
        </p>
      </div>
    )
  }

  if (!data.synced) {
    return (
      <div ref={containerRef} className={cn('flex-1 overflow-y-auto px-6 sm:px-10 py-8', className)}>
        <p className={cn('whitespace-pre-wrap text-on-surface leading-relaxed font-semibold', fontSizeClass)}>
          {data.plain}
        </p>
        {data.source && (
          <div className="mt-8 pt-4 border-t border-outline-variant/40 type-label-md text-on-surface-variant flex items-center gap-1.5">
            <ExternalLink className="size-3.5" />
            <span>Source: {data.source}</span>
          </div>
        )}
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      onWheel={onScroll}
      onTouchMove={onScroll}
      className={cn(
        'flex-1 overflow-y-auto px-6 sm:px-12 py-[40%] scrollbar-none [mask-image:linear-gradient(to_bottom,transparent,#000_10%,#000_88%,transparent)]',
        className
      )}
    >
      <div className={cn('flex flex-col select-none transition-all duration-300', alignClass)} style={{ gap: `${(spacing - 0.4) * 1.5}rem` }}>
        {lines.map((line, i) => {
          const isActive = i === active
          const isPast = i < active

          return (
            <button
              key={i}
              data-line={i}
              disabled={!seekOnClick}
              onClick={() => {
                if (seekOnClick) audioEngine.seek(line.time)
              }}
              style={{
                lineHeight: spacing,
              }}
              className={cn(
                'rounded-2xl transition-[transform,opacity,filter] duration-300 origin-left block py-1.5 px-3 -mx-3 outline-none text-left',
                fontSizeClass,
                seekOnClick ? 'cursor-pointer hover:opacity-90' : 'cursor-default',
                isActive
                  ? 'opacity-100 scale-[1.03] z-10'
                  : cn(
                      isPast ? 'opacity-35 hover:opacity-75' : 'opacity-45 hover:opacity-85',
                      blur && 'blur-[1.5px]'
                    )
              )}
            >
              {isActive ? (
                <ActiveLineRenderer
                  line={line}
                  syncOffsetMs={settings.lyricsSyncOffsetMs}
                  glow={glow}
                  style={animStyle}
                  position={position}
                />
              ) : (
                <span className={cn('font-semibold text-on-surface-variant transition-colors duration-200')}>
                  {line.text || '♪'}
                </span>
              )}
            </button>
          )
        })}

        {/* Source metadata pill */}
        {data.source && (
          <div className="mt-8 pt-6 border-t border-outline-variant/30 flex items-center gap-2 type-label-sm text-on-surface-variant/70">
            <Sparkles className="size-3.5 text-primary" />
            <span>
              Synced with <strong>{data.source}</strong> {data.kind === 'richsync' ? '(RichSync word-level)' : '(LRC timestamps)'}
            </span>
          </div>
        )}
      </div>
    </div>
  )
}
