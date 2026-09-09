import React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ListPlus, ListEnd, Heart, Disc3, User, Download, Link2, Radio, Share2 } from 'lucide-react'
import { Menu, type MenuItem } from '@/components/md3'
import { useUiStore, toast } from '@/stores/uiStore'
import { useQueueStore } from '@/stores/queueStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { getDownloadUrl } from '@/api/stream'
import { fetchShuffle } from '@/api/browse'
import { Artwork } from '@/components/common/Artwork'

/** Single global context menu for tracks, driven by uiStore.contextMenu */
export const TrackContextMenu: React.FC = () => {
  const state = useUiStore((s) => s.contextMenu)
  const close = useUiStore((s) => s.closeContextMenu)
  const openAddToPlaylist = useUiStore((s) => s.openAddToPlaylist)
  const navigate = useNavigate()
  const isLiked = useLibraryStore((s) => (state ? s.likedIds.has(state.track.id) : false))

  if (!state) return null
  const { track } = state
  const queue = useQueueStore.getState()

  const items: MenuItem[] = [
    { id: 'next', label: 'Play next', icon: <ListEnd />, onSelect: () => queue.playNext(track) },
    { id: 'queue', label: 'Add to queue', icon: <ListPlus />, onSelect: () => { queue.addToQueue(track); toast('Added to queue') } },
    { id: 'like', label: isLiked ? 'Remove from favourites' : 'Add to favourites', icon: <Heart className={isLiked ? 'fill-current text-primary' : ''} />, onSelect: () => void useLibraryStore.getState().toggleLike(track) },
    { id: 'playlist', label: 'Add to playlist…', icon: <ListPlus />, onSelect: () => openAddToPlaylist([track]) },
    { id: 'd1', label: '', divider: true },
    {
      id: 'radio',
      label: 'Start radio from this artist',
      icon: <Radio />,
      onSelect: async () => {
        try {
          const tracks = await fetchShuffle(50, { artist: track.artist })
          if (tracks.length) void queue.playTrackWithQueue([track, ...tracks.filter((t) => t.id !== track.id)], 0, { type: 'radio', title: `${track.artist} radio` })
          else toast('Not enough tracks for a radio')
        } catch {
          toast('Radio unavailable', { variant: 'error' })
        }
      },
    },
    ...(track.album_id ? [{ id: 'album', label: 'Go to album', icon: <Disc3 />, onSelect: () => navigate({ to: '/album/$albumId', params: { albumId: track.album_id! } }) }] : []),
    ...(track.artist_id ? [{ id: 'artist', label: 'Go to artist', icon: <User />, onSelect: () => navigate({ to: '/artist/$artistId', params: { artistId: track.artist_id! } }) }] : []),
    { id: 'd2', label: '', divider: true },
    {
      id: 'download',
      label: 'Download',
      icon: <Download />,
      onSelect: () => {
        const a = document.createElement('a')
        a.href = getDownloadUrl(track.id)
        a.download = `${track.artist} - ${track.title}`
        a.rel = 'noopener'
        a.click()
      },
    },
    {
      id: 'copy',
      label: 'Copy link',
      icon: <Link2 />,
      onSelect: async () => {
        const url = `${window.location.origin}/track/${encodeURIComponent(track.id)}`
        try {
          await navigator.clipboard.writeText(url)
          toast('Link copied')
        } catch {
          toast('Could not copy link', { variant: 'error' })
        }
      },
    },
    ...(typeof navigator !== 'undefined' && 'share' in navigator
      ? [{ id: 'share', label: 'Share…', icon: <Share2 />, onSelect: () => navigator.share({ title: track.title, text: `${track.title} — ${track.artist}`, url: `${window.location.origin}/track/${encodeURIComponent(track.id)}` }).catch(() => {}) }]
      : []),
  ]

  return (
    <Menu
      open
      onClose={close}
      anchor={state.anchor}
      align="end"
      items={items}
      header={
        <div className="flex items-center gap-3 py-1">
          <Artwork src={track.cover_url} alt="" className="size-10 rounded-sm" />
          <div className="min-w-0">
            <div className="type-title-sm truncate">{track.title}</div>
            <div className="type-body-sm text-on-surface-variant truncate">{track.artist}</div>
          </div>
        </div>
      }
    />
  )
}
