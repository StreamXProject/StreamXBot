import React from 'react'
import { Play, Shuffle, Heart, MoreHorizontal, Pause } from 'lucide-react'
import { Artwork } from './Artwork'
import { Button, IconButton } from '@/components/md3'
import { cn } from '@/lib/cn'

export interface CollectionHeaderProps {
  kind: 'album' | 'artist' | 'playlist' | 'mix'
  eyebrow?: string
  title: string
  subtitle?: React.ReactNode
  meta?: React.ReactNode
  imageUrl?: string | null
  collage?: string[]
  onPlay?: () => void
  onShuffle?: () => void
  isPlaying?: boolean
  liked?: boolean
  onLike?: () => void
  onMore?: (anchor: HTMLElement) => void
  children?: React.ReactNode
}

/** Hero for album / artist / playlist / mix pages */
export const CollectionHeader: React.FC<CollectionHeaderProps> = ({ kind, eyebrow, title, subtitle, meta, imageUrl, collage, onPlay, onShuffle, isPlaying, liked, onLike, onMore, children }) => {
  const isArtist = kind === 'artist'
  return (
    <header className="relative">
      <div className="absolute inset-x-0 top-0 h-64 -z-10 bg-gradient-to-b from-primary-container/20 to-transparent pointer-events-none" />
      <div className="flex flex-col sm:flex-row sm:items-end gap-6 pt-2 pb-6">
        <Artwork
          src={imageUrl}
          collage={collage}
          alt={title}
          kind={isArtist ? 'artist' : kind === 'album' ? 'album' : 'playlist'}
          priority
          className={cn('shrink-0 shadow-md3-3 mx-auto sm:mx-0', isArtist ? 'size-44 sm:size-52 rounded-full' : 'size-44 sm:size-52 rounded-lg')}
        />
        <div className="flex-1 min-w-0 text-center sm:text-left">
          {eyebrow && <p className="type-label-lg text-primary">{eyebrow}</p>}
          <h1 className="type-headline-md sm:type-headline-lg lg:type-display-sm text-on-surface mt-1 break-words [text-wrap:balance]">{title}</h1>
          {subtitle && <p className="type-body-lg text-on-surface mt-2">{subtitle}</p>}
          {meta && <p className="type-body-md text-on-surface-variant mt-1">{meta}</p>}
          <div className="mt-5 flex flex-wrap items-center justify-center sm:justify-start gap-2">
            {onPlay && (
              <Button size="lg" icon={isPlaying ? <Pause className="fill-current" /> : <Play className="fill-current" />} onClick={onPlay}>
                {isPlaying ? 'Pause' : 'Play'}
              </Button>
            )}
            {onShuffle && <Button size="lg" variant="tonal" icon={<Shuffle />} onClick={onShuffle}>Shuffle</Button>}
            {onLike && (
              <IconButton label={liked ? 'Remove from library' : 'Save to library'} size="lg" variant="outlined" selected={liked} onClick={onLike}>
                <Heart className={cn(liked && 'fill-current')} />
              </IconButton>
            )}
            {onMore && (
              <IconButton label="More" size="lg" variant="outlined" onClick={(e) => onMore(e.currentTarget)}>
                <MoreHorizontal />
              </IconButton>
            )}
            {children}
          </div>
        </div>
      </div>
    </header>
  )
}
