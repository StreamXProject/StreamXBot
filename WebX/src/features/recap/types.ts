import type { Track } from '@/schemas/track'

export type RecapPeriodType = 'weekly' | 'monthly' | 'yearly'

export interface RecapAvailable { type: RecapPeriodType; period: string; label: string; ongoing: boolean }

export interface TrackStat { track: Track; plays: number; minutes: number }
export interface ArtistStat { name: string; plays: number; minutes: number; cover_url?: string | null }
export interface AlbumStat { album: string; artist: string; album_id?: string | null; cover_url?: string | null; plays: number; minutes: number }

export interface RecapStats {
  totalMinutes: number
  totalPlays: number
  completedPlays: number
  skippedPlays: number
  uniqueTracks: number
  uniqueArtists: number
  uniqueAlbums: number
  topTracks: TrackStat[]
  topArtists: ArtistStat[]
  topAlbums: AlbumStat[]
  topGenres: { name: string; plays: number }[]
  listeningByHour: number[]
  listeningByWeekday: number[]
  listeningByDay: { date: string; minutes: number }[]
  listeningByMonth: { month: string; minutes: number }[]
  discoveryCount: number
  discoveryTracks: Track[]
  repeatCount: number
  mostActiveHour: number | null
  mostActiveWeekday: number | null
  mostActiveDate: string | null
  mostActiveDateMinutes: number
  longestSessionMinutes: number
  sessionCount: number
  nightShare: number
  earlyShare: number
  weekendShare: number
  losslessShare: number
  topArtistShare: number
}

export interface RecapPersonality { id: string; name: string; detail: string }

export interface RecapComparison {
  period: string
  totalMinutes: number
  totalPlays: number
  uniqueArtists: number
  uniqueTracks: number
  minutesDeltaPct: number | null
  playsDeltaPct: number | null
  artistsDelta: number
  nightShareDelta: number
  topArtistPrev: string | null
}

export interface RecapSnapshot {
  schemaVersion: number
  type: RecapPeriodType
  period: string
  label: string
  start: number
  end: number
  ongoing: boolean
  generatedAt: number
  eventCount: number
  legacyData: boolean
  stats: RecapStats
  personality: RecapPersonality[]
  comparison: RecapComparison | null
}

export interface RecapPublicSummary {
  type: RecapPeriodType
  period: string
  label: string
  totalMinutes: number
  totalPlays: number
  uniqueArtists: number
  uniqueTracks: number
  topArtist: ArtistStat | null
  topTrack: { title: string | null; artist: string | null; cover_url: string | null } | null
  personality: RecapPersonality[]
}

export type SceneAnimation = 'fade' | 'scaleFade' | 'slideUp' | 'zoom'

interface SceneBase { id: string; durationMs: number; enter: SceneAnimation; artwork?: string | null }
export type RecapScene =
  | (SceneBase & { type: 'intro'; label: string; eyebrow: string })
  | (SceneBase & { type: 'minutes'; minutes: number; plays: number; deltaPct: number | null; prevLabel: string | null })
  | (SceneBase & { type: 'top_artist'; artist: ArtistStat; runnersUp: ArtistStat[] })
  | (SceneBase & { type: 'top_track'; stat: TrackStat; runnersUp: TrackStat[] })
  | (SceneBase & { type: 'top_album'; album: AlbumStat })
  | (SceneBase & { type: 'discovery'; count: number; tracks: Track[]; share: number })
  | (SceneBase & { type: 'replayer'; repeats: number; stat: TrackStat })
  | (SceneBase & { type: 'habit'; byHour: number[]; byWeekday: number[]; peakHour: number | null; peakWeekday: number | null; nightShare: number })
  | (SceneBase & { type: 'trend'; byMonth: { month: string; minutes: number }[]; peakMonth: string; peakMinutes: number })
  | (SceneBase & { type: 'personality'; traits: RecapPersonality[] })
  | (SceneBase & { type: 'comparison'; comparison: RecapComparison; minutes: number; artists: number })
  | (SceneBase & { type: 'outro'; label: string; minutes: number; plays: number; artists: number; tracks: number; topArtist: ArtistStat | null; topTrack: TrackStat | null; traits: RecapPersonality[] })

export interface RecapSoundtrack { tracks: { track: Track; startMs: number }[] }

export interface RecapStory { scenes: RecapScene[]; soundtrack: RecapSoundtrack }
