import React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useAlbumDetail } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { CollectionHeader } from '@/components/common/CollectionHeader'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { Skeleton, TrackRowSkeleton } from '@/components/common/Skeleton'
import { pluralize, formatLongDuration } from '@/lib/format'

export const Route = createFileRoute('/album/$albumId')({
  component: AlbumPage,
})

function AlbumPage() {
  const { albumId } = Route.useParams()
  const navigate = useNavigate()
  const { data: album, isLoading, isError, error, refetch } = useAlbumDetail(albumId)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const contextId = useQueueStore((s) => s.context?.id)
  const isPlaying = usePlayerStore((s) => s.isPlaying) && contextId === albumId
  const togglePlay = usePlayerStore((s) => s.togglePlay)

  if (isError) return <PageContainer><ErrorState error={error} onRetry={() => refetch()} /></PageContainer>
  if (isLoading || !album) {
    return (
      <PageContainer>
        <div className="flex flex-col sm:flex-row sm:items-end gap-6 pb-6">
          <Skeleton className="size-44 sm:size-52 mx-auto sm:mx-0" />
          <div className="flex-1 space-y-3"><Skeleton variant="text" className="w-20" /><Skeleton className="h-10 w-2/3" /><Skeleton variant="text" className="w-1/3" /></div>
        </div>
        <TrackRowSkeleton />
      </PageContainer>
    )
  }

  const tracks = album.tracks
  const ctx = { type: 'album' as const, id: albumId, title: album.title, href: `/album/${albumId}` }
  const duration = tracks.reduce((a, t) => a + (t.duration_sec || 0), 0)

  return (
    <PageContainer>
      <CollectionHeader
        kind="album"
        eyebrow="Album"
        title={album.title}
        subtitle={
          album.artist_id ? (
            <button onClick={() => navigate({ to: '/artist/$artistId', params: { artistId: album.artist_id! } })} className="font-semibold hover:underline">{album.artist}</button>
          ) : (
            <span className="font-semibold">{album.artist}</span>
          )
        }
        meta={[album.year, pluralize(tracks.length, 'track'), formatLongDuration(duration)].filter(Boolean).join(' · ')}
        imageUrl={album.cover_url}
        isPlaying={isPlaying}
        onPlay={() => (isPlaying ? togglePlay() : tracks.length && void playTrackWithQueue(tracks, 0, ctx))}
        onShuffle={() => tracks.length && void playTrackWithQueue([...tracks].sort(() => Math.random() - 0.5), 0, ctx)}
      />
      <VirtualTrackList tracks={tracks} context={ctx} showAlbum={false} showCover={false} header={tracks.length ? <TrackListHeader showAlbum={false} showCover={false} /> : undefined} emptyTitle="No tracks in this album" emptyMessage="The server hasn't linked any tracks here yet." />
    </PageContainer>
  )
}
