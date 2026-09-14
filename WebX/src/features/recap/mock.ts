/**
 * Sample recap — a realistic, deterministic snapshot for previewing the recap experience
 * without any listening history. Uses real recently-played tracks when available (so the
 * soundtrack works), otherwise the bundled demo catalog.
 */
import { MOCK_TRACKS } from '@/api/mockData'
import { useLibraryStore } from '@/stores/libraryStore'
import type { Track } from '@/schemas/track'
import type { RecapPeriodType, RecapSnapshot } from './types'

export const DEMO_TYPE = 'demo'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

function seeded(seed: number) {
  let x = seed
  return () => {
    x = (x * 1664525 + 1013904223) % 4294967296
    return x / 4294967296
  }
}

export function buildSampleSnapshot(type: RecapPeriodType = 'monthly'): RecapSnapshot & { demo: true; playable: boolean } {
  const now = new Date()
  const prevMonth = new Date(now.getFullYear(), now.getMonth() - 1, 1)
  const rnd = seeded(42)

  const recent = useLibraryStore.getState().recentTracks
  const pool: Track[] = (recent.length >= 6 ? recent : MOCK_TRACKS).slice(0, 12)
  const plays = pool.map((_, i) => Math.max(2, Math.round(34 * Math.pow(0.72, i) + rnd() * 3)))
  const topTracks = pool.map((t, i) => ({ track: t, plays: plays[i]!, minutes: Math.round((plays[i]! * (t.duration_sec || 210)) / 60) })).sort((a, b) => b.plays - a.plays)

  const artistMap = new Map<string, { plays: number; minutes: number; cover_url: string | null }>()
  for (const t of topTracks) {
    const name = t.track.artist || 'Unknown artist'
    const cur = artistMap.get(name) ?? { plays: 0, minutes: 0, cover_url: t.track.cover_url ?? null }
    cur.plays += t.plays
    cur.minutes += t.minutes
    artistMap.set(name, cur)
  }
  const topArtists = [...artistMap.entries()].map(([name, v]) => ({ name, ...v })).sort((a, b) => b.plays - a.plays)

  const albumMap = new Map<string, { album: string; artist: string; album_id: string | null; cover_url: string | null; plays: number; minutes: number }>()
  for (const t of topTracks) {
    if (!t.track.album) continue
    const key = `${t.track.album}::${t.track.artist}`
    const cur = albumMap.get(key) ?? { album: t.track.album, artist: t.track.artist || '', album_id: t.track.album_id ?? null, cover_url: t.track.cover_url ?? null, plays: 0, minutes: 0 }
    cur.plays += t.plays
    cur.minutes += t.minutes
    albumMap.set(key, cur)
  }
  const topAlbums = [...albumMap.values()].sort((a, b) => b.plays - a.plays)

  const totalPlays = topTracks.reduce((a, t) => a + t.plays, 0) + 61
  const totalMinutes = topTracks.reduce((a, t) => a + t.minutes, 0) + 190
  // night-leaning listening curve
  const byHour = Array.from({ length: 24 }, (_, h) => {
    const night = h >= 20 || h <= 1 ? 1 : 0.25
    const evening = h >= 17 && h < 20 ? 0.7 : 0
    return Math.round((8 + rnd() * 6) * (night + evening) * (h >= 2 && h < 7 ? 0.1 : 1))
  })
  const byWeekday = [38, 41, 35, 52, 74, 96, 88]
  const daysInMonth = type === 'weekly' ? 7 : 30
  const listeningByDay = Array.from({ length: daysInMonth }, (_, i) => ({ date: `${prevMonth.getFullYear()}-${String(prevMonth.getMonth() + 1).padStart(2, '0')}-${String(i + 1).padStart(2, '0')}`, minutes: Math.round(10 + rnd() * 70) }))
  const listeningByMonth = MONTHS.map((_, i) => ({ month: `${now.getFullYear()}-${String(i + 1).padStart(2, '0')}`, minutes: Math.round(400 + rnd() * 900 + (i === 9 ? 1400 : 0)) }))

  const label = type === 'weekly' ? 'Sep 1 – Sep 7, 2026' : type === 'yearly' ? String(now.getFullYear()) : `${MONTHS[prevMonth.getMonth()]} ${prevMonth.getFullYear()}`
  const period = type === 'weekly' ? '2026-W36' : type === 'yearly' ? String(now.getFullYear()) : `${prevMonth.getFullYear()}-${String(prevMonth.getMonth() + 1).padStart(2, '0')}`

  return {
    demo: true,
    playable: recent.length >= 6, // demo catalog tracks have no stream — only play real recents
    schemaVersion: 1,
    type,
    period,
    label,
    start: 0,
    end: 0,
    ongoing: false,
    generatedAt: Date.now() / 1000,
    eventCount: totalPlays,
    legacyData: false,
    stats: {
      totalMinutes,
      totalPlays,
      completedPlays: Math.round(totalPlays * 0.78),
      skippedPlays: 23,
      uniqueTracks: pool.length + 47,
      uniqueArtists: topArtists.length + 19,
      uniqueAlbums: topAlbums.length + 11,
      topTracks,
      topArtists,
      topAlbums,
      topGenres: [{ name: 'Indie', plays: 61 }, { name: 'Pop', plays: 40 }],
      listeningByHour: byHour,
      listeningByWeekday: byWeekday,
      listeningByDay,
      listeningByMonth,
      discoveryCount: 31,
      discoveryTracks: pool.slice(3, 9),
      repeatCount: Math.round(totalPlays * 0.46),
      mostActiveHour: 22,
      mostActiveWeekday: 5,
      mostActiveDate: listeningByDay[16]?.date ?? null,
      mostActiveDateMinutes: 142,
      longestSessionMinutes: 137,
      sessionCount: 44,
      nightShare: 0.58,
      earlyShare: 0.06,
      weekendShare: 0.43,
      losslessShare: 0.81,
      topArtistShare: topArtists[0] ? Math.min(0.5, topArtists[0].plays / totalPlays + 0.2) : 0,
    },
    personality: [
      { id: 'night_owl', name: 'Night Owl', detail: '58% of your listening happened after 9 PM' },
      { id: 'replayer', name: 'Replayer', detail: `${Math.round(totalPlays * 0.46)} replays — you know what you like` },
      { id: 'audiophile', name: 'Audiophile', detail: '81% of plays were lossless' },
    ],
    comparison: {
      period: type === 'weekly' ? '2026-W35' : type === 'yearly' ? String(now.getFullYear() - 1) : `${prevMonth.getFullYear()}-${String(prevMonth.getMonth()).padStart(2, '0') || '12'}`,
      totalMinutes: Math.round(totalMinutes / 1.22),
      totalPlays: Math.round(totalPlays / 1.15),
      uniqueArtists: topArtists.length + 5,
      uniqueTracks: pool.length + 30,
      minutesDeltaPct: 22,
      playsDeltaPct: 15,
      artistsDelta: 14,
      nightShareDelta: 9,
      topArtistPrev: topArtists[1]?.name ?? 'Someone else',
    },
  }
}
