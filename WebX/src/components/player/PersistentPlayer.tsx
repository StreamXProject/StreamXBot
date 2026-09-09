import React, { useEffect, useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { Heart, Mic2, ListMusic, Maximize2, Play, Pause, SkipForward, Music2 } from 'lucide-react'
import { usePlayerStore } from '@/stores/playerStore'
import { useQueueStore } from '@/stores/queueStore'
import { useUiStore } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { audioEngine } from '@/audio/AudioEngine'
import { Scrubber } from './Scrubber'
import { PlaybackControls } from './PlaybackControls'
import { VolumeControl } from './VolumeControl'
import { Artwork } from '@/components/common/Artwork'
import { IconButton } from '@/components/md3'
import { cn } from '@/lib/cn'

/** Hairline progress for the compact mini-player, driven from the engine */
const MiniProgress: React.FC = () => {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => audioEngine.subscribeProgress((p) => {
    if (ref.current) ref.current.style.transform = `scaleX(${p.progressPercent / 100})`
  }), [])
  return (
    <div className="absolute top-0 inset-x-0 h-[3px] bg-on-surface/10 overflow-hidden">
      <div ref={ref} className="h-full bg-primary origin-left transition-transform duration-[250ms] ease-linear" style={{ transform: 'scaleX(0)' }} />
    </div>
  )
}

export const PersistentPlayer: React.FC = () => {
  const track = usePlayerStore((s) => s.currentTrack)
  const isPlaying = usePlayerStore((s) => s.isPlaying)
  const isBuffering = usePlayerStore((s) => s.isBuffering)
  const error = usePlayerStore((s) => s.error)
  const togglePlay = usePlayerStore((s) => s.togglePlay)
  const nextTrack = useQueueStore((s) => s.nextTrack)
  const openFullPlayer = useUiStore((s) => s.openFullPlayer)
  const toggleQueueDrawer = useUiStore((s) => s.toggleQueueDrawer)
  const toggleLyrics = useUiStore((s) => s.toggleLyrics)
  const queueDrawerOpen = useUiStore((s) => s.queueDrawerOpen)
  const fullPlayerOpen = useUiStore((s) => s.fullPlayerOpen)
  const fullPlayerPane = useUiStore((s) => s.fullPlayerPane)
  const isLiked = useLibraryStore((s) => (track ? s.likedIds.has(track.id) : false))
  const toggleLike = useLibraryStore((s) => s.toggleLike)
  const showLyricsButton = useSettingsStore((s) => s.showLyricsButton)
  const lyricsActive = fullPlayerOpen && fullPlayerPane === 'lyrics'

  if (!track) {
    return (
      <div className="h-[var(--webx-player-height)] shrink-0 elev-2 border-t border-outline-variant/40 px-4 md:px-6 flex items-center gap-4 text-on-surface-variant">
        <div className="size-12 rounded-sm bg-surface-highest flex items-center justify-center"><Music2 className="size-5 opacity-60" /></div>
        <div className="min-w-0">
          <p className="type-body-md text-on-surface">Nothing playing</p>
          <p className="type-body-sm">Pick a track, or press <kbd className="font-mono">Ctrl K</kbd> to search.</p>
        </div>
        <div className="hidden md:block flex-1" />
        <PlaybackControls className="hidden md:flex opacity-50" />
        <div className="hidden md:block w-1/4" />
      </div>
    )
  }

  return (
    <div className="relative h-[var(--webx-player-height)] shrink-0 elev-2 border-t border-outline-variant/40 select-none">
      {/* ---------- Compact (< md) ---------- */}
      <div className="md:hidden h-full flex items-center gap-3 px-3">
        <MiniProgress />
        <button onClick={() => openFullPlayer()} className="flex items-center gap-3 flex-1 min-w-0 text-left h-full">
          <Artwork src={track.cover_url} alt="" className="size-12 rounded-sm shadow-md3-1" />
          <div className="min-w-0">
            <p className="type-body-md font-semibold text-on-surface truncate">{track.title}</p>
            <p className={cn('type-body-sm truncate', error ? 'text-error' : 'text-on-surface-variant')}>{error || track.artist}</p>
          </div>
        </button>
        <IconButton label={isLiked ? 'Unlike' : 'Like'} selected={isLiked} onClick={() => void toggleLike(track)}>
          <Heart className={cn(isLiked && 'fill-current')} />
        </IconButton>
        <button onClick={togglePlay} aria-label={isPlaying ? 'Pause' : 'Play'} className={cn('state-layer size-11 bg-primary text-on-primary flex items-center justify-center transition-[border-radius] duration-300 ease-emphasized', isPlaying ? 'rounded-md' : 'rounded-full')}>
          {isBuffering ? <span className="size-4 border-2 border-on-primary/30 border-t-on-primary rounded-full animate-spin" /> : isPlaying ? <Pause className="size-5 fill-current" /> : <Play className="size-5 fill-current ml-0.5" />}
        </button>
        <IconButton label="Next" onClick={() => void nextTrack()}><SkipForward className="fill-current" /></IconButton>
      </div>

      {/* ---------- Expanded (≥ md) ---------- */}
      <div className="hidden md:grid h-full grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_minmax(0,1fr)] items-center gap-4 px-4 lg:px-6">
        {/* Track */}
        <div className="flex items-center gap-3 min-w-0">
          <button onClick={() => openFullPlayer()} className="group relative size-14 shrink-0 rounded-sm overflow-hidden shadow-md3-1" aria-label="Open full player">
            <Artwork src={track.cover_url} alt="" className="size-full rounded-none" />
            <span className="absolute inset-0 bg-scrim/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center text-white"><Maximize2 className="size-4" /></span>
          </button>
          <div className="min-w-0">
            <button onClick={() => openFullPlayer()} className="block type-body-md font-semibold text-on-surface truncate hover:underline text-left max-w-full">{track.title}</button>
            <p className={cn('type-body-sm truncate', error ? 'text-error' : 'text-on-surface-variant')}>
              {error ? error : track.artist_id ? <Link to="/artist/$artistId" params={{ artistId: track.artist_id }} className="hover:underline">{track.artist}</Link> : track.artist}
            </p>
          </div>
          <IconButton label={isLiked ? 'Remove from favourites' : 'Add to favourites'} size="md" selected={isLiked} onClick={() => void toggleLike(track)} className="ml-1">
            <Heart className={cn(isLiked && 'fill-current')} />
          </IconButton>
        </div>

        {/* Controls */}
        <div className="flex flex-col items-center justify-center min-w-0 max-w-[520px] w-full mx-auto">
          <PlaybackControls />
          <Scrubber size="sm" className="mt-0.5" />
        </div>

        {/* Right */}
        <div className="flex items-center justify-end gap-1 lg:gap-1.5 min-w-0">
          {showLyricsButton && (
            <IconButton label="Lyrics" size="md" selected={lyricsActive} onClick={toggleLyrics}><Mic2 /></IconButton>
          )}
          <IconButton label="Queue" size="md" selected={queueDrawerOpen || (fullPlayerOpen && fullPlayerPane === 'queue')} onClick={toggleQueueDrawer}><ListMusic /></IconButton>
          <VolumeControl className="hidden lg:flex" />
          <IconButton label="Full screen player" size="md" onClick={() => openFullPlayer()}><Maximize2 /></IconButton>
        </div>
      </div>
    </div>
  )
}
