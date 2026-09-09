import React from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Music2, Play } from 'lucide-react'
import { useSharedPlaylist } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { useAuthStore } from '@/stores/authStore'
import { Artwork } from '@/components/common/Artwork'
import { Button } from '@/components/md3'
import { ErrorState } from '@/components/common/ErrorState'
import { CircularProgress } from '@/components/md3'
import { formatDuration, pluralize } from '@/lib/format'

/** Public share page (no auth required to view) */
export const Route = createFileRoute('/share/playlist/$playlistId')({
  component: SharedPlaylistPage,
})

function SharedPlaylistPage() {
  const { playlistId } = Route.useParams()
  const { data, isLoading, isError, error } = useSharedPlaylist(playlistId)
  const token = useAuthStore((s) => s.token)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)

  if (isLoading) return <CircularProgress size={40} className="text-primary" />
  if (isError || !data) return <ErrorState error={error} />

  return (
    <div className="w-full max-w-2xl page-enter">
      <div className="flex items-center gap-3 mb-8">
        <span className="size-10 rounded-md bg-primary text-on-primary flex items-center justify-center"><Music2 className="size-5" strokeWidth={2.5} /></span>
        <span className="type-title-lg text-on-surface">WebX</span>
      </div>
      <div className="flex gap-5 items-end">
        <Artwork src={data.cover_url} alt={data.name} kind="playlist" className="size-40 rounded-lg shadow-md3-3" />
        <div className="min-w-0">
          <p className="type-label-lg text-primary">Shared playlist</p>
          <h1 className="type-headline-md text-on-surface truncate">{data.name}</h1>
          <p className="type-body-md text-on-surface-variant mt-1">{pluralize(data.tracks.length, 'track')}</p>
          <div className="mt-4 flex gap-2">
            {token ? (
              <Button icon={<Play className="fill-current" />} onClick={() => void playTrackWithQueue(data.tracks, 0, { type: 'playlist', id: playlistId, title: data.name })}>Play</Button>
            ) : (
              <Link to="/login" search={{ redirect: `/share/playlist/${playlistId}` }}><Button>Sign in to play</Button></Link>
            )}
          </div>
        </div>
      </div>
      <ol className="mt-8 divide-y divide-outline-variant/60 rounded-lg bg-surface-low overflow-hidden">
        {data.tracks.map((t, i) => (
          <li key={t.id} className="flex items-center gap-3 h-14 px-4">
            <span className="w-6 text-right tabular type-body-sm text-on-surface-variant">{i + 1}</span>
            <Artwork src={t.cover_url} alt="" className="size-9 rounded-xs" />
            <span className="min-w-0 flex-1">
              <span className="block type-body-md text-on-surface truncate">{t.title}</span>
              <span className="block type-body-sm text-on-surface-variant truncate">{t.artist}</span>
            </span>
            <span className="tabular type-body-sm text-on-surface-variant">{formatDuration(t.duration_sec)}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}
