import React, { useMemo, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Play, Shuffle, Clock, Sparkles, Flame } from 'lucide-react'
import { useBrowseTracks, useFeaturedMixes, useAlbums, useArtists, useHistory } from '@/hooks/useQueries'
import { useQueueStore } from '@/stores/queueStore'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { fetchMixTracks, fetchShuffle, type FeaturedMix } from '@/api/browse'
import { TrackRow } from '@/components/common/TrackRow'
import { MediaCard } from '@/components/common/MediaCard'
import { Shelf } from '@/components/common/Shelf'
import { SectionHeader } from '@/components/common/SectionHeader'
import { Skeleton, TrackRowSkeleton, CardGridSkeleton } from '@/components/common/Skeleton'
import { ErrorState } from '@/components/common/ErrorState'
import { PageContainer } from '@/components/common/PageContainer'
import { Artwork } from '@/components/common/Artwork'
import { Button } from '@/components/md3'
import { toast } from '@/stores/uiStore'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/')({
  component: HomePage,
})

function greeting() {
  const h = new Date().getHours()
  if (h < 5) return 'Up late'
  if (h < 12) return 'Good morning'
  if (h < 18) return 'Good afternoon'
  return 'Good evening'
}

const MixCard: React.FC<{ mix: FeaturedMix; onPlay: (m: FeaturedMix) => Promise<void> }> = ({ mix, onPlay }) => {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const slug = mix.id.replace(/^daily:/, '')
  return (
    <MediaCard
      title={mix.title}
      subtitle={mix.subtitle}
      imageUrl={mix.cover_url}
      collage={mix.thumbnails}
      kind="mix"
      loading={loading}
      onClick={() => navigate({ to: '/mix/$mixId', params: { mixId: slug }, search: { endpoint: mix.endpoint, title: mix.title } })}
      onPlay={async () => {
        setLoading(true)
        try {
          await onPlay(mix)
        } finally {
          setLoading(false)
        }
      }}
    />
  )
}

function HomePage() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const kind = useAuthStore((s) => sessionKind(s.token, s.user))
  const browse = useBrowseTracks(1, 50)
  const mixes = useFeaturedMixes()
  const albums = useAlbums()
  const artists = useArtists()
  const history = useHistory(20)
  const recentLocal = useLibraryStore((s) => s.recentTracks)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const [shuffling, setShuffling] = useState(false)

  const tracks = browse.data?.items ?? []
  const recent = useMemo(() => (history.data && history.data.length ? history.data : recentLocal).slice(0, 12), [history.data, recentLocal])

  const playMix = async (mix: FeaturedMix) => {
    try {
      const t = await fetchMixTracks(mix.endpoint)
      if (t.length) await playTrackWithQueue(t, 0, { type: 'mix', id: mix.id, title: mix.title })
      else toast('This mix is empty right now')
    } catch (e) {
      toast(`Couldn't load mix: ${(e as Error).message}`, { variant: 'error' })
    }
  }

  const shuffleAll = async () => {
    setShuffling(true)
    try {
      const t = await fetchShuffle(100)
      if (t.length) await playTrackWithQueue(t, 0, { type: 'radio', title: 'Library shuffle' })
    } catch (e) {
      toast(`Shuffle failed: ${(e as Error).message}`, { variant: 'error' })
    } finally {
      setShuffling(false)
    }
  }

  if (browse.isError && !browse.data) {
    return (
      <PageContainer>
        <ErrorState error={browse.error} onRetry={() => browse.refetch()} />
      </PageContainer>
    )
  }

  const hero = tracks[0]

  return (
    <PageContainer className="space-y-10">
      {/* Greeting + quick actions */}
      <section className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <p className="type-label-lg text-primary">{greeting()}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</p>
          <h1 className="type-headline-lg text-on-surface mt-1">
            {browse.isLoading ? <Skeleton className="h-9 w-72" /> : browse.data?.total ? `${browse.data.total.toLocaleString()} tracks, ready to play.` : 'Your library is waiting.'}
          </h1>
        </div>
        <div className="flex gap-2">
          <Button size="lg" icon={<Shuffle />} onClick={() => void shuffleAll()} loading={shuffling}>Shuffle everything</Button>
          {tracks.length > 0 && (
            <Button size="lg" variant="tonal" icon={<Play className="fill-current" />} onClick={() => void playTrackWithQueue(tracks, 0, { type: 'browse', title: 'Recently added' })}>Latest</Button>
          )}
        </div>
      </section>

      {/* Recently played (compact chips grid) */}
      {recent.length > 0 && (
        <section>
          <SectionHeader title="Jump back in" onMore={kind === 'user' ? () => navigate({ to: '/library', search: { tab: 'history' } }) : undefined} />
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
            {recent.slice(0, 6).map((t, i) => (
              <button
                key={t.id}
                onClick={() => void playTrackWithQueue(recent, i, { type: 'library', title: 'Recently played' })}
                className="group state-layer flex items-center gap-3 h-16 pr-3 rounded-md bg-surface-low overflow-hidden text-left"
              >
                <Artwork src={t.cover_url} alt="" className="size-16 rounded-none shrink-0" />
                <span className="min-w-0 flex-1">
                  <span className="block type-title-sm text-on-surface truncate">{t.title}</span>
                  <span className="block type-body-sm text-on-surface-variant truncate">{t.artist}</span>
                </span>
                <span className="size-9 rounded-full bg-primary text-on-primary flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                  <Play className="size-4 fill-current ml-0.5" />
                </span>
              </button>
            ))}
          </div>
        </section>
      )}

      {/* Mixes */}
      {(mixes.isLoading || (mixes.data && mixes.data.length > 0)) && (
        <Shelf title="Made for you" subtitle="Rotating mixes built from your library" itemWidth="w-44 sm:w-48">
          {mixes.isLoading
            ? [1, 2, 3, 4].map((i) => <div key={i} className="space-y-2"><Skeleton className="aspect-square" /><Skeleton variant="text" className="w-2/3" /></div>)
            : mixes.data!.map((m) => <MixCard key={m.id} mix={m} onPlay={playMix} />)}
        </Shelf>
      )}

      {/* Spotlight + latest tracks */}
      <section className="grid lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] gap-8">
        <div>
          <SectionHeader title="Newest addition" />
          {browse.isLoading ? (
            <Skeleton className="aspect-[4/3] w-full" />
          ) : hero ? (
            <div className="relative rounded-lg overflow-hidden bg-surface-low">
              <Artwork src={hero.cover_url} alt={hero.title} priority className="w-full aspect-[4/3] rounded-none" />
              <div className="absolute inset-0 bg-gradient-to-t from-surface via-surface/40 to-transparent" />
              <div className="absolute inset-x-0 bottom-0 p-5 flex items-end justify-between gap-4">
                <div className="min-w-0">
                  <p className="type-label-md text-primary inline-flex items-center gap-1"><Sparkles className="size-3.5" /> Just added</p>
                  <h3 className="type-headline-sm text-on-surface truncate mt-1">{hero.title}</h3>
                  <p className="type-body-md text-on-surface-variant truncate">{hero.artist}{hero.album ? ` · ${hero.album}` : ''}</p>
                </div>
                <button onClick={() => void playTrackWithQueue(tracks, 0, { type: 'browse', title: 'Recently added' })} aria-label="Play" className="state-layer size-14 rounded-full bg-primary text-on-primary flex items-center justify-center shadow-md3-2 shrink-0">
                  <Play className="size-6 fill-current ml-0.5" />
                </button>
              </div>
            </div>
          ) : null}
        </div>
        <div className="min-w-0">
          <SectionHeader
            title="Recently added"
            subtitle={browse.data ? `${browse.data.total.toLocaleString()} in the library` : undefined}
            action={tracks.length > 0 && <Button variant="text" size="sm" icon={<Play className="fill-current" />} onClick={() => void playTrackWithQueue(tracks, 0, { type: 'browse', title: 'Recently added' })}>Play all</Button>}
          />
          {browse.isLoading ? (
            <TrackRowSkeleton count={8} />
          ) : tracks.length === 0 ? (
            <div className="rounded-lg bg-surface-low p-8 text-center text-on-surface-variant">
              <Flame className="size-8 mx-auto mb-2 opacity-60" />
              <p className="type-title-md text-on-surface">No tracks yet</p>
              <p className="type-body-sm mt-1">Send audio to the StreamX bot to fill the library.</p>
            </div>
          ) : (
            <div className={cn('space-y-0.5')}>
              {tracks.slice(0, 10).map((t, i) => (
                <TrackRow key={t.id} track={t} index={i} tracks={tracks} showAlbum={false} onPlay={() => void playTrackWithQueue(tracks, i, { type: 'browse', title: 'Recently added' })} />
              ))}
            </div>
          )}
        </div>
      </section>

      {/* Albums */}
      <section>
        <SectionHeader title="Albums" onMore={() => navigate({ to: '/explore/albums' })} />
        {albums.isLoading ? (
          <CardGridSkeleton />
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-x-4 gap-y-6">
            {(albums.data ?? []).slice(0, 12).map((a) => (
              <MediaCard key={a.id} title={a.title} subtitle={[a.artist, a.year].filter(Boolean).join(' · ')} imageUrl={a.cover_url} onClick={() => navigate({ to: '/album/$albumId', params: { albumId: a.id } })} />
            ))}
          </div>
        )}
      </section>

      {/* Artists */}
      <section>
        <SectionHeader title="Artists" onMore={() => navigate({ to: '/explore/artists' })} />
        {artists.isLoading ? (
          <CardGridSkeleton circle />
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-x-4 gap-y-6">
            {(artists.data ?? []).slice(0, 12).map((ar) => (
              <MediaCard key={ar.id} title={ar.name} subtitle="Artist" imageUrl={ar.avatar_url} kind="artist" onClick={() => navigate({ to: '/artist/$artistId', params: { artistId: ar.id } })} />
            ))}
          </div>
        )}
      </section>
    </PageContainer>
  )
}
