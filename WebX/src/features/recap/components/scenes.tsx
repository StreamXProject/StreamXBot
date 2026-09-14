import React, { useEffect, useState } from 'react'
import { Artwork } from '@/components/common/Artwork'
import { cn } from '@/lib/cn'
import type { RecapScene, SceneAnimation } from '../types'

const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const fmtHour = (h: number) => `${h % 12 === 0 ? 12 : h % 12}${h < 12 ? ' AM' : ' PM'}`

const enterClass: Record<SceneAnimation, string> = {
  fade: 'recap-enter-fade',
  scaleFade: 'recap-enter-scale',
  slideUp: 'recap-enter-slideUp',
  zoom: 'recap-enter-zoom',
}

/** Count-up number driven by scene progress (no own timer). */
const CountUp: React.FC<{ value: number; progress: number; className?: string }> = ({ value, progress, className }) => {
  const eased = 1 - Math.pow(1 - Math.min(1, progress * 2.2), 3)
  return <span className={cn('tabular', className)}>{Math.round(value * eased).toLocaleString()}</span>
}

const Stagger: React.FC<{ i: number; children: React.ReactNode; className?: string }> = ({ i, children, className }) => (
  <div className={cn('recap-enter-slideUp', className)} style={{ animationDelay: `${180 + i * 110}ms` }}>
    {children}
  </div>
)

const Eyebrow: React.FC<{ children: React.ReactNode }> = ({ children }) => <p className="type-label-lg text-primary uppercase tracking-[0.18em]">{children}</p>

const Bars: React.FC<{ values: number[]; labels?: string[]; highlight?: number | null; progress: number }> = ({ values, labels, highlight, progress }) => {
  const max = Math.max(1, ...values)
  const reveal = Math.min(1, progress * 2.5)
  return (
    <div className="flex items-end gap-1 h-36 w-full">
      {values.map((v, i) => (
        <div key={i} className="flex-1 flex flex-col items-center gap-1 min-w-0 h-full justify-end">
          <div
            className={cn('w-full rounded-t-sm transition-[height] duration-500 ease-emphasized', highlight === i ? 'bg-primary' : 'bg-on-surface/30')}
            style={{ height: `${Math.max(2, (v / max) * 100 * reveal)}%` }}
          />
          {labels && <span className={cn('type-label-sm truncate', highlight === i ? 'text-primary' : 'text-on-surface-variant/70')}>{labels[i]}</span>}
        </div>
      ))}
    </div>
  )
}

export const Scene: React.FC<{ scene: RecapScene; progress: number }> = ({ scene, progress }) => {
  const [mounted, setMounted] = useState(false)
  useEffect(() => {
    const t = requestAnimationFrame(() => setMounted(true))
    return () => cancelAnimationFrame(t)
  }, [])
  const wrap = cn('flex-1 min-h-0 flex flex-col justify-center gap-5', mounted ? enterClass[scene.enter] : 'opacity-0')

  switch (scene.type) {
    case 'intro':
      return (
        <div className={cn(wrap, 'items-center text-center')}>
          <Eyebrow>{scene.eyebrow}</Eyebrow>
          <h1 className="type-display-md sm:type-display-lg text-on-surface">{scene.label}</h1>
          <p className="type-title-md text-on-surface-variant recap-enter-fade" style={{ animationDelay: '900ms' }}>in music</p>
        </div>
      )

    case 'minutes':
      return (
        <div className={wrap}>
          <Eyebrow>You listened for</Eyebrow>
          <p className="type-display-lg text-on-surface leading-none">
            <CountUp value={scene.minutes} progress={progress} />
          </p>
          <p className="type-headline-sm text-on-surface-variant">minutes</p>
          <Stagger i={2} className="space-y-1">
            <p className="type-body-lg text-on-surface">{scene.plays.toLocaleString()} plays</p>
            {scene.deltaPct !== null && scene.prevLabel && (
              <p className={cn('type-body-lg', scene.deltaPct >= 0 ? 'text-tertiary' : 'text-on-surface-variant')}>
                {scene.deltaPct >= 0 ? '↑' : '↓'} {Math.abs(scene.deltaPct)}% vs {scene.prevLabel}
              </p>
            )}
          </Stagger>
        </div>
      )

    case 'top_artist':
      return (
        <div className={wrap}>
          <Eyebrow>Your #1 artist</Eyebrow>
          <Artwork src={scene.artist.cover_url} alt={scene.artist.name} kind="artist" priority className="size-44 sm:size-56 shadow-md3-3 recap-enter-zoom" />
          <div>
            <h2 className="type-display-sm text-on-surface break-words">{scene.artist.name}</h2>
            <p className="type-title-md text-on-surface-variant mt-1">{scene.artist.plays} plays · {Math.round(scene.artist.minutes)} min</p>
          </div>
          {scene.runnersUp.length > 0 && (
            <ol className="space-y-1.5">
              {scene.runnersUp.map((a, i) => (
                <Stagger key={a.name} i={i}>
                  <li className="flex items-center gap-3 type-body-lg text-on-surface-variant">
                    <span className="tabular w-6 text-on-surface-variant/60">{i + 2}</span>
                    <span className="truncate text-on-surface">{a.name}</span>
                    <span className="ml-auto tabular type-body-sm">{a.plays}</span>
                  </li>
                </Stagger>
              ))}
            </ol>
          )}
        </div>
      )

    case 'top_track':
      return (
        <div className={wrap}>
          <Eyebrow>Your #1 track</Eyebrow>
          <Artwork src={scene.stat.track.cover_url} alt={scene.stat.track.title} priority className="w-full max-w-[320px] aspect-square rounded-xl shadow-md3-3 recap-enter-scale" />
          <div>
            <h2 className="type-headline-lg text-on-surface break-words">{scene.stat.track.title}</h2>
            <p className="type-title-md text-on-surface-variant mt-1 truncate">{scene.stat.track.artist}</p>
            <p className="type-body-lg text-primary mt-2">Played {scene.stat.plays}× · {Math.round(scene.stat.minutes)} min</p>
          </div>
          {scene.runnersUp.length > 0 && (
            <ol className="space-y-1">
              {scene.runnersUp.slice(0, 3).map((t, i) => (
                <Stagger key={t.track.id} i={i}>
                  <li className="flex items-center gap-3 type-body-md text-on-surface-variant">
                    <span className="tabular w-5 text-on-surface-variant/60">{i + 2}</span>
                    <span className="truncate text-on-surface">{t.track.title}</span>
                    <span className="ml-auto tabular type-body-sm">{t.plays}×</span>
                  </li>
                </Stagger>
              ))}
            </ol>
          )}
        </div>
      )

    case 'top_album':
      return (
        <div className={wrap}>
          <Eyebrow>Album on repeat</Eyebrow>
          <Artwork src={scene.album.cover_url} alt={scene.album.album} kind="album" priority className="w-full max-w-[300px] aspect-square rounded-xl shadow-md3-3" />
          <div>
            <h2 className="type-headline-lg text-on-surface break-words">{scene.album.album}</h2>
            <p className="type-title-md text-on-surface-variant mt-1">{scene.album.artist}</p>
            <p className="type-body-lg text-primary mt-2">{scene.album.plays} plays · {Math.round(scene.album.minutes)} min</p>
          </div>
        </div>
      )

    case 'discovery':
      return (
        <div className={wrap}>
          <Eyebrow>Explorer mode</Eyebrow>
          <p className="type-display-md text-on-surface leading-none">
            <CountUp value={scene.count} progress={progress} />
          </p>
          <p className="type-headline-sm text-on-surface-variant">tracks you heard for the first time</p>
          <p className="type-body-lg text-on-surface">{Math.round(scene.share * 100)}% of everything you played was new</p>
          {scene.tracks.length > 0 && (
            <div className="flex -space-x-3">
              {scene.tracks.slice(0, 6).map((t, i) => (
                <Stagger key={t.id} i={i}>
                  <Artwork src={t.cover_url} alt={t.title} className="size-14 rounded-md ring-2 ring-surface" />
                </Stagger>
              ))}
            </div>
          )}
        </div>
      )

    case 'replayer':
      return (
        <div className={wrap}>
          <Eyebrow>On repeat</Eyebrow>
          <p className="type-display-md text-on-surface leading-none">
            <CountUp value={scene.repeats} progress={progress} />
          </p>
          <p className="type-headline-sm text-on-surface-variant">replays</p>
          <div className="flex items-center gap-4 mt-2">
            <Artwork src={scene.stat.track.cover_url} alt={scene.stat.track.title} className="size-20 rounded-lg shadow-md3-2 recap-enter-zoom" />
            <div className="min-w-0">
              <p className="type-title-lg text-on-surface truncate">{scene.stat.track.title}</p>
              <p className="type-body-md text-on-surface-variant truncate">{scene.stat.track.artist}</p>
              <p className="type-body-md text-primary">{scene.stat.plays}× alone</p>
            </div>
          </div>
        </div>
      )

    case 'habit':
      return (
        <div className={wrap}>
          <Eyebrow>When you listen</Eyebrow>
          <h2 className="type-headline-md text-on-surface">
            {scene.nightShare >= 0.45 ? 'Night owl hours.' : scene.peakHour !== null && scene.peakHour < 10 ? 'Early starts.' : 'Peak time:'}{' '}
            {scene.peakHour !== null && <span className="text-primary">{fmtHour(scene.peakHour)}</span>}
          </h2>
          <Bars values={scene.byHour} labels={scene.byHour.map((_, h) => (h % 6 === 0 ? fmtHour(h).replace(' ', '') : ''))} highlight={scene.peakHour} progress={progress} />
          <div>
            <p className="type-label-lg text-on-surface-variant mb-2">By day</p>
            <Bars values={scene.byWeekday} labels={WEEKDAYS} highlight={scene.peakWeekday} progress={progress} />
          </div>
          {scene.peakWeekday !== null && <p className="type-body-lg text-on-surface">{WEEKDAYS[scene.peakWeekday]}s are your biggest music days</p>}
        </div>
      )

    case 'trend':
      return (
        <div className={wrap}>
          <Eyebrow>Through the year</Eyebrow>
          <h2 className="type-headline-md text-on-surface">
            Your biggest month was <span className="text-primary">{scene.peakMonth}</span>
          </h2>
          <p className="type-body-lg text-on-surface-variant">{scene.peakMinutes.toLocaleString()} minutes</p>
          <Bars
            values={scene.byMonth.map((m) => m.minutes)}
            labels={scene.byMonth.map((m) => MONTHS[Number(m.month.split('-')[1]) - 1] ?? m.month)}
            highlight={scene.byMonth.findIndex((m) => m.minutes === scene.peakMinutes)}
            progress={progress}
          />
        </div>
      )

    case 'personality':
      return (
        <div className={wrap}>
          <Eyebrow>Your listening personality</Eyebrow>
          <h2 className="type-display-sm text-on-surface">{scene.traits[0]?.name}</h2>
          <p className="type-title-md text-on-surface-variant">{scene.traits[0]?.detail}</p>
          {scene.traits.length > 1 && (
            <div className="flex flex-wrap gap-2 mt-2">
              {scene.traits.slice(1).map((t, i) => (
                <Stagger key={t.id} i={i}>
                  <span className="inline-flex flex-col h-auto py-2 px-4 rounded-2xl bg-secondary-container text-on-secondary-container">
                    <span className="type-label-lg">{t.name}</span>
                    <span className="type-body-sm opacity-80">{t.detail}</span>
                  </span>
                </Stagger>
              ))}
            </div>
          )}
        </div>
      )

    case 'comparison': {
      const c = scene.comparison
      const rows: { label: string; value: string; up: boolean | null }[] = [
        { label: 'Minutes', value: c.minutesDeltaPct === null ? `${scene.minutes.toLocaleString()}` : `${c.minutesDeltaPct >= 0 ? '↑' : '↓'} ${Math.abs(c.minutesDeltaPct)}%`, up: c.minutesDeltaPct === null ? null : c.minutesDeltaPct >= 0 },
        { label: 'Artists', value: `${c.artistsDelta >= 0 ? '+' : ''}${c.artistsDelta}`, up: c.artistsDelta >= 0 },
        { label: 'Night listening', value: `${c.nightShareDelta >= 0 ? '+' : ''}${c.nightShareDelta} pts`, up: c.nightShareDelta >= 0 },
      ]
      return (
        <div className={wrap}>
          <Eyebrow>Compared to before</Eyebrow>
          <h2 className="type-headline-md text-on-surface">vs {c.period}</h2>
          <div className="space-y-3">
            {rows.map((r, i) => (
              <Stagger key={r.label} i={i}>
                <div className="flex items-center justify-between h-14 px-4 rounded-2xl glass border border-outline-variant/40">
                  <span className="type-body-lg text-on-surface">{r.label}</span>
                  <span className={cn('type-title-lg tabular', r.up === null ? 'text-on-surface' : r.up ? 'text-tertiary' : 'text-on-surface-variant')}>{r.value}</span>
                </div>
              </Stagger>
            ))}
          </div>
          {c.topArtistPrev && <p className="type-body-md text-on-surface-variant">Previous #1 artist: {c.topArtistPrev}</p>}
        </div>
      )
    }

    case 'outro':
      return (
        <div className={cn(wrap, 'gap-4')}>
          <Eyebrow>{scene.label}</Eyebrow>
          {scene.plays === 0 ? (
            <>
              <h2 className="type-headline-md text-on-surface">Nothing played yet</h2>
              <p className="type-body-lg text-on-surface-variant">Play some music and come back — this recap fills in as you listen.</p>
            </>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3">
                {[
                  [scene.minutes, 'minutes'],
                  [scene.plays, 'plays'],
                  [scene.artists, 'artists'],
                  [scene.tracks, 'tracks'],
                ].map(([n, l], i) => (
                  <Stagger key={l} i={i}>
                    <div className="rounded-2xl glass border border-outline-variant/40 p-4">
                      <p className="type-headline-md text-on-surface tabular">{Number(n).toLocaleString()}</p>
                      <p className="type-label-lg text-on-surface-variant">{l}</p>
                    </div>
                  </Stagger>
                ))}
              </div>
              {scene.topArtist && (
                <Stagger i={4}>
                  <div className="flex items-center gap-3">
                    <Artwork src={scene.topArtist.cover_url} alt={scene.topArtist.name} kind="artist" className="size-12" />
                    <div className="min-w-0">
                      <p className="type-label-md text-primary">#1 ARTIST</p>
                      <p className="type-title-md text-on-surface truncate">{scene.topArtist.name}</p>
                    </div>
                  </div>
                </Stagger>
              )}
              {scene.topTrack && (
                <Stagger i={5}>
                  <div className="flex items-center gap-3">
                    <Artwork src={scene.topTrack.track.cover_url} alt={scene.topTrack.track.title} className="size-12 rounded-md" />
                    <div className="min-w-0">
                      <p className="type-label-md text-primary">#1 TRACK</p>
                      <p className="type-title-md text-on-surface truncate">{scene.topTrack.track.title}</p>
                    </div>
                  </div>
                </Stagger>
              )}
              {scene.traits.length > 0 && (
                <Stagger i={6}>
                  <p className="type-body-md text-on-surface-variant">{scene.traits.map((t) => t.name).join(' · ')}</p>
                </Stagger>
              )}
            </>
          )}
        </div>
      )
  }
}
