import React, { useMemo, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Users, Search } from 'lucide-react'
import { useArtists } from '@/hooks/useQueries'
import { useLibraryStore } from '@/stores/libraryStore'
import { MediaCard } from '@/components/common/MediaCard'
import { CardGridSkeleton } from '@/components/common/Skeleton'
import { ErrorState } from '@/components/common/ErrorState'
import { EmptyState } from '@/components/common/EmptyState'
import { PageContainer } from '@/components/common/PageContainer'
import { Chip } from '@/components/md3'
import { pluralize, formatCount } from '@/lib/format'

export const Route = createFileRoute('/explore/artists')({
  component: ExploreArtists,
})

function ExploreArtists() {
  const navigate = useNavigate()
  const { data = [], isLoading, isError, error, refetch } = useArtists()
  const followed = useLibraryStore((s) => s.favouriteArtistIds)
  const [q, setQ] = useState('')
  const [onlyFollowed, setOnlyFollowed] = useState(false)

  const artists = useMemo(() => {
    const lq = q.trim().toLowerCase()
    return data.filter((a) => (!lq || a.name.toLowerCase().includes(lq)) && (!onlyFollowed || followed.has(a.id))).sort((a, b) => a.name.localeCompare(b.name))
  }, [data, q, onlyFollowed, followed])

  return (
    <PageContainer className="space-y-5">
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="type-headline-lg text-on-surface">Artists</h1>
          <p className="type-body-md text-on-surface-variant mt-1">{isLoading ? 'Loading…' : pluralize(data.length, 'artist')}</p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 size-4 text-on-surface-variant" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter artists" className="w-full h-11 pl-10 pr-4 rounded-full bg-surface-high text-on-surface type-body-md outline-none focus:ring-2 ring-primary/60" />
        </div>
      </div>
      <div className="flex gap-2">
        <Chip variant="filter" label="All" selected={!onlyFollowed} onClick={() => setOnlyFollowed(false)} />
        <Chip variant="filter" label={`Following${followed.size ? ` · ${followed.size}` : ''}`} selected={onlyFollowed} onClick={() => setOnlyFollowed(true)} />
      </div>
      {isError ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : isLoading ? (
        <CardGridSkeleton count={12} circle />
      ) : artists.length === 0 ? (
        <EmptyState icon={<Users />} title={onlyFollowed ? 'You aren\u2019t following anyone yet' : q ? `No artists match “${q}”` : 'No artists yet'} description={onlyFollowed ? 'Open an artist and tap the heart to follow.' : undefined} />
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-x-4 gap-y-6">
          {artists.map((a) => (
            <MediaCard key={a.id} title={a.name} subtitle={a.monthly_listeners ? `${formatCount(a.monthly_listeners)} listeners` : 'Artist'} imageUrl={a.avatar_url} kind="artist" onClick={() => navigate({ to: '/artist/$artistId', params: { artistId: a.id } })} />
          ))}
        </div>
      )}
    </PageContainer>
  )
}
