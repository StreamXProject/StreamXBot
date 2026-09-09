import { useQuery, useInfiniteQuery } from '@tanstack/react-query'
import { fetchBrowseTracks, fetchFeaturedMixes, fetchMixTracks, fetchShuffle, fetchTopics, fetchTopicTracks } from '@/api/browse'
import { fetchAlbums, fetchAlbumById } from '@/api/albums'
import { fetchArtists, fetchArtistById } from '@/api/artists'
import { fetchTrackLyrics } from '@/api/lyrics'
import { searchAll } from '@/api/search'
import { fetchHistory, fetchTopPlayed } from '@/api/favourites'
import { fetchPlaylistTracks, fetchSharedPlaylist } from '@/api/playlists'
import { fetchMe } from '@/api/auth'
import { useAuthStore, sessionKind } from '@/stores/authStore'

export const QUERY_KEYS = {
  BROWSE: ['browse'] as const,
  FEATURED_MIXES: ['featured_mixes'] as const,
  MIX_DETAIL: (id: string) => ['mixes', id] as const,
  ALBUMS: ['albums'] as const,
  ALBUM_DETAIL: (id: string) => ['albums', id] as const,
  ARTISTS: ['artists'] as const,
  ARTIST_DETAIL: (id: string) => ['artists', id] as const,
  SEARCH: (q: string) => ['search', q] as const,
  LYRICS: (id: string) => ['lyrics', id] as const,
  HISTORY: ['me', 'history'] as const,
  TOP_PLAYED: ['me', 'top-played'] as const,
  PLAYLIST_TRACKS: (id: string) => ['me', 'playlists', id, 'tracks'] as const,
  SHARED_PLAYLIST: (id: string) => ['share', 'playlists', id] as const,
  ME: ['me'] as const,
  TOPICS: ['topics'] as const,
  TOPIC_TRACKS: (n: string) => ['topics', n] as const,
  SHUFFLE: ['shuffle'] as const,
}

const useIsUser = () => useAuthStore((s) => sessionKind(s.token, s.user) === 'user')
const useHasToken = () => useAuthStore((s) => Boolean(s.token))

export function useBrowseTracks(page = 1, perPage = 50) {
  const enabled = useHasToken()
  return useQuery({
    queryKey: [...QUERY_KEYS.BROWSE, page, perPage],
    queryFn: ({ signal }) => fetchBrowseTracks(page, perPage, signal),
    enabled,
  })
}

export function useInfiniteBrowse(perPage = 50) {
  const enabled = useHasToken()
  return useInfiniteQuery({
    queryKey: [...QUERY_KEYS.BROWSE, 'infinite', perPage],
    queryFn: ({ pageParam, signal }) => fetchBrowseTracks(pageParam, perPage, signal),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
    enabled,
  })
}

export function useFeaturedMixes() {
  const enabled = useHasToken()
  return useQuery({ queryKey: QUERY_KEYS.FEATURED_MIXES, queryFn: ({ signal }) => fetchFeaturedMixes(signal), staleTime: 10 * 60_000, enabled })
}

export function useMixDetail(endpointOrKey: string) {
  return useQuery({
    queryKey: QUERY_KEYS.MIX_DETAIL(endpointOrKey),
    queryFn: ({ signal }) => fetchMixTracks(endpointOrKey, signal),
    enabled: Boolean(endpointOrKey),
  })
}

export function useShuffle(enabled: boolean, limit = 50) {
  return useQuery({ queryKey: [...QUERY_KEYS.SHUFFLE, limit], queryFn: ({ signal }) => fetchShuffle(limit, undefined, signal), enabled, staleTime: 60_000 })
}

export function useAlbums() {
  const enabled = useHasToken()
  return useQuery({ queryKey: QUERY_KEYS.ALBUMS, queryFn: ({ signal }) => fetchAlbums(signal), enabled })
}

export function useAlbumDetail(albumId: string) {
  return useQuery({ queryKey: QUERY_KEYS.ALBUM_DETAIL(albumId), queryFn: ({ signal }) => fetchAlbumById(albumId, signal), enabled: Boolean(albumId) })
}

export function useArtists() {
  const enabled = useHasToken()
  return useQuery({ queryKey: QUERY_KEYS.ARTISTS, queryFn: ({ signal }) => fetchArtists(signal), enabled })
}

export function useArtistDetail(artistId: string) {
  return useQuery({ queryKey: QUERY_KEYS.ARTIST_DETAIL(artistId), queryFn: ({ signal }) => fetchArtistById(artistId, signal), enabled: Boolean(artistId) })
}

export function useSearch(query: string) {
  return useQuery({
    queryKey: QUERY_KEYS.SEARCH(query),
    queryFn: ({ signal }) => searchAll(query, signal),
    enabled: query.trim().length > 0,
    staleTime: 2 * 60_000,
    placeholderData: (prev) => prev,
  })
}

export function useTrackLyrics(trackId?: string | null) {
  return useQuery({
    queryKey: QUERY_KEYS.LYRICS(trackId || ''),
    queryFn: ({ signal }) => fetchTrackLyrics(trackId!, signal),
    enabled: Boolean(trackId),
    staleTime: 60 * 60_000,
    gcTime: 60 * 60_000,
    retry: 0,
  })
}

export function useHistory(limit = 100) {
  const enabled = useIsUser()
  return useQuery({ queryKey: [...QUERY_KEYS.HISTORY, limit], queryFn: ({ signal }) => fetchHistory(limit, signal), enabled, staleTime: 30_000 })
}

export function useTopPlayed(limit = 50) {
  const enabled = useIsUser()
  return useQuery({ queryKey: [...QUERY_KEYS.TOP_PLAYED, limit], queryFn: ({ signal }) => fetchTopPlayed(limit, signal), enabled })
}

export function usePlaylistTracks(playlistId: string) {
  return useQuery({ queryKey: QUERY_KEYS.PLAYLIST_TRACKS(playlistId), queryFn: ({ signal }) => fetchPlaylistTracks(playlistId, 1, 500, signal), enabled: Boolean(playlistId) })
}

export function useSharedPlaylist(playlistId: string) {
  return useQuery({ queryKey: QUERY_KEYS.SHARED_PLAYLIST(playlistId), queryFn: ({ signal }) => fetchSharedPlaylist(playlistId, signal), enabled: Boolean(playlistId) })
}

export function useMe() {
  const enabled = useHasToken()
  return useQuery({ queryKey: QUERY_KEYS.ME, queryFn: ({ signal }) => fetchMe(signal), enabled, staleTime: 5 * 60_000, retry: 0 })
}

export function useTopics() {
  const enabled = useHasToken()
  return useQuery({ queryKey: QUERY_KEYS.TOPICS, queryFn: ({ signal }) => fetchTopics(signal), enabled, staleTime: 30 * 60_000, retry: 0 })
}

export function useTopicTracks(name: string) {
  return useQuery({ queryKey: QUERY_KEYS.TOPIC_TRACKS(name), queryFn: ({ signal }) => fetchTopicTracks(name, signal), enabled: Boolean(name) })
}
