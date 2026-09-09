import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { parseTracks, type Track } from '@/schemas/track'
import { PlaylistSchema, type Playlist } from '@/schemas/playlist'

export async function fetchMyPlaylists(signal?: AbortSignal): Promise<Playlist[]> {
  const data = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.ME_PLAYLISTS, { signal })
  return (data?.items ?? []).map((i) => PlaylistSchema.safeParse(i)).filter((r) => r.success).map((r) => r.data!)
}

export async function createPlaylist(name: string): Promise<Playlist> {
  const data = await http.post<unknown>(API_ENDPOINTS.ME_PLAYLISTS, { name })
  return PlaylistSchema.parse(data)
}

export async function renamePlaylist(id: string, name: string): Promise<void> {
  await http.patch(API_ENDPOINTS.ME_PLAYLIST(id), { name })
}

export async function deletePlaylist(id: string): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_PLAYLIST(id))
}

export async function addTracksToPlaylist(id: string, trackIds: string[]): Promise<void> {
  await http.post(API_ENDPOINTS.ME_PLAYLIST_TRACKS(id), { track_ids: trackIds })
}

export async function removeTrackFromPlaylist(id: string, trackId: string): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_PLAYLIST_TRACK(id, trackId))
}

// Backend caps /me/playlists/{id}/tracks at limit<=200.
const PLAYLIST_PAGE_MAX = 200

export async function fetchPlaylistTracks(id: string, page = 1, limit = 200, signal?: AbortSignal): Promise<{ items: Track[]; total: number }> {
  const safeLimit = Math.min(Math.max(1, limit), PLAYLIST_PAGE_MAX)
  const data = await http.get<{ items?: unknown[]; total?: number }>(API_ENDPOINTS.ME_PLAYLIST_TRACKS(id), { params: { page, limit: safeLimit }, signal })
  return { items: parseTracks(data?.items), total: data?.total ?? 0 }
}

/** Fetch every track in a playlist by walking pages under the backend cap. */
export async function fetchAllPlaylistTracks(id: string, max = 5000, signal?: AbortSignal): Promise<{ items: Track[]; total: number }> {
  const items: Track[] = []
  let total = 0
  for (let page = 1; items.length < max; page++) {
    const res = await fetchPlaylistTracks(id, page, PLAYLIST_PAGE_MAX, signal)
    items.push(...res.items)
    total = res.total || items.length
    if (res.items.length < PLAYLIST_PAGE_MAX || items.length >= total) break
  }
  return { items, total: Math.max(total, items.length) }
}

export async function fetchSharedPlaylist(id: string, signal?: AbortSignal): Promise<{ name: string; cover_url: string | null; tracks: Track[] }> {
  const data = await http.get<{ name?: string; cover_url?: string | null; tracks?: unknown[] }>(API_ENDPOINTS.SHARE_PLAYLIST(id), { signal, anonymous: true })
  return { name: data?.name ?? 'Playlist', cover_url: data?.cover_url ?? null, tracks: parseTracks(data?.tracks) }
}
