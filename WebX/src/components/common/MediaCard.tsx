import React from 'react'
import { Play } from 'lucide-react'
import { Artwork } from './Artwork'
import { cn } from '@/lib/cn'

interface MediaCardProps {
  title: string
  subtitle?: string
  imageUrl?: string | null
  collage?: string[]
  kind?: 'album' | 'artist' | 'playlist' | 'mix'
  onClick?: () => void
  onPlay?: () => void | Promise<void>
  loading?: boolean
  className?: string
  size?: 'sm' | 'md'
}

export const MediaCard: React.FC<MediaCardProps> = ({ title, subtitle, imageUrl, collage, kind = 'album', onClick, onPlay, loading, className, size = 'md' }) => {
  const isArtist = kind === 'artist'
  return (
    <div
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onClick={onClick}
      onKeyDown={(e) => e.key === 'Enter' && onClick?.()}
      className={cn(
        'group relative flex flex-col gap-2.5 p-2 -m-2 rounded-lg cursor-pointer select-none outline-none',
        'transition-colors duration-200 hover:bg-on-surface/[0.04] focus-visible:bg-on-surface/[0.06]',
        size === 'sm' && 'w-36 shrink-0',
        className
      )}
    >
      <div className="relative">
        <Artwork
          src={imageUrl}
          collage={collage}
          alt={title}
          kind={isArtist ? 'artist' : kind === 'mix' || kind === 'playlist' ? 'playlist' : 'album'}
          className={cn('w-full aspect-square shadow-md3-1', isArtist ? 'rounded-full' : 'rounded-md')}
        />
        {onPlay && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              void onPlay()
            }}
            aria-label={`Play ${title}`}
            className={cn(
              'absolute bottom-2 right-2 size-11 rounded-full bg-primary text-on-primary shadow-md3-2 flex items-center justify-center',
              'opacity-0 translate-y-1 group-hover:opacity-100 group-hover:translate-y-0 focus-visible:opacity-100 max-md:opacity-100 max-md:translate-y-0',
              'transition-[opacity,transform] duration-200 ease-emphasized active:scale-95'
            )}
          >
            {loading ? <span className="size-4 border-2 border-on-primary/40 border-t-on-primary rounded-full animate-spin" /> : <Play className="size-5 fill-current ml-0.5" />}
          </button>
        )}
      </div>
      <div className={cn('min-w-0 px-0.5', isArtist && 'text-center')}>
        <h3 className="type-title-sm text-on-surface truncate">{title}</h3>
        {subtitle && <p className="type-body-sm text-on-surface-variant truncate mt-0.5">{subtitle}</p>}
      </div>
    </div>
  )
}
