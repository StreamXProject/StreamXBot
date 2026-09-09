import React from 'react'
import { Play, Pause, SkipBack, SkipForward, Shuffle, Repeat, Repeat1 } from 'lucide-react'
import { usePlayerStore } from '@/stores/playerStore'
import { useQueueStore } from '@/stores/queueStore'
import { IconButton } from '@/components/md3'
import { cn } from '@/lib/cn'

/** Shuffle / prev / play / next / repeat — shared by the bar and the full player */
export const PlaybackControls: React.FC<{ size?: 'sm' | 'lg'; className?: string }> = ({ size = 'sm', className }) => {
  const isPlaying = usePlayerStore((s) => s.isPlaying)
  const isBuffering = usePlayerStore((s) => s.isBuffering)
  const togglePlay = usePlayerStore((s) => s.togglePlay)
  const hasTrack = usePlayerStore((s) => Boolean(s.currentTrack))
  const isShuffle = useQueueStore((s) => s.isShuffle)
  const repeatMode = useQueueStore((s) => s.repeatMode)
  const nextTrack = useQueueStore((s) => s.nextTrack)
  const previousTrack = useQueueStore((s) => s.previousTrack)
  const toggleShuffle = useQueueStore((s) => s.toggleShuffle)
  const cycleRepeatMode = useQueueStore((s) => s.cycleRepeatMode)
  const lg = size === 'lg'

  return (
    <div className={cn('flex items-center justify-center', lg ? 'gap-3 sm:gap-5' : 'gap-1', className)}>
      <IconButton label="Shuffle" selected={isShuffle} size={lg ? 'lg' : 'sm'} onClick={toggleShuffle}>
        <Shuffle />
      </IconButton>
      <IconButton label="Previous" size={lg ? 'lg' : 'md'} onClick={() => void previousTrack()} disabled={!hasTrack} className="text-on-surface">
        <SkipBack className="fill-current" />
      </IconButton>
      <button
        onClick={togglePlay}
        disabled={!hasTrack}
        aria-label={isPlaying ? 'Pause' : 'Play'}
        className={cn(
          'state-layer relative inline-flex items-center justify-center bg-primary text-on-primary shadow-md3-1 transition-[border-radius,transform] duration-300 ease-emphasized active:scale-95 disabled:opacity-40',
          lg ? 'size-16 sm:size-[72px] [&_svg]:size-8' : 'size-10 [&_svg]:size-5',
          isPlaying ? 'rounded-lg' : 'rounded-full'
        )}
      >
        {isBuffering ? (
          <span className={cn('border-[3px] border-on-primary/30 border-t-on-primary rounded-full animate-spin', lg ? 'size-7' : 'size-4')} />
        ) : isPlaying ? (
          <Pause className="fill-current" />
        ) : (
          <Play className="fill-current ml-0.5" />
        )}
      </button>
      <IconButton label="Next" size={lg ? 'lg' : 'md'} onClick={() => void nextTrack()} disabled={!hasTrack} className="text-on-surface">
        <SkipForward className="fill-current" />
      </IconButton>
      <IconButton label={`Repeat: ${repeatMode}`} selected={repeatMode !== 'off'} size={lg ? 'lg' : 'sm'} onClick={cycleRepeatMode}>
        {repeatMode === 'one' ? <Repeat1 /> : <Repeat />}
      </IconButton>
    </div>
  )
}
