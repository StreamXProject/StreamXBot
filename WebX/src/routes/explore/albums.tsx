import React, { useMemo, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Disc3, Search } from 'lucide-react'
import { useAlbums } from '@/hooks/useQueries'
import { MediaCard } from '@/components/common/MediaCard'
import { CardGridSkeleton } from '@/components/common/Skeleton'
import { ErrorState } from '@/components/common/ErrorState'
import { EmptyState } from '@/components/common/EmptyState'
import { PageContainer } from '@/components/common/PageContainer'
import { Chip } from '@/components/md3'
import { pluralize } from '@/lib/format'

export const Route = createFileRoute('/explore/albums')({
  component: ExploreAlbums,
})

type Sort = 'recent' | 'title' | 'artist' | 'year'

function ExploreAlbums() {
  const navigate = useNavigate()
  const { data = [], isLoading, isError, error, refetch } = useAlbums()
  const [q, setQ] = useState('')
  const [sort, setSort] = useState<Sort>('recent')

  const albums = useMemo(() => {
    const lq = q.trim().toLowerCase()
    const list = lq ? data.filter((a) => a.title.toLowerCase().includes(lq) || a.artist.toLowerCase().includes(lq)) : [...data]
    if (sort === 'title') list.sort((a, b) => a.title.localeCompare(b.title))
    if (sort === 'artist') list.sort((a, b) => a.artist.localeCompare(b.artist) || a.title.localeCompare(b.title))
    if (sort === 'year') list.sort((a, b) => (b.year ?? 0) - (a.year ?? 0))
    return list
  }, [data, q, sort])

  return (
    <PageContainer className="space-y-5">
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="type-headline-lg text-on-surface">Albums</h1>
          <p className="type-body-md text-on-surface-variant mt-1">{isLoading ? 'Loading…' : pluralize(data.length, 'album')}</p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 size-4 text-on-surface-variant" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter albums" className="w-full h-11 pl-10 pr-4 rounded-full bg-surface-high text-on-surface type-body-md outline-none focus:ring-2 ring-primary/60" />
        </div>
      </div>
      <div className="flex gap-2 flex-wrap">
        {([['recent', 'Recently added'], ['title', 'Title'], ['artist', 'Artist'], ['year', 'Year']] as Array<[Sort, string]>).map(([v, l]) => (
          <Chip key={v} variant="filter" label={l} selected={sort === v} onClick={() => setSort(v)} />
        ))}
      </div>
      {isError ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : isLoading ? (
        <CardGridSkeleton count={12} />
      ) : albums.length === 0 ? (
        <EmptyState icon={<Disc3 />} title={q ? `No albums match “${q}”` : 'No albums yet'} description={q ? 'Try a different filter.' : 'Albums are grouped from track metadata on the server.'} />
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-x-4 gap-y-6">
          {albums.map((a) => (
            <MediaCard key={a.id} title={a.title} subtitle={[a.artist, a.year].filter(Boolean).join(' · ')} imageUrl={a.cover_url} onClick={() => navigate({ to: '/album/$albumId', params: { albumId: a.id } })} />
          ))}
        </div>
      )}
    </PageContainer>
  )
}
