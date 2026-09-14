import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { AlbumSchema, type Album, type AlbumDetail } from '@/schemas/album'
import { parseTracks } from '@/schemas/track'

export async function fetchAlbums(signal?: AbortSignal, limit = 200): Promise<Album[]> {
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(API_ENDPOINTS.ALBUMS, { signal, params: { limit } })
  const items = Array.isArray(raw) ? raw : raw?.items ?? []
  return items.map((i) => AlbumSchema.safeParse(i)).filter((r) => r.success).map((r) => r.data!).filter((a) => a.id)
}

export async function fetchAlbumById(albumId: string, signal?: AbortSignal): Promise<AlbumDetail> {
  const res = await http.get<Record<string, unknown>>(API_ENDPOINTS.ALBUM_DETAILS(albumId), { signal })
  const album = AlbumSchema.parse(res.album ?? res)
  let tracks = parseTracks(res.tracks ?? res.items)
  if (tracks.length === 0) {
    try {
      const t = await http.get<Record<string, unknown> | unknown[]>(API_ENDPOINTS.ALBUM_TRACKS(albumId), { signal })
      tracks = parseTracks(Array.isArray(t) ? t : t.items ?? t.tracks)
    } catch {}
  }
  return { ...album, tracks }
}

export async function fetchSavedAlbums(signal?: AbortSignal): Promise<Album[]> {
  const raw = await http.get<{ items?: Array<{ album_id: string; album?: Record<string, unknown> | null }> }>(API_ENDPOINTS.ME_ALBUMS, { signal, params: { limit: 200 } })
  return (raw?.items ?? [])
    .map((i) => AlbumSchema.safeParse({ id: i.album_id, ...(i.album ?? {}) }))
    .filter((r) => r.success)
    .map((r) => r.data!)
}

export async function saveAlbum(albumId: string): Promise<void> {
  await http.post(API_ENDPOINTS.ME_ALBUMS, { album_id: albumId })
}
