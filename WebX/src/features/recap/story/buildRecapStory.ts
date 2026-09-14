/**
 * Story builder — turns a RecapSnapshot into an ordered, conditional list of scenes plus a soundtrack.
 * Pure and renderer-independent: the same output could drive a video renderer or a native client.
 */
import type { RecapScene, RecapSnapshot, RecapStory } from '../types'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export function periodTitle(snap: Pick<RecapSnapshot, 'type' | 'label' | 'period'>): string {
  if (snap.type === 'weekly') return 'Your week'
  if (snap.type === 'yearly') return `Your ${snap.period}`
  return `Your ${snap.label.split(' ')[0]}`
}

export function buildRecapStory(snap: RecapSnapshot): RecapStory {
  const s = snap.stats
  const scenes: RecapScene[] = []
  const topTrack = s.topTracks[0]
  const topArtist = s.topArtists[0]
  const topAlbum = s.topAlbums[0]
  const art = (i: number) => s.topTracks[i]?.track.cover_url ?? topTrack?.track.cover_url ?? null

  scenes.push({ id: 'intro', type: 'intro', durationMs: 4500, enter: 'fade', eyebrow: snap.label, label: periodTitle(snap), artwork: art(0) })

  if (s.totalPlays === 0) {
    scenes.push({ id: 'outro', type: 'outro', durationMs: 12000, enter: 'fade', label: snap.label, minutes: 0, plays: 0, artists: 0, tracks: 0, topArtist: null, topTrack: null, traits: [] })
    return { scenes, soundtrack: { tracks: [] } }
  }

  scenes.push({
    id: 'minutes',
    type: 'minutes',
    durationMs: 6000,
    enter: 'scaleFade',
    minutes: s.totalMinutes,
    plays: s.totalPlays,
    deltaPct: snap.comparison?.minutesDeltaPct ?? null,
    prevLabel: snap.comparison ? (snap.type === 'weekly' ? 'last week' : snap.type === 'monthly' ? 'last month' : 'last year') : null,
    artwork: art(1),
  })

  if (topArtist) scenes.push({ id: 'top_artist', type: 'top_artist', durationMs: 6500, enter: 'zoom', artist: topArtist, runnersUp: s.topArtists.slice(1, 5), artwork: topArtist.cover_url ?? art(0) })
  if (topTrack) scenes.push({ id: 'top_track', type: 'top_track', durationMs: 6500, enter: 'scaleFade', stat: topTrack, runnersUp: s.topTracks.slice(1, 5), artwork: topTrack.track.cover_url })
  if (topAlbum && topAlbum.plays >= 5) scenes.push({ id: 'top_album', type: 'top_album', durationMs: 5500, enter: 'slideUp', album: topAlbum, artwork: topAlbum.cover_url ?? art(0) })

  // conditional scenes — make the recap feel individually generated
  if (s.discoveryCount >= 5 && s.uniqueTracks > 0) {
    scenes.push({ id: 'discovery', type: 'discovery', durationMs: 6000, enter: 'slideUp', count: s.discoveryCount, tracks: s.discoveryTracks, share: s.discoveryCount / s.uniqueTracks, artwork: s.discoveryTracks[0]?.cover_url ?? art(2) })
  }
  if (s.totalPlays >= 10 && s.repeatCount / s.totalPlays >= 0.4 && topTrack && topTrack.plays >= 3) {
    scenes.push({ id: 'replayer', type: 'replayer', durationMs: 5500, enter: 'zoom', repeats: s.repeatCount, stat: topTrack, artwork: topTrack.track.cover_url })
  }
  if (s.totalMinutes >= 30) {
    scenes.push({ id: 'habit', type: 'habit', durationMs: 7000, enter: 'fade', byHour: s.listeningByHour, byWeekday: s.listeningByWeekday, peakHour: s.mostActiveHour, peakWeekday: s.mostActiveWeekday, nightShare: s.nightShare, artwork: art(3) })
  }
  if (snap.type === 'yearly' && s.listeningByMonth.length >= 3) {
    const peak = s.listeningByMonth.reduce((a, b) => (b.minutes > a.minutes ? b : a))
    scenes.push({ id: 'trend', type: 'trend', durationMs: 7000, enter: 'fade', byMonth: s.listeningByMonth, peakMonth: MONTHS[Number(peak.month.split('-')[1]) - 1] ?? peak.month, peakMinutes: peak.minutes, artwork: art(4) })
  }
  if (snap.personality.length) scenes.push({ id: 'personality', type: 'personality', durationMs: 6500, enter: 'scaleFade', traits: snap.personality, artwork: art(1) })
  if (snap.comparison && snap.comparison.totalPlays > 0 && snap.type !== 'yearly') {
    scenes.push({ id: 'comparison', type: 'comparison', durationMs: 6000, enter: 'slideUp', comparison: snap.comparison, minutes: s.totalMinutes, artists: s.uniqueArtists, artwork: art(2) })
  }

  scenes.push({
    id: 'outro',
    type: 'outro',
    durationMs: 15000,
    enter: 'fade',
    label: snap.label,
    minutes: s.totalMinutes,
    plays: s.totalPlays,
    artists: s.uniqueArtists,
    tracks: s.uniqueTracks,
    topArtist: topArtist ?? null,
    topTrack: topTrack ?? null,
    traits: snap.personality,
    artwork: art(0),
  })

  // soundtrack: top tracks, one per ~2 scenes, driven by the existing audio engine
  const soundtrack = s.topTracks.slice(0, 3).map((t, i) => ({ track: t.track, startMs: i * 30_000 }))
  return { scenes, soundtrack: { tracks: soundtrack } }
}
