/** Client-side 1080×1920 share card renderer (canvas). Returns a PNG blob. */
import type { RecapSnapshot } from './types'

function cssVar(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

async function loadImage(url: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => resolve(img)
    img.onerror = () => resolve(null)
    img.src = url
  })
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.moveTo(x + r, y)
  ctx.arcTo(x + w, y, x + w, y + h, r)
  ctx.arcTo(x + w, y + h, x, y + h, r)
  ctx.arcTo(x, y + h, x, y, r)
  ctx.arcTo(x, y, x + w, y, r)
  ctx.closePath()
}

function fitText(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string {
  if (ctx.measureText(text).width <= maxWidth) return text
  let t = text
  while (t.length > 1 && ctx.measureText(t + '…').width > maxWidth) t = t.slice(0, -1)
  return t + '…'
}

export async function renderShareCard(snap: RecapSnapshot): Promise<Blob> {
  const W = 1080
  const H = 1920
  const canvas = document.createElement('canvas')
  canvas.width = W
  canvas.height = H
  const ctx = canvas.getContext('2d')!
  const surface = cssVar('--md-sys-color-surface', '#1A1011')
  const primary = cssVar('--md-sys-color-primary', '#FFB3B3')
  const onSurface = cssVar('--md-sys-color-on-surface', '#F5DDDE')
  const variant = cssVar('--md-sys-color-on-surface-variant', '#DAC0C1')
  const font = cssVar('--md-ref-typeface-brand', 'Manrope, system-ui, sans-serif')

  const s = snap.stats
  const top = s.topTracks[0]
  const cover = top?.track.cover_url ? await loadImage(top.track.cover_url) : null

  // background: blurred cover + tint
  ctx.fillStyle = surface
  ctx.fillRect(0, 0, W, H)
  if (cover) {
    ctx.save()
    ctx.filter = 'blur(60px) saturate(1.3)'
    ctx.globalAlpha = 0.55
    ctx.drawImage(cover, -200, -200, W + 400, H + 400)
    ctx.restore()
    const g = ctx.createLinearGradient(0, 0, 0, H)
    g.addColorStop(0, `${surface}99`)
    g.addColorStop(0.5, `${surface}cc`)
    g.addColorStop(1, surface)
    ctx.fillStyle = g
    ctx.fillRect(0, 0, W, H)
  }

  ctx.textBaseline = 'top'
  ctx.fillStyle = primary
  ctx.font = `600 40px ${font}`
  ctx.fillText(snap.label.toUpperCase(), 96, 140)
  ctx.fillStyle = onSurface
  ctx.font = `700 110px ${font}`
  ctx.fillText(snap.type === 'weekly' ? 'My week' : snap.type === 'yearly' ? `My ${snap.period}` : `My ${snap.label.split(' ')[0]}`, 96, 200)
  ctx.font = `500 44px ${font}`
  ctx.fillStyle = variant
  ctx.fillText('in WebX', 96, 330)

  // cover
  const size = 620
  const cx = (W - size) / 2
  const cy = 440
  ctx.save()
  roundRect(ctx, cx, cy, size, size, 48)
  ctx.clip()
  if (cover) ctx.drawImage(cover, cx, cy, size, size)
  else {
    ctx.fillStyle = `${primary}33`
    ctx.fillRect(cx, cy, size, size)
  }
  ctx.restore()

  // big number
  ctx.fillStyle = onSurface
  ctx.font = `800 170px ${font}`
  const minutes = s.totalMinutes.toLocaleString()
  ctx.fillText(minutes, 96, 1130)
  ctx.fillStyle = variant
  ctx.font = `500 48px ${font}`
  ctx.fillText('minutes of music', 96, 1310)

  // stat row
  const stats: [string, string][] = [
    [s.totalPlays.toLocaleString(), 'plays'],
    [s.uniqueArtists.toLocaleString(), 'artists'],
    [s.uniqueTracks.toLocaleString(), 'tracks'],
  ]
  stats.forEach(([n, l], i) => {
    const x = 96 + i * 300
    ctx.fillStyle = onSurface
    ctx.font = `700 64px ${font}`
    ctx.fillText(n, x, 1400)
    ctx.fillStyle = variant
    ctx.font = `500 34px ${font}`
    ctx.fillText(l, x, 1475)
  })

  // top artist / track
  const rows: [string, string][] = []
  if (s.topArtists[0]) rows.push(['#1 artist', s.topArtists[0].name])
  if (top) rows.push(['#1 track', `${top.track.title}${top.track.artist ? ` · ${top.track.artist}` : ''}`])
  if (snap.personality[0]) rows.push(['Listener type', snap.personality[0].name])
  rows.forEach(([k, v], i) => {
    const y = 1580 + i * 92
    ctx.fillStyle = primary
    ctx.font = `600 30px ${font}`
    ctx.fillText(k.toUpperCase(), 96, y)
    ctx.fillStyle = onSurface
    ctx.font = `600 46px ${font}`
    ctx.fillText(fitText(ctx, v, W - 192), 96, y + 34)
  })

  ctx.fillStyle = variant
  ctx.font = `500 30px ${font}`
  ctx.fillText('webx · streamx', 96, 1860)

  return new Promise((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('Could not render image'))), 'image/png'))
}

export async function downloadShareCard(snap: RecapSnapshot): Promise<void> {
  const blob = await renderShareCard(snap)
  const file = new File([blob], `webx-recap-${snap.period}.png`, { type: 'image/png' })
  if (navigator.canShare?.({ files: [file] })) {
    try {
      await navigator.share({ files: [file], title: `My ${snap.label} in WebX` })
      return
    } catch {
      /* fall through to download */
    }
  }
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = file.name
  a.click()
  URL.revokeObjectURL(a.href)
}
