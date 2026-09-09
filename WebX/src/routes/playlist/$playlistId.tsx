import React, { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Pencil, Trash2, Link2, X } from 'lucide-react'
import { usePlaylistTracks } from '@/hooks/useQueries'
import { useLibraryStore } from '@/stores/libraryStore'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { useQueryClient } from '@tanstack/react-query'
import { CollectionHeader } from '@/components/common/CollectionHeader'
import { VirtualTrackList, TrackListHeader } from '@/components/common/VirtualTrackList'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { TrackRowSkeleton } from '@/components/common/Skeleton'
import { Menu, Dialog, Button, TextField, IconButton } from '@/components/md3'
import { pluralize, formatLongDuration } from '@/lib/format'

export const Route = createFileRoute('/playlist/$playlistId')({
  component: PlaylistPage,
})

function PlaylistPage() {
  const { playlistId } = Route.useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const playlist = useLibraryStore((s) => s.playlists.find((p) => p.id === playlistId))
  const renamePlaylist = useLibraryStore((s) => s.renamePlaylist)
  const deletePlaylist = useLibraryStore((s) => s.deletePlaylist)
  const removeTrackFromPlaylist = useLibraryStore((s) => s.removeTrackFromPlaylist)
  const confirmDestructive = useSettingsStore((s) => s.confirmDestructive)
  const { data, isLoading, isError, error, refetch } = usePlaylistTracks(playlistId)
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const contextId = useQueueStore((s) => s.context?.id)
  const isPlaying = usePlayerStore((s) => s.isPlaying) && contextId === playlistId
  const togglePlay = usePlayerStore((s) => s.togglePlay)
  const [menu, setMenu] = useState<HTMLElement | null>(null)
  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(playlist?.name ?? '')

  const tracks = data?.items ?? []
  const ctx = { type: 'playlist' as const, id: playlistId, title: playlist?.name ?? 'Playlist' }
  const duration = tracks.reduce((a, t) => a + (t.duration_sec || 0), 0)

  if (isError) return <PageContainer><ErrorState error={error} onRetry={() => refetch()} /></PageContainer>

  return (
    <PageContainer>
      <CollectionHeader
        kind="playlist"
        eyebrow="Playlist"
        title={playlist?.name ?? 'Playlist'}
        meta={isLoading ? 'Loading…' : `${pluralize(tracks.length, 'track')} · ${formatLongDuration(duration)}`}
        imageUrl={playlist?.cover_url}
        collage={playlist?.thumbnails}
        isPlaying={isPlaying}
        onPlay={() => (isPlaying ? togglePlay() : tracks.length && void playTrackWithQueue(tracks, 0, ctx))}
        onShuffle={() => tracks.length && void playTrackWithQueue([...tracks].sort(() => Math.random() - 0.5), 0, ctx)}
        onMore={(a) => setMenu(a)}
      />
      {isLoading ? (
        <TrackRowSkeleton />
      ) : (
        <VirtualTrackList
          tracks={tracks}
          context={ctx}
          header={tracks.length ? <TrackListHeader /> : undefined}
          emptyTitle="This playlist is empty"
          emptyMessage="Right-click any track → Add to playlist."
          trailing={(t) => (
            <IconButton
              label="Remove from playlist"
              size="sm"
              className="opacity-0 group-hover:opacity-100 max-md:opacity-100"
              onClick={async (e) => {
                e.stopPropagation()
                await removeTrackFromPlaylist(playlistId, t.id)
                qc.invalidateQueries({ queryKey: ['me', 'playlists', playlistId] })
                toast(`Removed “${t.title}”`)
              }}
            >
              <X />
            </IconButton>
          )}
        />
      )}

      <Menu
        open={Boolean(menu)}
        onClose={() => setMenu(null)}
        anchor={menu}
        align="end"
        items={[
          { id: 'rename', label: 'Rename', icon: <Pencil />, onSelect: () => { setName(playlist?.name ?? ''); setRenaming(true) } },
          {
            id: 'share',
            label: 'Copy share link',
            icon: <Link2 />,
            onSelect: () => navigator.clipboard.writeText(`${window.location.origin}/share/playlist/${playlistId}`).then(() => toast('Link copied')),
          },
          { id: 'd', label: '', divider: true },
          {
            id: 'delete',
            label: 'Delete playlist',
            icon: <Trash2 />,
            destructive: true,
            onSelect: async () => {
              if (confirmDestructive && !confirm(`Delete “${playlist?.name}”? This cannot be undone.`)) return
              await deletePlaylist(playlistId)
              toast('Playlist deleted')
              navigate({ to: '/library', search: { tab: 'playlists' } })
            },
          },
        ]}
      />
      <Dialog open={renaming} onClose={() => setRenaming(false)} title="Rename playlist" actions={<><Button variant="text" onClick={() => setRenaming(false)}>Cancel</Button><Button disabled={!name.trim()} onClick={async () => { await renamePlaylist(playlistId, name.trim()); setRenaming(false) }}>Save</Button></>}>
        <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Dialog>
    </PageContainer>
  )
}
