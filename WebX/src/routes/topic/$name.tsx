import React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTopicTracks } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { CollectionHeader } from '@/components/common/CollectionHeader'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { TrackRowSkeleton } from '@/components/common/Skeleton'
import { pluralize } from '@/lib/format'

export const Route = createFileRoute('/topic/$name')({
  component: TopicPage,
})

function TopicPage() {
  const { name } = Route.useParams()
  const { data: tracks = [], isLoading, isError, error, refetch } = useTopicTracks(name)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const ctx = { type: 'custom' as const, id: `topic:${name}`, title: name }
  const covers = tracks.map((t) => t.cover_url).filter((c): c is string => Boolean(c)).slice(0, 4)
  if (isError) return <PageContainer><ErrorState error={error} onRetry={() => refetch()} /></PageContainer>
  return (
    <PageContainer>
      <CollectionHeader kind="playlist" eyebrow="Topic" title={name} meta={isLoading ? 'Loading…' : pluralize(tracks.length, 'track')} imageUrl={covers[0]} collage={covers} onPlay={() => tracks.length && void playTrackWithQueue(tracks, 0, ctx)} onShuffle={() => tracks.length && void playTrackWithQueue([...tracks].sort(() => Math.random() - 0.5), 0, ctx)} />
      {isLoading ? <TrackRowSkeleton /> : <VirtualTrackList tracks={tracks} context={ctx} header={tracks.length ? <TrackListHeader /> : undefined} />}
    </PageContainer>
  )
}
