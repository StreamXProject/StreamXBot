import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import type { LyricLine, Lyrics } from '@/schemas/track'

const LRC_LINE = /\[(\d{1,2}):(\d{1,2})(?:[.:](\d{1,3}))?\]/g

/** Parse LRC text into timed lines. Returns [] if the text has no timestamps. */
export function parseLrc(text: string): LyricLine[] {
  const lines: LyricLine[] = []
  for (const raw of text.split(/\r?\n/)) {
    const stamps: number[] = []
    let m: RegExpExecArray | null
    LRC_LINE.lastIndex = 0
    let lastIndex = 0
    while ((m = LRC_LINE.exec(raw))) {
      const min = parseInt(m[1], 10)
      const sec = parseInt(m[2], 10)
      const fracRaw = m[3] ?? '0'
      const frac = parseInt(fracRaw.padEnd(3, '0').slice(0, 3), 10) / 1000
      stamps.push(min * 60 + sec + frac)
      lastIndex = LRC_LINE.lastIndex
    }
    if (stamps.length === 0) continue
    const content = raw.slice(lastIndex).trim()
    for (const t of stamps) lines.push({ time: t, text: content })
  }
  lines.sort((a, b) => a.time - b.time)
  return lines
}

export function stripLrcTags(text: string): string {
  return text
    .split(/\r?\n/)
    .map((l) => l.replace(/\[[^\]]*\]/g, '').trim())
    .filter((l, i, arr) => l.length > 0 || (i > 0 && arr[i - 1].length > 0))
    .join('\n')
    .trim()
}

export async function fetchTrackLyrics(trackId: string, signal?: AbortSignal): Promise<Lyrics> {
  const raw = await http.get<unknown>(API_ENDPOINTS.TRACK_LYRICS(trackId), { params: { format: 'json' }, signal, parse: 'json' })
  let text = ''
  let source: string | undefined
  if (typeof raw === 'string') text = raw
  else if (raw && typeof raw === 'object') {
    const o = raw as Record<string, unknown>
    if (o.ok === false) return { lines: [], plain: '', synced: false }
    if (typeof o.lyrics === 'string') text = o.lyrics
    else if (Array.isArray(o.lyrics)) {
      const lines = (o.lyrics as LyricLine[]).filter((l) => typeof l?.time === 'number' && typeof l?.text === 'string')
      return { lines, plain: lines.map((l) => l.text).join('\n'), synced: lines.length > 0 }
    }
    source = typeof o.source === 'string' ? o.source : undefined
  }
  const lines = parseLrc(text)
  return { lines, plain: stripLrcTags(text), synced: lines.length > 0, source }
}
