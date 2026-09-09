import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import { TrackSchema, parseTracks, type Track } from '@/schemas/track'
import { useSettingsStore } from '@/stores/settingsStore'
import { MOCK_TRACKS } from './mockData'

export { fetchTrackLyrics } from './lyrics'
export { getStreamUrl, getDownloadUrl, warmTrack } from './stream'

const demo = () => useSettingsStore.getState().demoMode

export async function fetchTrackById(trackId: string, signal?: AbortSignal): Promise<Track> {
  if (demo()) return MOCK_TRACKS.find((t) => t.id === trackId) ?? MOCK_TRACKS[0]
  const raw = await http.get<unknown>(API_ENDPOINTS.TRACK_DETAILS(trackId), { signal })
  return TrackSchema.parse(raw)
}

export async function fetchRandomTracks(limit = 20, signal?: AbortSignal): Promise<Track[]> {
  if (demo()) return [...MOCK_TRACKS].sort(() => 0.5 - Math.random()).slice(0, limit)
  const raw = await http.get<{ items?: unknown[] }>(API_ENDPOINTS.TRACKS_RANDOM, { signal, params: { limit }, noDedupe: true })
  return parseTracks(raw?.items)
}
