import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { ArtistSchema, type Artist, type ArtistDetail } from '@/schemas/artist'
import { AlbumSchema } from '@/schemas/album'
import { parseTracks } from '@/schemas/track'

export async function fetchArtists(signal?: AbortSignal, limit = 200): Promise<Artist[]> {
  const raw = await http.get<{ items?: unknown[] } | unknown[]>(API_ENDPOINTS.ARTISTS, { signal, params: { limit } })
  const items = Array.isArray(raw) ? raw : raw?.items ?? []
  return items.map((i) => ArtistSchema.safeParse(i)).filter((r) => r.success).map((r) => r.data!).filter((a) => a.id)
}

export async function fetchArtistById(artistId: string, signal?: AbortSignal): Promise<ArtistDetail> {
  const res = await http.get<Record<string, unknown>>(API_ENDPOINTS.ARTIST_DETAILS(artistId), { signal })
  const artist = ArtistSchema.parse(res.artist ?? res)
  const popular = parseTracks(res.popular_tracks ?? res.top_tracks)
  let all = parseTracks(res.tracks ?? res.items)
  if (all.length === 0) {
    try {
      const t = await http.get<Record<string, unknown> | unknown[]>(API_ENDPOINTS.ARTIST_TRACKS(artistId), { signal, params: { limit: 200 } })
      all = parseTracks(Array.isArray(t) ? t : t.items ?? t.tracks)
    } catch {}
  }
  const rawAlbums = (res.releases ?? res.albums ?? []) as unknown[]
  const albums = (Array.isArray(rawAlbums) ? rawAlbums : []).map((a) => AlbumSchema.safeParse(a)).filter((r) => r.success).map((r) => r.data!)
  return {
    ...artist,
    top_tracks: popular.length ? popular : all.slice(0, 5),
    all_tracks: all.length ? all : popular,
    albums,
  }
}
