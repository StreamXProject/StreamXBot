import React, { useState } from 'react'
import { Music2, Disc3, User } from 'lucide-react'
import { cn } from '@/lib/cn'
import { getBaseUrl } from '@/api/client'

interface ArtworkProps extends Omit<React.ImgHTMLAttributes<HTMLImageElement>, 'src'> {
  src?: string | null
  alt: string
  className?: string
  kind?: 'track' | 'album' | 'artist' | 'playlist'
  collage?: string[]
  priority?: boolean
  rounded?: string
  style?: React.CSSProperties
}

function resolveMediaUrl(url?: string | null): string | undefined {
  if (!url) return undefined
  if (url.startsWith('/') && !url.startsWith('//')) {
    const base = getBaseUrl()
    return base ? `${base}${url}` : url
  }
  return url
}

/**
 * Artwork with a themed placeholder (no external fallback image), lazy
 * decoding and a soft fade-in. Renders a 2×2 collage when `collage` has ≥4.
 */
export const Artwork: React.FC<ArtworkProps> = React.memo(({ src, alt, className, kind = 'track', collage, priority, rounded, style, ...rest }) => {
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState(false)
  const Icon = kind === 'artist' ? User : kind === 'album' ? Disc3 : Music2
  const shape = rounded ?? (kind === 'artist' ? 'rounded-full' : 'rounded-md')

  if (collage && collage.length >= 4) {
    return (
      <div className={cn('relative overflow-hidden bg-surface-highest grid grid-cols-2 grid-rows-2', shape, className)} style={style}>
        {collage.slice(0, 4).map((u, i) => (
          <img key={i} src={resolveMediaUrl(u)} alt="" loading="lazy" decoding="async" className="w-full h-full object-cover" />
        ))}
      </div>
    )
  }

  const resolvedSrc = resolveMediaUrl(src)
  const show = resolvedSrc && !error
  return (
    <div className={cn('relative overflow-hidden bg-surface-highest text-on-surface-variant/50', shape, className)} style={style}>
      {!show || !loaded ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <Icon className="w-[38%] h-[38%]" strokeWidth={1.25} />
        </div>
      ) : null}
      {show && (
        <img
          src={resolvedSrc}
          alt={alt}
          loading={priority ? 'eager' : 'lazy'}
          decoding="async"
          fetchPriority={priority ? 'high' : 'auto'}
          onLoad={() => setLoaded(true)}
          onError={() => setError(true)}
          className={cn('absolute inset-0 w-full h-full object-cover transition-opacity duration-300', loaded ? 'opacity-100' : 'opacity-0')}
          {...rest}
        />
      )}
    </div>
  )
})
Artwork.displayName = 'Artwork'

export const OptimizedImage = Artwork
