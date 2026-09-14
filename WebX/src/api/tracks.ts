import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { TrackSchema, parseTracks, type Track } from '@/schemas/track'

export { fetchTrackLyrics } from './lyrics'
export { getStreamUrl, getDownloadUrl, warmTrack } from './stream'

export async function fetchTrackById(trackId: string, signal?: AbortSignal): Promise<Track> {
  const raw = await http.get<unknown>(API_ENDPOINTS.TRACK_DETAILS(trackId), { signal })
  return TrackSchema.parse(raw)
}

export async function fetchRandomTracks(limit = 20, signal?: AbortSignal): Promise<Track[]> {
  const raw = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.TRACKS_RANDOM, { signal, params: { limit }, noDedupe: true })
  return parseTracks(raw?.items)
}
