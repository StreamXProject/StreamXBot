import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import type { LyricLine, LyricWordSpan, Lyrics } from '@/schemas/track'
import { useSettingsStore } from '@/stores/settingsStore'

const LRC_LINE = /\[(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?\]/g
const WORD_TAG = /<(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?>/g
const OFFSET_TAG = /\[offset:\s*([+-]?\d+)\s*\]/i

function decodeEntities(text: string): string {
  return text
    .replace(/\ufeff/g, '')
    .replace(/&apos;/g, "'")
    .replace(/&#39;/g, "'")
    .replace(/&#039;/g, "'")
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
}

function parseSeconds(m: string, s: string, fracRaw?: string): number {
  const min = parseInt(m, 10) || 0
  const sec = parseInt(s, 10) || 0
  const frac = fracRaw ? parseInt(fracRaw.padEnd(3, '0').slice(0, 3), 10) / 1000 : 0
  return min * 60 + sec + frac
}

/** Char-count estimation mirroring piTube estimateWords / Apple Music V2 fallback */
export function estimateLineWords(line: LyricLine, activeDurationSec = 3.5): LyricWordSpan[] {
  if (line.spans && line.spans.length > 0) return line.spans
  const words = line.text.split(/\s+/).filter((w) => w.length > 0)
  if (words.length === 0) {
    return [{ time: line.time, duration: activeDurationSec, text: line.text }]
  }
  const totalChars = Math.max(1, line.text.length)
  let accumulated = line.time
  return words.map((word, i) => {
    const charCount = word.length + (i < words.length - 1 ? 1 : 0)
    const dur = Math.max(0.08, (activeDurationSec * charCount) / totalChars)
    const span: LyricWordSpan = {
      time: accumulated,
      duration: dur,
      text: word,
    }
    accumulated += dur
    return span
  })
}

/** Parse standard and enhanced (word-level) LRC text into timed lines. */
export function parseLrc(text: string): LyricLine[] {
  if (!text) return []
  const clean = decodeEntities(text)

  // Extract global offset in seconds if present
  let offsetSec = 0
  const offsetMatch = clean.match(OFFSET_TAG)
  if (offsetMatch) {
    const ms = parseInt(offsetMatch[1], 10)
    if (!Number.isNaN(ms)) offsetSec = ms / 1000
  }

  const rawLines = clean.split(/\r?\n/)
  const lines: LyricLine[] = []

  for (const raw of rawLines) {
    const trimmed = raw.trim()
    if (!trimmed || OFFSET_TAG.test(trimmed)) continue

    const timestamps: number[] = []
    let m: RegExpExecArray | null
    LRC_LINE.lastIndex = 0
    let lastIndex = 0

    while ((m = LRC_LINE.exec(trimmed))) {
      const t = parseSeconds(m[1], m[2], m[3]) - offsetSec
      timestamps.push(Math.max(0, t))
      lastIndex = LRC_LINE.lastIndex
      while (lastIndex < trimmed.length && trimmed[lastIndex] === ' ') lastIndex++
    }

    if (timestamps.length === 0 || lastIndex > trimmed.length) continue
    const content = trimmed.slice(lastIndex).trim()
    if (!content) continue

    // Check for word-level karaoke tags: <mm:ss.xx>word
    if (WORD_TAG.test(content)) {
      WORD_TAG.lastIndex = 0
      const spans: LyricWordSpan[] = []
      const wMatches: RegExpExecArray[] = []
      let wm: RegExpExecArray | null
      while ((wm = WORD_TAG.exec(content))) {
        wMatches.push(wm)
      }

      const firstTime = timestamps[0]

      if (wMatches.length > 0 && wMatches[0].index > 0) {
        const prefix = content.slice(0, wMatches[0].index).trim()
        if (prefix) {
          const nextTime = parseSeconds(wMatches[0][1], wMatches[0][2], wMatches[0][3]) - offsetSec
          spans.push({
            time: firstTime,
            duration: Math.max(0.05, nextTime - firstTime),
            text: prefix,
          })
        }
      }

      for (let i = 0; i < wMatches.length; i++) {
        const cur = wMatches[i]
        const wordTime = Math.max(0, parseSeconds(cur[1], cur[2], cur[3]) - offsetSec)
        const wordStart = cur.index + cur[0].length
        const wordEnd = i + 1 < wMatches.length ? wMatches[i + 1].index : content.length
        const wordText = content.slice(wordStart, wordEnd).trim()
        if (wordText) {
          let dur = 0.35
          if (i + 1 < wMatches.length) {
            const nextTime = Math.max(0, parseSeconds(wMatches[i + 1][1], wMatches[i + 1][2], wMatches[i + 1][3]) - offsetSec)
            dur = Math.max(0.05, nextTime - wordTime)
          }
          spans.push({
            time: wordTime,
            duration: dur,
            text: wordText,
          })
        }
      }

      const strippedText = content.replace(WORD_TAG, '').replace(/\s+/g, ' ').trim()
      for (const t of timestamps) {
        lines.push({
          time: t,
          text: strippedText,
          spans: spans.length > 0 ? spans : undefined,
        })
      }
    } else {
      for (const t of timestamps) {
        lines.push({ time: t, text: content })
      }
    }
  }

  lines.sort((a, b) => a.time - b.time)

  // Compute line durations and populate fallback word spans
  for (let i = 0; i < lines.length; i++) {
    const cur = lines[i]
    const nextTime = i + 1 < lines.length ? lines[i + 1].time : cur.time + 4.5
    const lineDur = Math.max(0.5, Math.min(nextTime - cur.time, 7.0))
    cur.duration = lineDur

    if (!cur.spans || cur.spans.length === 0) {
      cur.spans = estimateLineWords(cur, lineDur)
    } else {
      // Ensure the last span's duration extends reasonably towards line end
      const lastSpan = cur.spans[cur.spans.length - 1]
      if (!lastSpan.duration || lastSpan.duration <= 0.05) {
        lastSpan.duration = Math.max(0.2, (cur.time + lineDur) - lastSpan.time)
      }
    }
  }

  return lines
}

export function stripLrcTags(text: string): string {
  return decodeEntities(text)
    .split(/\r?\n/)
    .map((l) => l.replace(/\[[^\]]*\]/g, '').replace(/<[^>]*>/g, '').trim())
    .filter((l, i, arr) => l.length > 0 || (i > 0 && arr[i - 1].length > 0))
    .join('\n')
    .trim()
}

export async function fetchTrackLyrics(trackId: string, signal?: AbortSignal, provider?: string): Promise<Lyrics> {
  const chosenProvider = provider || useSettingsStore.getState().lyricsProvider
  const params: Record<string, string> = { format: 'json' }
  if (chosenProvider && chosenProvider !== 'auto') {
    params.provider = chosenProvider
  }

  const raw = await http.get<unknown>(API_ENDPOINTS.TRACK_LYRICS(trackId), { params, signal, parse: 'json' })
  let text = ''
  let source: string | undefined
  let kind: string | undefined

  if (typeof raw === 'string') {
    text = raw
  } else if (raw && typeof raw === 'object') {
    const o = raw as Record<string, unknown>
    if (o.ok === false) return { lines: [], plain: '', synced: false }
    if (typeof o.lyrics === 'string') text = o.lyrics
    else if (Array.isArray(o.lyrics)) {
      const lines = (o.lyrics as LyricLine[]).filter((l) => typeof l?.time === 'number' && typeof l?.text === 'string')
      return { lines, plain: lines.map((l) => l.text).join('\n'), synced: lines.length > 0, source: typeof o.source === 'string' ? o.source : undefined, kind: typeof o.kind === 'string' ? o.kind : undefined }
    }
    source = typeof o.source === 'string' ? o.source : undefined
    kind = typeof o.kind === 'string' ? o.kind : undefined
  }

  const lines = parseLrc(text)
  return { lines, plain: stripLrcTags(text), synced: lines.length > 0, source, kind }
}
