import React, { useEffect, useRef, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Search as SearchIcon, X, Music2, Play, Clock } from 'lucide-react'
import { useDebounce } from '@/hooks/useDebounce'
import { useSearch, useTopics } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { TrackRow } from '@/components/common/TrackRow'
import { MediaCard } from '@/components/common/MediaCard'
import { TrackRowSkeleton } from '@/components/common/Skeleton'
import { ErrorState } from '@/components/common/ErrorState'
import { EmptyState } from '@/components/common/EmptyState'
import { SectionHeader } from '@/components/common/SectionHeader'
import { PageContainer } from '@/components/common/PageContainer'
import { Chip, Button } from '@/components/md3'
import { pushRecentSearch } from '@/components/overlays/CommandPalette'
import { cn } from '@/lib/cn'

type Filter = 'all' | 'tracks' | 'albums' | 'artists'

export const Route = createFileRoute('/search')({
  validateSearch: (s: Record<string, unknown>): { q?: string; f?: Filter } => {
    const out: { q?: string; f?: Filter } = {}
    if (typeof s.q === 'string' && s.q) out.q = s.q
    if ((['tracks', 'albums', 'artists'] as const).includes(s.f as never)) out.f = s.f as Filter
    return out
  },
  component: SearchPage,
})

const readRecent = (): string[] => {
  try {
    return JSON.parse(localStorage.getItem('webx.search.recent') || '[]')
  } catch {
    return []
  }
}

function SearchPage() {
  const { q = '', f } = Route.useSearch()
  const navigate = useNavigate()
  const [term, setTerm] = useState(q)
  const debounced = useDebounce(term, 220)
  const filter: Filter = f ?? 'all'
  const inputRef = useRef<HTMLInputElement>(null)
  const { data, isLoading, isFetching, isError, error, refetch } = useSearch(debounced)
  const topics = useTopics()
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const [recent, setRecent] = useState(readRecent)

  // keep URL in sync (shareable searches, back button restores)
  useEffect(() => {
    if (debounced !== q) navigate({ to: '/search', search: { q: debounced || undefined, f }, replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced])
  useEffect(() => {
    if (q !== term) setTerm(q)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])
  useEffect(() => {
    if (debounced.trim() && data && data.total > 0) {
      pushRecentSearch(debounced.trim())
      setRecent(readRecent())
    }
  }, [debounced, data])

  const setFilter = (nf: Filter) => navigate({ to: '/search', search: { q: q || undefined, f: nf === 'all' ? undefined : nf }, replace: true })

  const tracks = data?.tracks ?? []
  const albums = data?.albums ?? []
  const artists = data?.artists ?? []
  const nothing = data && tracks.length === 0 && albums.length === 0 && artists.length === 0

  return (
    <PageContainer className="space-y-6">
      {/* Search bar */}
      <div className="relative max-w-2xl">
        <SearchIcon className="absolute left-4 top-1/2 -translate-y-1/2 size-5 text-on-surface-variant pointer-events-none" />
        <input
          ref={inputRef}
          autoFocus
          type="search"
          enterKeyHint="search"
          placeholder="What do you want to hear?"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          className="w-full h-14 pl-12 pr-12 rounded-full bg-surface-high text-on-surface type-body-lg placeholder:text-on-surface-variant/70 outline-none focus:ring-2 ring-primary/60 transition-shadow [&::-webkit-search-cancel-button]:hidden"
        />
        {term && (
          <button onClick={() => { setTerm(''); inputRef.current?.focus() }} aria-label="Clear" className="absolute right-3 top-1/2 -translate-y-1/2 state-layer size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant">
            <X className="size-4" />
          </button>
        )}
        {isFetching && !isLoading && <span className="absolute right-12 top-1/2 -translate-y-1/2 size-4 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />}
      </div>

      {/* Filters */}
      {debounced && (
        <div className="flex flex-wrap items-center gap-2">
          {(['all', 'tracks', 'albums', 'artists'] as Filter[]).map((t) => (
            <Chip key={t} variant="filter" selected={filter === t} onClick={() => setFilter(t)} label={t === 'all' ? 'All' : t === 'tracks' ? `Tracks${data ? ` · ${data.total}` : ''}` : t === 'albums' ? `Albums${data ? ` · ${albums.length}` : ''}` : `Artists${data ? ` · ${artists.length}` : ''}`} />
          ))}
        </div>
      )}

      {/* Idle state: recent + topics */}
      {!debounced && (
        <div className="space-y-8">
          {recent.length > 0 && (
            <section>
              <SectionHeader title="Recent searches" action={<Button variant="text" size="sm" onClick={() => { localStorage.removeItem('webx.search.recent'); setRecent([]) }}>Clear</Button>} />
              <div className="flex flex-wrap gap-2">
                {recent.map((r) => <Chip key={r} label={r} icon={<Clock />} onClick={() => setTerm(r)} />)}
              </div>
            </section>
          )}
          {topics.data && topics.data.length > 0 && (
            <section>
              <SectionHeader title="Browse by topic" subtitle="Channels and topics your server organises tracks into" />
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                {topics.data.slice(0, 16).map((t, i) => (
                  <button
                    key={t.name}
                    onClick={() => navigate({ to: '/topic/$name', params: { name: t.name } })}
                    className={cn('state-layer h-24 rounded-lg p-4 text-left flex flex-col justify-between overflow-hidden relative', ['bg-primary-container text-on-primary-container', 'bg-secondary-container text-on-secondary-container', 'bg-tertiary-container text-on-tertiary-container', 'bg-surface-highest text-on-surface'][i % 4])}
                  >
                    <span className="type-title-md truncate">{t.name}</span>
                    {t.count != null && <span className="type-label-md opacity-80">{t.count} tracks</span>}
                    <Music2 className="absolute -right-3 -bottom-3 size-16 opacity-15 rotate-12" />
                  </button>
                ))}
              </div>
            </section>
          )}
          {recent.length === 0 && (!topics.data || topics.data.length === 0) && (
            <EmptyState icon={<SearchIcon />} title="Search your library" description="Find tracks, albums and artists. Results update as you type." />
          )}
        </div>
      )}

      {/* Results */}
      {debounced && isError && <ErrorState error={error} onRetry={() => refetch()} compact />}
      {debounced && isLoading && <TrackRowSkeleton count={8} />}
      {debounced && !isLoading && nothing && (
        <EmptyState icon={<SearchIcon />} title={`No results for “${debounced}”`} description="Check the spelling or try fewer words." compact />
      )}
      {debounced && data && !nothing && (
        <div className="space-y-10">
          {(filter === 'all' || filter === 'artists') && artists.length > 0 && (
            <section>
              <SectionHeader title="Artists" />
              <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-x-4 gap-y-6">
                {(filter === 'all' ? artists.slice(0, 8) : artists).map((a) => (
                  <MediaCard key={a.id} title={a.name} subtitle="Artist" imageUrl={a.avatar_url} kind="artist" onClick={() => navigate({ to: '/artist/$artistId', params: { artistId: a.id } })} />
                ))}
              </div>
            </section>
          )}
          {(filter === 'all' || filter === 'albums') && albums.length > 0 && (
            <section>
              <SectionHeader title="Albums" />
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-x-4 gap-y-6">
                {(filter === 'all' ? albums.slice(0, 6) : albums).map((a) => (
                  <MediaCard key={a.id} title={a.title} subtitle={a.artist} imageUrl={a.cover_url} onClick={() => navigate({ to: '/album/$albumId', params: { albumId: a.id } })} />
                ))}
              </div>
            </section>
          )}
          {(filter === 'all' || filter === 'tracks') && tracks.length > 0 && (
            <section>
              <SectionHeader
                title="Tracks"
                subtitle={`${data.total} match${data.total === 1 ? '' : 'es'}`}
                action={<Button variant="text" size="sm" icon={<Play className="fill-current" />} onClick={() => void playTrackWithQueue(tracks, 0, { type: 'search', title: `Search “${debounced}”` })}>Play all</Button>}
              />
              <div className="space-y-0.5">
                {tracks.map((t, i) => (
                  <TrackRow key={t.id} track={t} index={i} tracks={tracks} onPlay={() => void playTrackWithQueue(tracks, i, { type: 'search', title: `Search “${debounced}”` })} />
                ))}
              </div>
            </section>
          )}
        </div>
      )}
    </PageContainer>
  )
}
