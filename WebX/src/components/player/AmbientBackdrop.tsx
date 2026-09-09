import React, { useEffect, useState } from 'react'
import { cn } from '@/lib/cn'

/**
 * Cheap ambient backdrop: a *tiny* image (128px) blurred once and scaled up on
 * the GPU. The blur is computed on a 128×128 layer instead of a full-viewport
 * layer, which is what made the previous full-player open stutter. Two layers
 * crossfade when the artwork changes.
 */
export const AmbientBackdrop: React.FC<{ src?: string | null; className?: string; enabled?: boolean }> = ({ src, className, enabled = true }) => {
  const [layers, setLayers] = useState<[string | null, string | null]>([src ?? null, null])
  const [front, setFront] = useState<0 | 1>(0)

  useEffect(() => {
    if (!enabled) return
    const current = layers[front]
    if ((src ?? null) === current) return
    // preload then swap
    if (!src) {
      setLayers((l) => (front === 0 ? [l[0], null] : [null, l[1]]))
      setFront((f) => (f === 0 ? 1 : 0))
      return
    }
    const img = new Image()
    img.decoding = 'async'
    img.onload = () => {
      const next: 0 | 1 = front === 0 ? 1 : 0
      setLayers((l) => (next === 0 ? [src, l[1]] : [l[0], src]))
      setFront(next)
    }
    img.src = src
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [src, enabled])

  return (
    <div className={cn('absolute inset-0 overflow-hidden pointer-events-none', className)} aria-hidden style={{ contain: 'strict' }}>
      {enabled &&
        layers.map((url, i) =>
          url ? (
            <img
              key={i}
              src={url}
              alt=""
              className={cn(
                'absolute left-1/2 top-1/2 w-32 h-32 object-cover transition-opacity duration-700 ease-linear',
                i === front ? 'opacity-100' : 'opacity-0'
              )}
              style={{ transform: 'translate(-50%, -50%) scale(18)', filter: 'blur(6px) saturate(1.3)', willChange: 'opacity' }}
            />
          ) : null
        )}
      {/* tint + vignette in theme colors */}
      <div className="absolute inset-0" style={{ background: 'linear-gradient(to bottom, color-mix(in srgb, var(--md-sys-color-surface) 55%, transparent), color-mix(in srgb, var(--md-sys-color-surface) 78%, transparent) 55%, var(--md-sys-color-surface))' }} />
      <div className="absolute inset-0 bg-primary-container/10 mix-blend-overlay" />
    </div>
  )
}
