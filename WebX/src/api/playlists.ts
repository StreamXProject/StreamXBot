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

export async function fetchPlaylistTracks(id: string, page = 1, limit = 200, signal?: AbortSignal): Promise<{ items: Track[]; total: number }> {
  const data = await http.get<{ items?: unknown[]; total?: number }>(API_ENDPOINTS.ME_PLAYLIST_TRACKS(id), { params: { page, limit }, signal })
  return { items: parseTracks(data?.items), total: data?.total ?? 0 }
}

export async function fetchSharedPlaylist(id: string, signal?: AbortSignal): Promise<{ name: string; cover_url: string | null; tracks: Track[] }> {
  const data = await http.get<{ name?: string; cover_url?: string | null; tracks?: unknown[] }>(API_ENDPOINTS.SHARE_PLAYLIST(id), { signal, anonymous: true })
  return { name: data?.name ?? 'Playlist', cover_url: data?.cover_url ?? null, tracks: parseTracks(data?.tracks) }
}
