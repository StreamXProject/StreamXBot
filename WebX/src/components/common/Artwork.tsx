import React, { useState } from 'react'
import { Music2, Disc3, User } from 'lucide-react'
import { cn } from '@/lib/cn'

interface ArtworkProps extends Omit<React.ImgHTMLAttributes<HTMLImageElement>, 'src'> {
  src?: string | null
  alt: string
  className?: string
  /** Shape & fallback icon */
  kind?: 'track' | 'album' | 'artist' | 'playlist'
  /** Optional 2x2 collage when a list of urls is provided */
  collage?: string[]
  priority?: boolean
  rounded?: string
  style?: React.CSSProperties
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
          <img key={i} src={u} alt="" loading="lazy" decoding="async" className="w-full h-full object-cover" />
        ))}
      </div>
    )
  }

  const show = src && !error
  return (
    <div className={cn('relative overflow-hidden bg-surface-highest text-on-surface-variant/50', shape, className)} style={style}>
      {!show || !loaded ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <Icon className="w-[38%] h-[38%]" strokeWidth={1.25} />
        </div>
      ) : null}
      {show && (
        <img
          src={src}
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

/** Backwards-compatible alias */
export const OptimizedImage = Artwork
