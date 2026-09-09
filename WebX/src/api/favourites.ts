import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { parseTracks, type Track } from '@/schemas/track'

export async function fetchFavouriteIds(signal?: AbortSignal): Promise<string[]> {
  const data = await http.get<{ ids?: string[] }>(API_ENDPOINTS.ME_FAVOURITE_IDS, { params: { limit: 5000 }, signal })
  return Array.isArray(data?.ids) ? data.ids : []
}

export async function fetchFavourites(page = 1, limit = 200, signal?: AbortSignal): Promise<{ items: Track[]; total: number }> {
  const data = await http.get<{ items?: unknown[]; total?: number }>(API_ENDPOINTS.ME_FAVOURITES, { params: { page, limit }, signal })
  return { items: parseTracks(data?.items), total: data?.total ?? 0 }
}

export async function addFavourite(trackId: string): Promise<void> {
  await http.post(API_ENDPOINTS.ME_FAVOURITES, { track_id: trackId })
}

export async function removeFavourite(trackId: string): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_FAVOURITE(trackId))
}

export async function fetchTopPlayed(limit = 50, signal?: AbortSignal): Promise<Track[]> {
  const data = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.ME_TOP_PLAYED, { params: { limit }, signal })
  return parseTracks(data?.items)
}

export async function fetchHistory(limit = 100, signal?: AbortSignal): Promise<Track[]> {
  const data = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.ME_HISTORY, { params: { limit }, signal })
  return parseTracks(data?.items)
}

export async function fetchFavouriteArtistIds(signal?: AbortSignal): Promise<string[]> {
  const data = await http.get<{ ids?: string[]; items?: string[] }>(API_ENDPOINTS.ME_ARTIST_FAVOURITE_IDS, { signal })
  return data?.ids ?? data?.items ?? []
}

export async function addFavouriteArtist(artistId: string): Promise<void> {
  await http.post(API_ENDPOINTS.ME_ARTIST_FAVOURITES, { artist_id: artistId })
}

export async function removeFavouriteArtist(artistId: string): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_ARTIST_FAVOURITE(artistId))
}
