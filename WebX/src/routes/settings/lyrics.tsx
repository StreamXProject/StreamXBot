import React, { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  Mic2,
  Sparkles,
  Wand2,
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
} from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Slider, SegmentedButton } from '@/components/md3'
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

  const alignClass = {
    left: 'text-left items-start',
    center: 'text-center items-center',
    right: 'text-right items-end',
  }[position]

  return (
    <div className="rounded-3xl border border-outline-variant/60 bg-surface-container p-5 sm:p-6 shadow-md3-1 transition-colors">
      {/* MD3 Header bar */}
      <div className="flex items-center justify-between gap-3 mb-4">
        <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-surface-container-high text-on-surface-variant type-label-md border border-outline-variant/40">
          <Mic2 className="size-3.5 text-primary" />
          <span>Previewing: <strong className="text-on-surface capitalize font-semibold">{animStyle.replace(/_/g, ' ')}</strong></span>
        </div>
        <button
          type="button"
          onClick={() => setPlaying((p) => !p)}
          className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full bg-secondary-container state-layer text-on-secondary-container type-label-sm font-semibold transition-colors"
        >
          {playing ? <Pause className="size-3.5" /> : <Play className="size-3.5 fill-current" />}
          <span>{playing ? 'Pause' : 'Play'}</span>
        </button>
      </div>

      {/* Viewport showing actual player lyrics display */}
      <div className="rounded-2xl bg-surface-container-lowest border border-outline-variant/30 p-6 sm:p-8 min-h-[210px] flex flex-col justify-center overflow-hidden">
        <div className={cn('flex flex-col select-none transition-all duration-300', alignClass)} style={{ gap: `${(spacing - 0.5) * 1.25}rem` }}>
          {PREVIEW_LINES.map((l, i) => {
            const isActive = i === activeLine
            const isPast = i < activeLine

            if (!isActive) {
              return (
                <div
                  key={i}
                  className={cn(
                    'font-semibold transition-all duration-300 leading-snug',
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
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug',
                    fontSizeClass,
                    position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
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
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug',
                    fontSizeClass,
                    position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
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

            // Apple Music: Word-level smoothstep scale & illumination
            if (animStyle === 'apple_music') {
              const wordIdx = Math.floor(lineProgress * words.length)
              return (
                <div
                  key={i}
                  className={cn(
                    'font-bold tracking-tight transition-all duration-300 flex flex-wrap leading-snug',
                    fontSizeClass,
                    position === 'center' ? 'justify-center' : position === 'right' ? 'justify-end' : 'justify-start'
                  )}
                >
                  {words.map((word, wIdx) => {
                    const isWordPassed = wIdx < wordIdx
                    const isWordActive = wIdx === wordIdx
                    return (
                      <span
                        key={wIdx}
                        className={cn(
                          'inline-block mr-[0.3em] last:mr-0 transition-all duration-150',
                          isWordActive
                            ? 'text-primary scale-105 font-extrabold'
                            : isWordPassed
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
                        {word}
                      </span>
                    )
                  })}
                </div>
              )
            }

            // Classic / Glow / Fade
            return (
              <div
                key={i}
                className={cn(
                  'font-bold tracking-tight text-on-surface transition-all duration-300 scale-[1.02] leading-snug',
                  fontSizeClass
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

      {/* MD3 Timeline Progress Footer */}
      <div className="mt-4 pt-3 flex flex-col gap-2">
        <div className="h-1.5 w-full bg-surface-container-highest rounded-full overflow-hidden">
          <div
            className="h-full bg-primary rounded-full transition-all duration-75"
            style={{ width: `${Math.round(lineProgress * 100)}%` }}
          />
        </div>
        <div className="flex items-center justify-between text-xs text-on-surface-variant">
          <span>Rick Astley — Never Gonna Give You Up</span>
          <span className="font-mono">
            0:0{Math.floor(lineProgress * curLine.duration)} / 0:0{Math.ceil(curLine.duration)}
          </span>
        </div>
      </div>
    </div>
  )
}

function LyricsSettings() {
  const s = useSettingsStore()

  return (
    <SettingsPage
      title="Lyrics"
      description="Provider priority, Apple Music V2 syllable sync, typography and effects"
    >
      {/* Live Interactive Preview */}
      <div className="mb-8">
        <LyricsPreviewCard />
      </div>

      {/* Provider Selection */}
      <SettingsSection title="Providers">
        <SettingRow
          icon={<Music />}
          label="Preferred lyrics provider"
          description="Choose preferred source for synchronized lyrics and word timings (piMusic library suite)"
          stacked
          control={
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full" role="radiogroup" aria-label="Lyrics Provider">
              {[
                {
                  id: 'auto' as LyricsProvider,
                  name: 'Auto Priority',
                  desc: 'Prioritizes Apple Music TTML & Musixmatch RichSync with LRCLIB & KuGou fallback',
                  icon: Sparkles,
                  badge: 'Recommended',
                },
                {
                  id: 'betterlyrics' as LyricsProvider,
                  name: 'BetterLyrics',
                  desc: 'Official Apple Music syllable & word-timed TTML from piMusic',
                  icon: Music,
                  badge: 'Apple Music TTML',
                },
                {
                  id: 'musixmatch' as LyricsProvider,
                  name: 'Musixmatch',
                  desc: 'Word-level synchronized timings and rich subtitle lyrics catalog',
                  icon: Mic2,
                  badge: 'RichSync',
                },
                {
                  id: 'lrclib' as LyricsProvider,
                  name: 'LRCLIB',
                  desc: 'Fast, community-driven synced lyrics with millisecond precision',
                  icon: FileText,
                  badge: 'Community LRC',
                },
                {
                  id: 'kugou' as LyricsProvider,
                  name: 'KuGou',
                  desc: 'Extensive Asian, anime, and international synchronized LRC database',
                  icon: AudioLines,
                  badge: 'Global Catalog',
                },
              ].map((provider) => {
                const selected = s.lyricsProvider === provider.id
                const Icon = provider.icon
                return (
                  <div
                    key={provider.id}
                    role="radio"
                    tabIndex={0}
                    aria-checked={selected}
                    onClick={() => s.set('lyricsProvider', provider.id)}
                    onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && s.set('lyricsProvider', provider.id)}
                    className={cn(
                      'group relative p-3.5 rounded-2xl cursor-pointer select-none transition-[background-color,border-color,box-shadow] duration-200 ease-emphasized outline-none flex flex-col justify-between min-h-[96px]',
                      selected
                        ? 'bg-secondary-container/35 border-2 border-primary text-on-surface shadow-md3-1 ring-1 ring-primary/25'
                        : 'bg-surface-container-low border border-outline-variant/60 text-on-surface hover:bg-surface-container hover:border-outline'
                    )}
                  >
                    <div className="flex items-start justify-between gap-3 mb-1.5">
                      <div className="flex items-center gap-2.5">
                        <span
                          className={cn(
                            'size-8 rounded-xl flex items-center justify-center transition-colors shrink-0',
                            selected
                              ? 'bg-primary text-on-primary shadow-sm'
                              : 'bg-surface-container-high text-on-surface-variant group-hover:text-primary'
                          )}
                        >
                          <Icon className="size-4" />
                        </span>
                        <div>
                          <span className="type-title-sm font-semibold text-on-surface block leading-tight">{provider.name}</span>
                          {provider.badge && (
                            <span className="inline-block mt-0.5 px-1.5 py-0.5 rounded text-[10px] font-semibold bg-surface-container-highest text-primary border border-outline-variant/40">
                              {provider.badge}
                            </span>
                          )}
                        </div>
                      </div>
                      <span
                        className={cn(
                          'size-4 rounded-full border-2 transition-all mt-0.5 shrink-0 flex items-center justify-center',
                          selected ? 'border-primary bg-primary' : 'border-outline-variant group-hover:border-outline'
                        )}
                      >
                        {selected && <span className="size-1.5 rounded-full bg-on-primary" />}
                      </span>
                    </div>
                    <p className="type-body-sm text-on-surface-variant line-clamp-2 text-xs leading-relaxed">
                      {provider.desc}
                    </p>
                  </div>
                )
              })}
            </div>
          }
        />
      </SettingsSection>

      {/* Sync Animation Style - True MD3 Selectable Cards */}
      <SettingsSection title="Sync Animation">
        <SettingRow
          icon={<Wand2 />}
          label="Animation style"
          description="Choose how synced text highlights as vocals play"
          stacked
          control={
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full" role="radiogroup" aria-label="Sync Animation Style">
              {[
                {
                  id: 'apple_music_v2' as LyricsAnimationStyle,
                  name: 'Apple Music V2',
                  desc: 'Letter-by-letter fluid illumination with character glow (piTube Apple V2 engine)',
                  icon: AudioLines,
                  badge: 'Recommended',
                },
                {
                  id: 'lyrics_v2_fluid' as LyricsAnimationStyle,
                  name: 'Lyrics V2 Fluid',
                  desc: 'Liquid horizontal feathered gradient sweep across words with subtle float',
                  icon: Waves,
                  badge: 'Liquid Fill',
                },
                {
                  id: 'apple_music' as LyricsAnimationStyle,
                  name: 'Apple Music',
                  desc: 'Word-level smoothstep scale bump with warm glowing ambient shadow',
                  icon: Music,
                },
                {
                  id: 'glow' as LyricsAnimationStyle,
                  name: 'Ambient Glow',
                  desc: 'Soft radiant glow surrounding the active singing line',
                  icon: Flame,
                },
                {
                  id: 'fade' as LyricsAnimationStyle,
                  name: 'Gentle Fade',
                  desc: 'Subtle opacity crossfades between past and upcoming lines',
                  icon: Wind,
                },
                {
                  id: 'classic' as LyricsAnimationStyle,
                  name: 'Classic Line',
                  desc: 'Clean, instantaneous line highlight without letter interpolation',
                  icon: FileText,
                },
              ].map((style) => {
                const selected = s.lyricsAnimationStyle === style.id
                const Icon = style.icon
                return (
                  <div
                    key={style.id}
                    role="radio"
                    tabIndex={0}
                    aria-checked={selected}
                    onClick={() => s.set('lyricsAnimationStyle', style.id)}
                    onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && s.set('lyricsAnimationStyle', style.id)}
                    className={cn(
                      'group relative p-4 rounded-2xl cursor-pointer select-none transition-[background-color,border-color,box-shadow] duration-200 ease-emphasized outline-none flex flex-col justify-between min-h-[116px]',
                      selected
                        ? 'bg-secondary-container/35 border-2 border-primary text-on-surface shadow-md3-1 ring-1 ring-primary/25'
                        : 'bg-surface-container-low border border-outline-variant/60 text-on-surface hover:bg-surface-container hover:border-outline'
                    )}
                  >
                    <div className="flex items-start justify-between gap-3 mb-2">
                      <div className="flex items-center gap-2.5">
                        <span
                          className={cn(
                            'size-9 rounded-xl flex items-center justify-center transition-colors',
                            selected
                              ? 'bg-primary text-on-primary shadow-sm'
                              : 'bg-surface-container-high text-on-surface-variant group-hover:text-primary'
                          )}
                        >
                          <Icon className="size-4.5" />
                        </span>
                        <div>
                          <span className="type-title-sm font-semibold text-on-surface block leading-tight">{style.name}</span>
                          {style.badge && (
                            <span className="inline-block mt-0.5 px-1.5 py-0.5 rounded text-[10px] font-semibold bg-surface-container-highest text-primary border border-outline-variant/40">
                              {style.badge}
                            </span>
                          )}
                        </div>
                      </div>
                      <span
                        className={cn(
                          'size-5 rounded-full border-2 flex items-center justify-center transition-all shrink-0 mt-0.5',
                          selected ? 'border-primary bg-primary text-on-primary' : 'border-outline group-hover:border-on-surface'
                        )}
                      >
                        {selected && <span className="size-2 rounded-full bg-surface" />}
                      </span>
                    </div>
                    <p className="type-body-sm text-on-surface-variant/90 leading-snug">{style.desc}</p>
                  </div>
                )
              })}
            </div>
          }
        />
      </SettingsSection>

      {/* Visual Effects */}
      <SettingsSection title="Visual Effects">
        <SettingRow
          icon={<Flame />}
          label="Glowing lyrics"
          description="Highlight the active singing line with dynamic ambient glow"
          control={
            <Switch
              checked={s.lyricsGlow}
              onChange={(v) => s.set('lyricsGlow', v)}
              label="Glowing lyrics"
            />
          }
        />
        <SettingRow
          icon={<Eye />}
          label="Blur inactive lines"
          description="Apply subtle depth-of-field blur to inactive lines (Apple Music style)"
          control={
            <Switch
              checked={s.lyricsBlur}
              onChange={(v) => s.set('lyricsBlur', v)}
              label="Blur inactive lines"
            />
          }
        />
      </SettingsSection>

      {/* Typography & Layout */}
      <SettingsSection title="Typography & Layout">
        <SettingRow
          icon={<AlignLeft />}
          label="Text alignment"
          description="Position lyrics on the screen"
          stacked
          control={
            <SegmentedButton<LyricsTextPosition>
              value={s.lyricsTextPosition}
              onChange={(v) => s.set('lyricsTextPosition', v)}
              options={[
                { value: 'left', label: 'Left', icon: <AlignLeft className="size-4" /> },
                { value: 'center', label: 'Center', icon: <AlignCenter className="size-4" /> },
                { value: 'right', label: 'Right', icon: <AlignRight className="size-4" /> },
              ]}
              className="w-full sm:w-auto"
            />
          }
        />

        <SettingRow
          icon={<Type />}
          label="Text size"
          description="Font size of lyric lines"
          stacked
          control={
            <SegmentedButton<LyricsTextSize>
              value={s.lyricsTextSize}
              onChange={(v) => s.set('lyricsTextSize', v)}
              options={[
                { value: 'sm', label: 'Small' },
                { value: 'md', label: 'Medium' },
                { value: 'lg', label: 'Large' },
                { value: 'xl', label: 'Extra Large' },
              ]}
              className="w-full sm:w-auto"
            />
          }
        />

        <SettingRow
          icon={<Layers />}
          label="Line spacing"
          description={`${s.lyricsLineSpacing.toFixed(1)}× line height`}
          stacked
          control={
            <div className="flex items-center gap-4 w-full">
              <Slider
                value={s.lyricsLineSpacing}
                min={1.0}
                max={2.2}
                step={0.1}
                onChange={(v) => s.set('lyricsLineSpacing', Math.round(v * 10) / 10)}
                className="flex-1"
              />
              <button
                onClick={() => s.set('lyricsLineSpacing', 1.5)}
                className="type-label-md text-primary font-semibold hover:underline"
              >
                Default
              </button>
            </div>
          }
        />
      </SettingsSection>

      {/* Behavior & Timing */}
      <SettingsSection title="Behavior & Timing">
        <SettingRow
          label="Auto-scroll"
          description="Keep active line centered in view during playback"
          control={
            <Switch
              checked={s.lyricsAutoScroll}
              onChange={(v) => s.set('lyricsAutoScroll', v)}
              label="Auto-scroll"
            />
          }
        />

        <SettingRow
          label="Tap to seek"
          description="Click or tap any lyric line to jump playback to that timestamp"
          control={
            <Switch
              checked={s.lyricsSeekOnClick}
              onChange={(v) => s.set('lyricsSeekOnClick', v)}
              label="Tap to seek"
            />
          }
        />

        <SettingRow
          icon={<Clock />}
          label="Sync offset nudge"
          description={
            s.lyricsSyncOffsetMs === 0
              ? 'Synchronized with audio timestamp (0 ms)'
              : s.lyricsSyncOffsetMs > 0
                ? `Highlights +${s.lyricsSyncOffsetMs} ms earlier (anticipates vocal)`
                : `Highlights ${s.lyricsSyncOffsetMs} ms later (delays highlight)`
          }
          stacked
          control={
            <div className="flex items-center gap-4 w-full">
              <Slider
                value={s.lyricsSyncOffsetMs}
                min={-3000}
                max={3000}
                step={50}
                onChange={(v) => s.set('lyricsSyncOffsetMs', Math.round(v / 50) * 50)}
                className="flex-1"
              />
              <button
                onClick={() => s.set('lyricsSyncOffsetMs', 0)}
                title="Reset offset to 0ms"
                className="inline-flex items-center gap-1 type-label-md text-primary font-semibold hover:underline"
              >
                <RotateCcw className="size-3.5" />
                <span>Reset</span>
              </button>
            </div>
          }
        />
      </SettingsSection>
    </SettingsPage>
  )
}
