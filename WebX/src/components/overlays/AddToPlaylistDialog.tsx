import React, { useState } from 'react'
import { Plus, ListMusic, Check } from 'lucide-react'
import { Dialog, Button, TextField, ListItem } from '@/components/md3'
import { useUiStore } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { Artwork } from '@/components/common/Artwork'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { useNavigate } from '@tanstack/react-router'

export const AddToPlaylistDialog: React.FC = () => {
  const tracks = useUiStore((s) => s.addToPlaylist)
  const close = useUiStore((s) => s.closeAddToPlaylist)
  const playlists = useLibraryStore((s) => s.playlists)
  const addTracksToPlaylist = useLibraryStore((s) => s.addTracksToPlaylist)
  const createPlaylist = useLibraryStore((s) => s.createPlaylist)
  const isUser = useAuthStore((s) => sessionKind(s.token, s.user) === 'user')
  const navigate = useNavigate()
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [done, setDone] = useState<string | null>(null)

  if (!tracks) return null

  const add = async (id: string) => {
    setBusy(id)
    await addTracksToPlaylist(id, tracks)
    setBusy(null)
    setDone(id)
    setTimeout(close, 450)
  }

  const create = async () => {
    if (!name.trim()) return
    setBusy('new')
    const pl = await createPlaylist(name.trim())
    if (pl) await addTracksToPlaylist(pl.id, tracks)
    setBusy(null)
    close()
  }

  return (
    <Dialog
      open
      onClose={close}
      title={tracks.length === 1 ? 'Add to playlist' : `Add ${tracks.length} tracks to playlist`}
      actions={<Button variant="text" onClick={close}>Cancel</Button>}
    >
      {!isUser ? (
        <div className="space-y-4">
          <p>Playlists are stored on your account. Sign in with a username to create and sync playlists across devices.</p>
          <Button onClick={() => { close(); navigate({ to: '/login', search: { mode: 'account' } }) }}>Sign in</Button>
        </div>
      ) : (
        <div className="-mx-2">
          {creating ? (
            <div className="px-2 flex flex-col gap-3">
              <TextField label="Playlist name" value={name} onChange={(e) => setName(e.target.value)} autoFocus onKeyDown={(e) => e.key === 'Enter' && void create()} />
              <div className="flex justify-end gap-2">
                <Button variant="text" onClick={() => setCreating(false)}>Back</Button>
                <Button onClick={() => void create()} loading={busy === 'new'} disabled={!name.trim()}>Create & add</Button>
              </div>
            </div>
          ) : (
            <>
              <ListItem
                headline="New playlist"
                leading={<span className="size-10 rounded-sm bg-primary-container text-on-primary-container flex items-center justify-center"><Plus /></span>}
                onClick={() => setCreating(true)}
              />
              {playlists.map((pl) => (
                <ListItem
                  key={pl.id}
                  headline={pl.name}
                  supporting={pl.track_count != null ? `${pl.track_count} tracks` : undefined}
                  leading={pl.cover_url || pl.thumbnails.length ? <Artwork src={pl.cover_url} collage={pl.thumbnails} alt="" className="size-10 rounded-sm" /> : <span className="size-10 rounded-sm bg-surface-highest flex items-center justify-center"><ListMusic /></span>}
                  trailing={done === pl.id ? <Check className="text-primary" /> : busy === pl.id ? <span className="size-4 border-2 border-primary/30 border-t-primary rounded-full animate-spin" /> : undefined}
                  onClick={() => void add(pl.id)}
                />
              ))}
              {playlists.length === 0 && <p className="px-4 py-3 type-body-sm text-on-surface-variant">You don't have any playlists yet.</p>}
            </>
          )}
        </div>
      )}
    </Dialog>
  )
}
