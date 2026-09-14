import React from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Music2, Sparkles } from 'lucide-react'
import { fetchPublicRecap } from '@/features/recap/api'
import { Artwork } from '@/components/common/Artwork'
import { ErrorState } from '@/components/common/ErrorState'
import { AmbientBackdrop } from '@/components/player/AmbientBackdrop'

export const Route = createFileRoute('/recap/share/$token')({
  component: SharedRecapPage,
})

/** Public summary — only deliberately shared, non-identifying numbers. */
function SharedRecapPage() {
  const { token } = Route.useParams()
  const q = useQuery({ queryKey: ['recaps', 'share', token], queryFn: ({ signal }) => fetchPublicRecap(token, signal), retry: false })

  return (
    <div className="relative min-h-full w-full flex items-center justify-center p-6 overflow-hidden">
      <AmbientBackdrop src={q.data?.topTrack?.cover_url ?? null} enabled />
      <div className="relative z-10 w-full max-w-md rounded-3xl glass border border-outline-variant/40 p-6 sm:p-8 space-y-6">
        {q.isLoading ? (
          <p className="type-body-lg text-on-surface-variant text-center">Loading recap…</p>
        ) : q.isError || !q.data ? (
          <ErrorState error={q.error ?? new Error('This recap link is no longer available')} />
        ) : (
          <>
            <div className="flex items-center gap-2 text-primary type-label-lg uppercase tracking-[0.18em]"><Sparkles className="size-4" /> {q.data.label}</div>
            <div className="flex items-end gap-4">
              <Artwork src={q.data.topTrack?.cover_url} alt="" priority className="size-28 rounded-xl shadow-md3-3" />
              <div className="min-w-0">
                <p className="type-display-sm text-on-surface tabular leading-none">{q.data.totalMinutes.toLocaleString()}</p>
                <p className="type-title-md text-on-surface-variant">minutes of music</p>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-3">
              {([[q.data.totalPlays, 'plays'], [q.data.uniqueArtists, 'artists'], [q.data.uniqueTracks, 'tracks']] as [number, string][]).map(([n, l]) => (
                <div key={l} className="rounded-2xl bg-surface-low/70 p-3">
                  <p className="type-headline-sm text-on-surface tabular">{n.toLocaleString()}</p>
                  <p className="type-label-md text-on-surface-variant">{l}</p>
                </div>
              ))}
            </div>
            {q.data.topArtist && (
              <div>
                <p className="type-label-md text-primary">#1 ARTIST</p>
                <p className="type-title-lg text-on-surface">{q.data.topArtist.name}</p>
              </div>
            )}
            {q.data.topTrack && (
              <div>
                <p className="type-label-md text-primary">#1 TRACK</p>
                <p className="type-title-lg text-on-surface">{q.data.topTrack.title}</p>
                <p className="type-body-md text-on-surface-variant">{q.data.topTrack.artist}</p>
              </div>
            )}
            {q.data.personality.length > 0 && <p className="type-body-md text-on-surface-variant">{q.data.personality.map((p) => p.name).join(' · ')}</p>}
            <Link to="/" className="inline-flex items-center gap-2 type-label-lg text-primary hover:underline"><Music2 className="size-4" /> Made with WebX</Link>
          </>
        )}
      </div>
    </div>
  )
}
