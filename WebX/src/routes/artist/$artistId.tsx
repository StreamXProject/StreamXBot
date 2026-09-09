import React, { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useArtistDetail } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { CollectionHeader } from '@/components/common/CollectionHeader'
import { TrackRow } from '@/components/common/TrackRow'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { MediaCard } from '@/components/common/MediaCard'
import { SectionHeader } from '@/components/common/SectionHeader'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { Skeleton, TrackRowSkeleton } from '@/components/common/Skeleton'
import { Button } from '@/components/md3'
import { formatCount, pluralize } from '@/lib/format'

export const Route = createFileRoute('/artist/$artistId')({
  component: ArtistPage,
})

function ArtistPage() {
  const { artistId } = Route.useParams()
  const navigate = useNavigate()
  const { data: artist, isLoading, isError, error, refetch } = useArtistDetail(artistId)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const contextId = useQueueStore((s) => s.context?.id)
  const isPlaying = usePlayerStore((s) => s.isPlaying) && contextId === artistId
  const togglePlay = usePlayerStore((s) => s.togglePlay)
  const followed = useLibraryStore((s) => s.favouriteArtistIds.has(artistId))
  const toggleFollow = useLibraryStore((s) => s.toggleFollowArtist)
  const [showAll, setShowAll] = useState(false)

  if (isError) return <PageContainer><ErrorState error={error} onRetry={() => refetch()} /></PageContainer>
  if (isLoading || !artist) {
    return (
      <PageContainer>
        <div className="flex flex-col sm:flex-row sm:items-end gap-6 pb-6">
          <Skeleton variant="circle" className="size-44 sm:size-52 mx-auto sm:mx-0" />
          <div className="flex-1 space-y-3"><Skeleton variant="text" className="w-20" /><Skeleton className="h-10 w-1/2" /><Skeleton variant="text" className="w-1/3" /></div>
        </div>
        <TrackRowSkeleton />
      </PageContainer>
    )
  }

  const all = artist.all_tracks
  const top = artist.top_tracks
  const ctx = { type: 'artist' as const, id: artistId, title: artist.name }

  return (
    <PageContainer className="space-y-10">
      <CollectionHeader
        kind="artist"
        eyebrow="Artist"
        title={artist.name}
        meta={[artist.monthly_listeners ? `${formatCount(artist.monthly_listeners)} listeners` : null, pluralize(all.length, 'track'), artist.albums.length ? pluralize(artist.albums.length, 'release') : null].filter(Boolean).join(' · ')}
        imageUrl={artist.avatar_url}
        isPlaying={isPlaying}
        onPlay={() => (isPlaying ? togglePlay() : all.length && void playTrackWithQueue(all, 0, ctx))}
        onShuffle={() => all.length && void playTrackWithQueue([...all].sort(() => Math.random() - 0.5), 0, ctx)}
        liked={followed}
        onLike={() => void toggleFollow(artistId)}
      />

      {artist.bio && <p className="type-body-lg text-on-surface-variant max-w-prose -mt-4">{artist.bio}</p>}

      {top.length > 0 && (
        <section>
          <SectionHeader title="Popular" />
          <div className="space-y-0.5">
            {(showAll ? top : top.slice(0, 5)).map((t, i) => (
              <TrackRow key={t.id} track={t} index={i} tracks={top} showAlbum onPlay={() => void playTrackWithQueue(top, i, ctx)} />
            ))}
          </div>
          {top.length > 5 && <Button variant="text" size="sm" className="mt-2" onClick={() => setShowAll((s) => !s)}>{showAll ? 'Show less' : 'Show more'}</Button>}
        </section>
      )}

      {artist.albums.length > 0 && (
        <section>
          <SectionHeader title="Releases" />
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-x-4 gap-y-6">
            {artist.albums.map((a) => (
              <MediaCard key={a.id} title={a.title} subtitle={[a.year, a.track_count ? pluralize(a.track_count, 'track') : null].filter(Boolean).join(' · ')} imageUrl={a.cover_url} onClick={() => navigate({ to: '/album/$albumId', params: { albumId: a.id } })} />
            ))}
          </div>
        </section>
      )}

      {all.length > 0 && (
        <section>
          <SectionHeader title="All tracks" subtitle={pluralize(all.length, 'track')} />
          <VirtualTrackList tracks={all} context={ctx} header={<TrackListHeader />} />
        </section>
      )}
    </PageContainer>
  )
}
