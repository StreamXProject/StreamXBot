import React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMixDetail } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { CollectionHeader } from '@/components/common/CollectionHeader'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { TrackRowSkeleton } from '@/components/common/Skeleton'
import { Button } from '@/components/md3'
import { RefreshCw } from 'lucide-react'
import { pluralize, formatLongDuration } from '@/lib/format'

export const Route = createFileRoute('/mix/$mixId')({
  validateSearch: (s: Record<string, unknown>): { endpoint?: string; title?: string } => {
    const out: { endpoint?: string; title?: string } = {}
    if (typeof s.endpoint === 'string') out.endpoint = s.endpoint
    if (typeof s.title === 'string') out.title = s.title
    return out
  },
  component: MixPage,
})

const TITLES: Record<string, [string, string]> = {
  random: ['Daily Mix', 'A fresh rotation from across your library'],
  rediscover: ['Rediscover', 'Tracks you loved and haven\u2019t played in a while'],
  rising: ['Rising', 'Recently added and picking up plays'],
  surprise: ['Surprise Me', 'Genres you don\u2019t usually reach for'],
  top_played: ['Most played', 'Your all-time favourites'],
}

function MixPage() {
  const { mixId } = Route.useParams()
  const { endpoint, title } = Route.useSearch()
  const key = endpoint ?? mixId
  const { data: tracks = [], isLoading, isError, error, refetch, isRefetching } = useMixDetail(key)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const contextId = useQueueStore((s) => s.context?.id)
  const isPlaying = usePlayerStore((s) => s.isPlaying) && contextId === key
  const togglePlay = usePlayerStore((s) => s.togglePlay)

  const [name, desc] = TITLES[mixId.replace(/^daily:/, '')] ?? [title ?? `${mixId.charAt(0).toUpperCase()}${mixId.slice(1)}`, 'Curated for you']
  const ctx = { type: 'mix' as const, id: key, title: title ?? name }
  const covers = tracks.map((t) => t.cover_url).filter((c): c is string => Boolean(c)).slice(0, 4)
  const duration = tracks.reduce((a, t) => a + (t.duration_sec || 0), 0)

  if (isError) return <PageContainer><ErrorState error={error} onRetry={() => refetch()} /></PageContainer>

  return (
    <PageContainer>
      <CollectionHeader
        kind="mix"
        eyebrow="Mix"
        title={title ?? name}
        subtitle={desc}
        meta={isLoading ? 'Loading…' : `${pluralize(tracks.length, 'track')} · ${formatLongDuration(duration)}`}
        imageUrl={covers[0]}
        collage={covers}
        isPlaying={isPlaying}
        onPlay={() => (isPlaying ? togglePlay() : tracks.length && void playTrackWithQueue(tracks, 0, ctx))}
        onShuffle={() => tracks.length && void playTrackWithQueue([...tracks].sort(() => Math.random() - 0.5), 0, ctx)}
      >
        <Button variant="text" size="lg" icon={<RefreshCw className={isRefetching ? 'animate-spin' : ''} />} onClick={() => refetch()}>Refresh</Button>
      </CollectionHeader>
      {isLoading ? <TrackRowSkeleton /> : <VirtualTrackList tracks={tracks} context={ctx} header={tracks.length ? <TrackListHeader /> : undefined} emptyTitle="This mix is empty" emptyMessage="Mixes rebuild daily — check back later." />}
    </PageContainer>
  )
}
