export function formatDuration(seconds?: number | null): string {
  if (!seconds || !Number.isFinite(seconds) || seconds < 0) return '0:00'
  const s = Math.floor(seconds)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
  return `${m}:${sec.toString().padStart(2, '0')}`
}

export function formatLongDuration(seconds: number): string {
  if (!seconds || seconds < 60) return `${Math.max(0, Math.round(seconds))} sec`
  const h = Math.floor(seconds / 3600)
  const m = Math.round((seconds % 3600) / 60)
  if (h > 0) return `${h} hr ${m} min`
  return `${m} min`
}

export function formatCount(n?: number | null): string {
  if (n == null) return ''
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1).replace(/\.0$/, '')}K`
  return String(n)
}

export function formatQuality(type?: string | null, hz?: number | null): string | null {
  if (!type && !hz) return null
  const t = (type || '').toLowerCase()
  const label = t.includes('flac') ? 'FLAC' : t.includes('alac') ? 'ALAC' : t.includes('mp3') ? 'MP3' : t.includes('aac') || t.includes('m4a') ? 'AAC' : t.includes('opus') ? 'Opus' : t.includes('wav') ? 'WAV' : t ? t.toUpperCase() : ''
  const khz = hz ? `${(hz / 1000).toFixed(hz % 1000 ? 1 : 0)} kHz` : ''
  return [label, khz].filter(Boolean).join(' · ') || null
}

export function isLossless(type?: string | null): boolean {
  const t = (type || '').toLowerCase()
  return t.includes('flac') || t.includes('alac') || t.includes('wav') || t.includes('aiff')
}

export function relativeTime(ts?: number | null): string {
  if (!ts) return ''
  const ms = ts < 1e12 ? ts * 1000 : ts
  const diff = Date.now() - ms
  const m = Math.floor(diff / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(ms).toLocaleDateString()
}

export function pluralize(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`
}
