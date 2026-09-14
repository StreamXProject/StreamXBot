import React, { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  Mic2,
  Sparkles,
  AudioLines,
  Music,
  AlignLeft,
  AlignCenter,
  AlignRight,
  Eye,
  Type,
  Clock,
  Play,
  Pause,
  RotateCcw,
  Layers,
  Flame,
  Waves,
  Wind,
  FileText,
  Check,
} from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Slider, SegmentedButton, IconButton } from '@/components/md3'
import {
  useSettingsStore,
  type LyricsProvider,
  type LyricsAnimationStyle,
  type LyricsTextPosition,
  type LyricsTextSize,
} from '@/stores/settingsStore'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/settings/lyrics')({
  component: LyricsSettings,
})

const PREVIEW_LINES = [
  { text: 'Never gonna give you up', duration: 2.8 },
  { text: 'Never gonna let you down', duration: 2.8 },
  { text: 'Never gonna run around and desert you', duration: 3.6 },
]

function LyricsPreviewCard() {
  const animStyle = useSettingsStore((s) => s.lyricsAnimationStyle)
  const position = useSettingsStore((s) => s.lyricsTextPosition)
  const glow = useSettingsStore((s) => s.lyricsGlow)
  const blur = useSettingsStore((s) => s.lyricsBlur)
  const textSize = useSettingsStore((s) => s.lyricsTextSize)
  const spacing = useSettingsStore((s) => s.lyricsLineSpacing)

  const [activeLine, setActiveLine] = useState(1)
  const [lineProgress, setLineProgress] = useState(0.4)
  const [playing, setPlaying] = useState(true)

  useEffect(() => {
    if (!playing) return
    let animId: number
    let start = performance.now()
    const lineDuration = PREVIEW_LINES[activeLine].duration * 1000

    const frame = (now: number) => {
      const elapsed = now - start
      const prog = (elapsed / lineDuration) % 1
      setLineProgress(prog)

      if (elapsed >= lineDuration) {
        setActiveLine((prev) => (prev + 1) % PREVIEW_LINES.length)
        start = now
      }
      animId = requestAnimationFrame(frame)
    }

    animId = requestAnimationFrame(frame)
    return () => cancelAnimationFrame(animId)
  }, [playing, activeLine])

  const curLine = PREVIEW_LINES[activeLine]
  const words = curLine.text.split(' ')
  const totalChars = curLine.text.length

  const fontSizeClass = {
    sm: 'text-base sm:text-lg',
    md: 'text-lg sm:text-xl',
    lg: 'text-xl sm:text-2xl',
    xl: 'text-2xl sm:text-3xl',
  }[textSize]

  const textAlignClass = {
    left: 'text-left',
    center: 'text-center',
    right: 'text-right',
  }[position]

  const justifyClass = {
    left: 'justify-start',
    center: 'justify-center',
    right: 'justify-end',
  }[position]

  const alignClass = {
    left: 'text-left items-start',
    center: 'text-center items-center',
    right: 'text-right items-end',
  }[position]

  return (
    <div className="rounded-lg bg-surface-low overflow-hidden">
      <div className="relative p-6 sm:p-8 min-h-[180px] flex flex-col justify-center overflow-hidden">
        <IconButton label={playing ? 'Pause preview' : 'Play preview'} size="sm" onClick={() => setPlaying((p) => !p)} className="absolute top-2 right-2 text-on-surface-variant">
          {playing ? <Pause /> : <Play className="fill-current" />}
        </IconButton>
        <div className={cn('flex flex-col select-none transition-all duration-300 w-full', alignClass)} style={{ gap: `${(spacing - 0.5) * 1.25}rem` }}>
          {PREVIEW_LINES.map((l, i) => {
            const isActive = i === activeLine
            const isPast = i < activeLine

            if (!isActive) {
              return (
                <div
                  key={i}
                  className={cn(
                    'font-semibold transition-all duration-300 leading-snug w-full',
                    textAlignClass,
                    fontSizeClass,
                    isPast ? 'text-on-surface-variant/35' : 'text-on-surface-variant/45',
                    blur && 'blur-[1.5px]'
                  )}
                >
                  {l.text}
                </div>
              )
            }

            // Apple Music V2: Letter-by-letter karaoke glow sweep
            if (animStyle === 'apple_music_v2') {
              let charAcc = 0
              return (
                <div
                  key={i}
                  className={cn(
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug w-full',
                    fontSizeClass,
                    justifyClass,
                    textAlignClass
                  )}
                >
                  {words.map((word, wIdx) => {
                    const wordChars = word.split('')
                    const wordSpan = (
                      <span key={wIdx} className="inline-flex mr-[0.3em] last:mr-0">
                        {wordChars.map((ch, cIdx) => {
                          const charIdx = charAcc + cIdx
                          const charStart = charIdx / totalChars
                          const charEnd = (charIdx + 1) / totalChars
                          const charProg = Math.max(0, Math.min(1, (lineProgress - charStart) / (charEnd - charStart)))
                          const isPassed = lineProgress >= charEnd
                          const isActiveChar = lineProgress >= charStart && lineProgress < charEnd
                          const alpha = isPassed ? 1 : Math.max(0.35, 0.35 + 0.65 * charProg)

                          return (
                            <span
                              key={cIdx}
                              style={{
                                opacity: alpha,
                                color: isPassed || isActiveChar ? 'var(--color-primary)' : 'var(--color-on-surface)',
                                textShadow:
                                  glow && (isPassed || isActiveChar)
                                    ? '0 0 16px var(--color-primary), 0 0 30px var(--color-primary-container)'
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
                    charAcc += wordChars.length + 1
                    return wordSpan
                  })}
                </div>
              )
            }

            // Lyrics V2 Fluid: Smooth liquid fill sweep across words (piTube LyricsV2FillLine single-layer engine)
            if (animStyle === 'lyrics_v2_fluid') {
              return (
                <div
                  key={i}
                  className={cn(
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug w-full',
                    fontSizeClass,
                    justifyClass,
                    textAlignClass
                  )}
                >
                  {words.map((word, wIdx) => {
                    const wordStart = wIdx / words.length
                    const wordEnd = (wIdx + 1) / words.length
                    const wProg = Math.max(0, Math.min(1, (lineProgress - wordStart) / (wordEnd - wordStart)))
                    const isPassed = lineProgress >= wordEnd
                    const isWordActive = lineProgress >= wordStart && lineProgress < wordEnd
                    const bounce = isWordActive ? Math.sin(wProg * Math.PI) * 2 : 0

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
                          {word}
                        </span>
                      )
                    }

                    if (!isWordActive) {
                      return (
                        <span
                          key={wIdx}
                          className="font-bold text-on-surface opacity-35 mr-[0.3em] last:mr-0 inline-block select-none"
                        >
                          {word}
                        </span>
                      )
                    }

                    // Single-layer fill: smooth horizontal feathered gradient sweep (zero ghosting, zero background box)
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
                        {word}
                      </span>
                    )
                  })}
                </div>
              )
            }

            // Apple Music: Word-level smooth scale bump & illumination
            if (animStyle === 'apple_music') {
              return (
                <div
                  key={i}
                  className={cn(
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug w-full',
                    fontSizeClass,
                    justifyClass,
                    textAlignClass
                  )}
                >
                  {words.map((word, wIdx) => {
                    const wordStart = wIdx / words.length
                    const wordEnd = (wIdx + 1) / words.length
                    const wProg = Math.max(0, Math.min(1, (lineProgress - wordStart) / (wordEnd - wordStart)))
                    const isPassed = lineProgress >= wordEnd
                    const isWordActive = lineProgress >= wordStart && lineProgress < wordEnd

                    const bump = isWordActive ? Math.sin(wProg * Math.PI) : 0
                    const scale = isWordActive ? 1.0 + bump * 0.08 : 1.0
                    const translateY = isWordActive ? -bump * 2.5 : 0

                    return (
                      <span
                        key={wIdx}
                        className={cn(
                          'font-bold inline-block mr-[0.3em] last:mr-0 transition-colors duration-150 origin-bottom select-none',
                          isWordActive
                            ? 'text-primary opacity-100'
                            : isPassed
                              ? 'text-on-surface opacity-90'
                              : 'text-on-surface opacity-35'
                        )}
                        style={{
                          transform: isWordActive ? `translateY(${translateY}px) scale(${scale})` : undefined,
                          willChange: isWordActive ? 'transform' : undefined,
                          textShadow:
                            glow && isWordActive
                              ? '0 0 16px var(--color-primary), 0 0 28px var(--color-primary-container)'
                              : undefined,
                        }}
                      >
                        {word}
                      </span>
                    )
                  })}
                </div>
              )
            }

            return (
              <div
                key={i}
                className={cn(
                  'font-bold tracking-tight text-on-surface transition-all duration-300 scale-[1.02] leading-snug w-full',
                  fontSizeClass,
                  textAlignClass
                )}
                style={{
                  textShadow:
                    glow || animStyle === 'glow'
                      ? '0 0 20px var(--color-primary), 0 0 35px var(--color-primary-container)'
                      : undefined,
                }}
              >
                {l.text}
              </div>
            )
          })}
        </div>
      </div>

      <div className="h-[3px] w-full bg-on-surface/10">
        <div className="h-full bg-primary" style={{ width: `${Math.round(lineProgress * 100)}%` }} />
      </div>
    </div>
  )
}

const PROVIDERS: Array<{ id: LyricsProvider; name: string; desc: string; icon: React.FC<{ className?: string }> }> = [
  { id: 'auto', name: 'Auto', desc: 'Best available · recommended', icon: Sparkles },
  { id: 'betterlyrics', name: 'BetterLyrics', desc: 'Apple Music TTML, syllable timing', icon: Music },
  { id: 'musixmatch', name: 'Musixmatch', desc: 'Word-level RichSync', icon: Mic2 },
  { id: 'lrclib', name: 'LRCLIB', desc: 'Community synced LRC', icon: FileText },
  { id: 'kugou', name: 'KuGou', desc: 'Asian and international catalog', icon: AudioLines },
]

const STYLES: Array<{ id: LyricsAnimationStyle; name: string; desc: string; icon: React.FC<{ className?: string }> }> = [
  { id: 'apple_music_v2', name: 'Apple Music V2', desc: 'Letter-by-letter glow · recommended', icon: AudioLines },
  { id: 'lyrics_v2_fluid', name: 'Fluid', desc: 'Liquid gradient sweep', icon: Waves },
  { id: 'apple_music', name: 'Apple Music', desc: 'Word scale bump', icon: Music },
  { id: 'glow', name: 'Glow', desc: 'Soft ambient glow', icon: Flame },
  { id: 'fade', name: 'Fade', desc: 'Opacity crossfade', icon: Wind },
  { id: 'classic', name: 'Classic', desc: 'Plain line highlight', icon: FileText },
]

/** MD3 list item with a trailing radio — compact alternative to card grids */
function RadioRow<T extends string>({ id, name, desc, icon: Icon, value, onChange }: { id: T; name: string; desc: string; icon: React.FC<{ className?: string }>; value: T; onChange: (v: T) => void }) {
  const selected = value === id
  return (
    <div
      role="radio"
      tabIndex={0}
      aria-checked={selected}
      onClick={() => onChange(id)}
      onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), onChange(id))}
      className={cn('state-layer flex items-center gap-4 px-4 h-14 cursor-pointer outline-none select-none', selected && 'bg-secondary-container/30')}
    >
      <Icon className={cn('size-5 shrink-0', selected ? 'text-primary' : 'text-on-surface-variant')} />
      <div className="min-w-0 flex-1">
        <p className="type-body-lg text-on-surface truncate">{name}</p>
        <p className="type-body-sm text-on-surface-variant truncate">{desc}</p>
      </div>
      <span className={cn('size-5 rounded-full border-2 shrink-0 flex items-center justify-center transition-colors', selected ? 'border-primary bg-primary text-on-primary' : 'border-outline')}>
        {selected && <Check className="size-3" strokeWidth={3} />}
      </span>
    </div>
  )
}

function LyricsSettings() {
  const s = useSettingsStore()

  return (
    <SettingsPage title="Lyrics" description="Provider, sync style, typography">
      <LyricsPreviewCard />

      <SettingsSection title="Provider">
        <div role="radiogroup" aria-label="Lyrics provider" className="divide-y divide-outline-variant/60">
          {PROVIDERS.map((p) => (
            <RadioRow key={p.id} {...p} value={s.lyricsProvider} onChange={(v) => s.set('lyricsProvider', v)} />
          ))}
        </div>
      </SettingsSection>

      <SettingsSection title="Sync style">
        <div role="radiogroup" aria-label="Sync animation style" className="divide-y divide-outline-variant/60">
          {STYLES.map((st) => (
            <RadioRow key={st.id} {...st} value={s.lyricsAnimationStyle} onChange={(v) => s.set('lyricsAnimationStyle', v)} />
          ))}
        </div>
        <SettingRow icon={<Flame />} label="Glow" description="Highlight the active line" control={<Switch checked={s.lyricsGlow} onChange={(v) => s.set('lyricsGlow', v)} label="Glow" />} />
        <SettingRow icon={<Eye />} label="Blur inactive lines" description="Depth-of-field effect" control={<Switch checked={s.lyricsBlur} onChange={(v) => s.set('lyricsBlur', v)} label="Blur inactive lines" />} />
      </SettingsSection>

      <SettingsSection title="Text">
        <SettingRow
          icon={<AlignLeft />}
          label="Alignment"
          control={
            <SegmentedButton<LyricsTextPosition>
              size="sm"
              showCheck={false}
              value={s.lyricsTextPosition}
              onChange={(v) => s.set('lyricsTextPosition', v)}
              options={[
                { value: 'left', label: 'Left', icon: <AlignLeft /> },
                { value: 'center', label: 'Center', icon: <AlignCenter /> },
                { value: 'right', label: 'Right', icon: <AlignRight /> },
              ]}
            />
          }
        />
        <SettingRow
          icon={<Type />}
          label="Size"
          control={
            <SegmentedButton<LyricsTextSize>
              size="sm"
              showCheck={false}
              value={s.lyricsTextSize}
              onChange={(v) => s.set('lyricsTextSize', v)}
              options={[
                { value: 'sm', label: 'S' },
                { value: 'md', label: 'M' },
                { value: 'lg', label: 'L' },
                { value: 'xl', label: 'XL' },
              ]}
            />
          }
        />
        <SettingRow
          icon={<Layers />}
          label="Line spacing"
          description={`${s.lyricsLineSpacing.toFixed(1)}×`}
          stacked
          control={
            <div className="flex items-center gap-3 w-full">
              <Slider value={s.lyricsLineSpacing} min={1.0} max={2.2} step={0.1} onChange={(v) => s.set('lyricsLineSpacing', Math.round(v * 10) / 10)} className="flex-1" aria-label="Line spacing" />
              <button onClick={() => s.set('lyricsLineSpacing', 1.5)} disabled={s.lyricsLineSpacing === 1.5} className="type-label-lg text-primary px-2 disabled:opacity-40">Reset</button>
            </div>
          }
        />
      </SettingsSection>

      <SettingsSection title="Behaviour">
        <SettingRow label="Auto-scroll" description="Keep the active line centred" control={<Switch checked={s.lyricsAutoScroll} onChange={(v) => s.set('lyricsAutoScroll', v)} label="Auto-scroll" />} />
        <SettingRow label="Tap to seek" description="Jump playback to a line" control={<Switch checked={s.lyricsSeekOnClick} onChange={(v) => s.set('lyricsSeekOnClick', v)} label="Tap to seek" />} />
        <SettingRow
          icon={<Clock />}
          label="Sync offset"
          description={s.lyricsSyncOffsetMs === 0 ? '0 ms' : `${s.lyricsSyncOffsetMs > 0 ? '+' : ''}${s.lyricsSyncOffsetMs} ms · ${s.lyricsSyncOffsetMs > 0 ? 'earlier' : 'later'}`}
          stacked
          control={
            <div className="flex items-center gap-3 w-full">
              <Slider value={s.lyricsSyncOffsetMs} min={-3000} max={3000} step={50} onChange={(v) => s.set('lyricsSyncOffsetMs', Math.round(v / 50) * 50)} className="flex-1" aria-label="Sync offset" />
              <button onClick={() => s.set('lyricsSyncOffsetMs', 0)} disabled={s.lyricsSyncOffsetMs === 0} className="type-label-lg text-primary px-2 inline-flex items-center gap-1 disabled:opacity-40"><RotateCcw className="size-4" /> Reset</button>
            </div>
          }
        />
      </SettingsSection>
    </SettingsPage>
  )
}
