import React, { useMemo, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Heart, ListMusic, Clock, Play, Shuffle, Plus, TrendingUp, LogIn } from 'lucide-react'
import { useLibraryStore } from '@/stores/libraryStore'
import { useQueueStore } from '@/stores/queueStore'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { useSettingsStore, type LibraryTab } from '@/stores/settingsStore'
import { useHistory, useTopPlayed } from '@/hooks/useQueries'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { MediaCard } from '@/components/common/MediaCard'
import { PageContainer } from '@/components/common/PageContainer'
import { EmptyState } from '@/components/common/EmptyState'
import { TrackRowSkeleton } from '@/components/common/Skeleton'
import { Tabs, Button, Dialog, TextField } from '@/components/md3'
import { pluralize, formatLongDuration } from '@/lib/format'

export const Route = createFileRoute('/library')({
  validateSearch: (s: Record<string, unknown>): { tab?: LibraryTab | 'top' } =>
    (['liked', 'playlists', 'history', 'top'] as const).includes(s.tab as never) ? { tab: s.tab as LibraryTab | 'top' } : {},
  component: LibraryPage,
})

function LibraryPage() {
  const { tab: tabParam } = Route.useSearch()
  const navigate = useNavigate()
  const defaultTab = useSettingsStore((s) => s.defaultLibraryTab)
  const tab = (tabParam ?? defaultTab) as 'liked' | 'playlists' | 'history' | 'top'
  const setTab = (t: typeof tab) => navigate({ to: '/library', search: { tab: t }, replace: true })

  const kind = useAuthStore((s) => sessionKind(s.token, s.user))
  const liked = useLibraryStore((s) => s.likedTracks)
  const playlists = useLibraryStore((s) => s.playlists)
  const recentLocal = useLibraryStore((s) => s.recentTracks)
  const loading = useLibraryStore((s) => s.loading && !s.synced)
  const createPlaylist = useLibraryStore((s) => s.createPlaylist)
  const history = useHistory(200)
  const top = useTopPlayed(100)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')

  const historyTracks = useMemo(() => (history.data?.length ? history.data : recentLocal), [history.data, recentLocal])
  const likedDuration = useMemo(() => liked.reduce((a, t) => a + (t.duration_sec || 0), 0), [liked])

  return (
    <PageContainer className="space-y-5">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="type-headline-lg text-on-surface">Library</h1>
          <p className="type-body-md text-on-surface-variant mt-1">
            {pluralize(liked.length, 'favourite')} · {pluralize(playlists.length, 'playlist')}{kind !== 'user' ? ' · stored on this device' : ''}
          </p>
        </div>
        {kind === 'user' && <Button variant="tonal" icon={<Plus />} onClick={() => setCreating(true)}>New playlist</Button>}
      </div>

      <Tabs
        value={tab}
        onChange={setTab}
        items={[
          { value: 'liked', label: 'Liked', icon: <Heart /> },
          { value: 'playlists', label: 'Playlists', icon: <ListMusic /> },
          { value: 'history', label: 'History', icon: <Clock /> },
          { value: 'top', label: 'Most played', icon: <TrendingUp /> },
        ]}
        className="max-w-2xl"
      />

      {tab === 'liked' && (
        <section>
          {liked.length > 0 && (
            <div className="flex items-center gap-2 mb-3 overflow-x-auto scrollbar-none pb-0.5">
              <Button className="shrink-0" icon={<Play className="fill-current" />} onClick={() => void playTrackWithQueue(liked, 0, { type: 'library', title: 'Liked songs' })}>Play</Button>
              <Button className="shrink-0" variant="tonal" icon={<Shuffle />} onClick={() => void playTrackWithQueue([...liked].sort(() => Math.random() - 0.5), 0, { type: 'library', title: 'Liked songs' })}>Shuffle</Button>
              <span className="ml-auto type-body-sm text-on-surface-variant shrink-0">{formatLongDuration(likedDuration)}</span>
            </div>
          )}
          {loading ? (
            <TrackRowSkeleton />
          ) : (
            <VirtualTrackList
              tracks={liked}
              context={{ type: 'library', title: 'Liked songs' }}
              header={liked.length ? <TrackListHeader /> : undefined}
              emptyTitle="No favourites yet"
              emptyMessage="Tap the heart on any track to keep it here."
              emptyAction={<Button variant="tonal" onClick={() => navigate({ to: '/' })}>Browse the library</Button>}
            />
          )}
        </section>
      )}

      {tab === 'playlists' && (
        <section>
          {kind !== 'user' ? (
            <EmptyState icon={<LogIn />} title="Sign in to use playlists" description="Playlists live on your account so they follow you across devices." action={<Button onClick={() => navigate({ to: '/login', search: { mode: 'account' } })}>Sign in</Button>} />
          ) : playlists.length === 0 ? (
            <EmptyState icon={<ListMusic />} title="No playlists yet" description="Create one here, or right-click any track and choose “Add to playlist”." action={<Button icon={<Plus />} onClick={() => setCreating(true)}>New playlist</Button>} />
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-x-4 gap-y-6">
              {playlists.map((pl) => (
                <MediaCard key={pl.id} title={pl.name} subtitle={pl.track_count != null ? pluralize(pl.track_count, 'track') : 'Playlist'} imageUrl={pl.cover_url} collage={pl.thumbnails} kind="playlist" onClick={() => navigate({ to: '/playlist/$playlistId', params: { playlistId: pl.id } })} />
              ))}
            </div>
          )}
        </section>
      )}

      {tab === 'history' && (
        <section>
          {history.isLoading && kind === 'user' ? (
            <TrackRowSkeleton />
          ) : (
            <VirtualTrackList
              tracks={historyTracks}
              context={{ type: 'library', title: 'History' }}
              header={historyTracks.length ? <TrackListHeader /> : undefined}
              emptyTitle="Nothing played yet"
              emptyMessage="Your listening history shows up here."
            />
          )}
        </section>
      )}

      {tab === 'top' && (
        <section>
          {kind !== 'user' ? (
            <EmptyState icon={<TrendingUp />} title="Most played needs an account" description="Play counts are tracked per account on the server." action={<Button onClick={() => navigate({ to: '/login', search: { mode: 'account' } })}>Sign in</Button>} />
          ) : top.isLoading ? (
            <TrackRowSkeleton />
          ) : (
            <VirtualTrackList tracks={top.data ?? []} context={{ type: 'library', title: 'Most played' }} header={top.data?.length ? <TrackListHeader /> : undefined} emptyTitle="No plays counted yet" emptyMessage="Keep listening — your top tracks will appear here." />
          )}
        </section>
      )}

      <Dialog
        open={creating}
        onClose={() => setCreating(false)}
        title="New playlist"
        actions={<><Button variant="text" onClick={() => setCreating(false)}>Cancel</Button><Button disabled={!name.trim()} onClick={async () => { const pl = await createPlaylist(name.trim()); setCreating(false); setName(''); if (pl) navigate({ to: '/playlist/$playlistId', params: { playlistId: pl.id } }) }}>Create</Button></>}
      >
        <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Dialog>
    </PageContainer>
  )
}
