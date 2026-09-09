import { create } from 'zustand'
import type { Track } from '@/schemas/track'
import type { Playlist } from '@/schemas/playlist'
import * as favApi from '@/api/favourites'
import * as plApi from '@/api/playlists'
import { useAuthStore, sessionKind } from './authStore'
import { toast } from './uiStore'

/**
 * Library = favourites + playlists + recents.
 * Backed by the server when signed in as a user; guest sessions keep a local
 * copy so the UI still works and can be merged later.
 */
interface LibraryStoreState {
  likedIds: Set<string>
  likedTracks: Track[]
  playlists: Playlist[]
  recentTracks: Track[]
  favouriteArtistIds: Set<string>
  loading: boolean
  synced: boolean
  lastError: string | null

  isLiked: (trackId: string) => boolean
  toggleLike: (track: Track) => Promise<boolean>
  isArtistFollowed: (artistId: string) => boolean
  toggleFollowArtist: (artistId: string) => Promise<boolean>
  addToRecent: (track: Track) => void
  sync: () => Promise<void>
  refreshPlaylists: () => Promise<void>
  createPlaylist: (name: string) => Promise<Playlist | null>
  renamePlaylist: (id: string, name: string) => Promise<void>
  deletePlaylist: (id: string) => Promise<void>
  addTracksToPlaylist: (playlistId: string, tracks: Track[]) => Promise<void>
  removeTrackFromPlaylist: (playlistId: string, trackId: string) => Promise<void>
  reset: () => void
}

const LOCAL_LIKES = 'webx.library.likes'
const LOCAL_RECENT = 'webx.library.recent'

const readJson = <T,>(key: string, fallback: T): T => {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : fallback
  } catch {
    return fallback
  }
}

const isUserSession = () => {
  const a = useAuthStore.getState()
  return sessionKind(a.token, a.user) === 'user'
}

export const useLibraryStore = create<LibraryStoreState>((set, get) => ({
  likedIds: new Set(readJson<Track[]>(LOCAL_LIKES, []).map((t) => t.id)),
  likedTracks: readJson<Track[]>(LOCAL_LIKES, []),
  playlists: [],
  recentTracks: readJson<Track[]>(LOCAL_RECENT, []),
  favouriteArtistIds: new Set(),
  loading: false,
  synced: false,
  lastError: null,

  isLiked: (id) => get().likedIds.has(id),

  toggleLike: async (track) => {
    const wasLiked = get().likedIds.has(track.id)
    // optimistic
    const ids = new Set(get().likedIds)
    let tracks = get().likedTracks
    if (wasLiked) {
      ids.delete(track.id)
      tracks = tracks.filter((t) => t.id !== track.id)
    } else {
      ids.add(track.id)
      tracks = [track, ...tracks.filter((t) => t.id !== track.id)]
    }
    set({ likedIds: ids, likedTracks: tracks })
    localStorage.setItem(LOCAL_LIKES, JSON.stringify(tracks.slice(0, 2000)))

    if (isUserSession()) {
      try {
        if (wasLiked) await favApi.removeFavourite(track.id)
        else await favApi.addFavourite(track.id)
      } catch (err) {
        // revert
        const revertIds = new Set(get().likedIds)
        let revertTracks = get().likedTracks
        if (wasLiked) {
          revertIds.add(track.id)
          revertTracks = [track, ...revertTracks]
        } else {
          revertIds.delete(track.id)
          revertTracks = revertTracks.filter((t) => t.id !== track.id)
        }
        set({ likedIds: revertIds, likedTracks: revertTracks })
        toast(`Couldn't update favourites: ${(err as Error).message}`, { variant: 'error' })
        return wasLiked
      }
    }
    return !wasLiked
  },

  isArtistFollowed: (id) => get().favouriteArtistIds.has(id),
  toggleFollowArtist: async (artistId) => {
    const was = get().favouriteArtistIds.has(artistId)
    const next = new Set(get().favouriteArtistIds)
    if (was) next.delete(artistId)
    else next.add(artistId)
    set({ favouriteArtistIds: next })
    if (isUserSession()) {
      try {
        if (was) await favApi.removeFavouriteArtist(artistId)
        else await favApi.addFavouriteArtist(artistId)
      } catch (err) {
        set({ favouriteArtistIds: get().favouriteArtistIds })
        toast(`Couldn't update artists: ${(err as Error).message}`, { variant: 'error' })
        return was
      }
    }
    return !was
  },

  addToRecent: (track) => {
    const updated = [track, ...get().recentTracks.filter((t) => t.id !== track.id)].slice(0, 100)
    set({ recentTracks: updated })
    localStorage.setItem(LOCAL_RECENT, JSON.stringify(updated))
  },

  sync: async () => {
    if (!isUserSession()) {
      set({ synced: true, playlists: [] })
      return
    }
    set({ loading: true, lastError: null })
    const results = await Promise.allSettled([
      favApi.fetchAllFavourites(2000),
      plApi.fetchMyPlaylists(),
      favApi.fetchFavouriteArtistIds(),
    ])
    const [favs, pls, artists] = results
    const patch: Partial<LibraryStoreState> = { loading: false, synced: true }
    if (favs.status === 'fulfilled') {
      patch.likedTracks = favs.value.items
      patch.likedIds = new Set(favs.value.items.map((t) => t.id))
      localStorage.setItem(LOCAL_LIKES, JSON.stringify(favs.value.items.slice(0, 2000)))
    } else patch.lastError = (favs.reason as Error)?.message ?? 'Failed to load favourites'
    if (pls.status === 'fulfilled') patch.playlists = pls.value
    if (artists.status === 'fulfilled') patch.favouriteArtistIds = new Set(artists.value)
    set(patch)
  },

  refreshPlaylists: async () => {
    if (!isUserSession()) return
    try {
      set({ playlists: await plApi.fetchMyPlaylists() })
    } catch {
      /* keep stale */
    }
  },

  createPlaylist: async (name) => {
    if (!isUserSession()) {
      toast('Sign in with an account to create playlists')
      return null
    }
    try {
      const pl = await plApi.createPlaylist(name)
      set({ playlists: [...get().playlists, pl] })
      toast(`Created “${pl.name}”`)
      return pl
    } catch (err) {
      toast(`Couldn't create playlist: ${(err as Error).message}`, { variant: 'error' })
      return null
    }
  },

  renamePlaylist: async (id, name) => {
    await plApi.renamePlaylist(id, name)
    set({ playlists: get().playlists.map((p) => (p.id === id ? { ...p, name } : p)) })
  },

  deletePlaylist: async (id) => {
    await plApi.deletePlaylist(id)
    set({ playlists: get().playlists.filter((p) => p.id !== id) })
  },

  addTracksToPlaylist: async (playlistId, tracks) => {
    const pl = get().playlists.find((p) => p.id === playlistId)
    try {
      await plApi.addTracksToPlaylist(playlistId, tracks.map((t) => t.id))
      toast(`Added ${tracks.length === 1 ? tracks[0].title : `${tracks.length} tracks`} to ${pl?.name ?? 'playlist'}`)
      void get().refreshPlaylists()
    } catch (err) {
      toast(`Couldn't add to playlist: ${(err as Error).message}`, { variant: 'error' })
    }
  },

  removeTrackFromPlaylist: async (playlistId, trackId) => {
    await plApi.removeTrackFromPlaylist(playlistId, trackId)
    void get().refreshPlaylists()
  },

  reset: () => set({ likedIds: new Set(), likedTracks: [], playlists: [], favouriteArtistIds: new Set(), synced: false }),
}))

// Re-sync whenever the session changes
if (typeof window !== 'undefined') {
  let lastToken = useAuthStore.getState().token
  useAuthStore.subscribe((s) => {
    if (s.token !== lastToken) {
      lastToken = s.token
      if (s.token) void useLibraryStore.getState().sync()
      else useLibraryStore.getState().reset()
    }
  })
}
