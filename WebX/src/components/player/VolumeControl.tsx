import React from 'react'
import { Volume, Volume1, Volume2, VolumeX } from 'lucide-react'
import { usePlayerStore } from '@/stores/playerStore'
import { Slider, IconButton } from '@/components/md3'
import { cn } from '@/lib/cn'

export const VolumeControl: React.FC<{ className?: string; sliderClassName?: string }> = ({ className, sliderClassName }) => {
  const volume = usePlayerStore((s) => s.volume)
  const isMuted = usePlayerStore((s) => s.isMuted)
  const setVolume = usePlayerStore((s) => s.setVolume)
  const toggleMute = usePlayerStore((s) => s.toggleMute)
  const adjustVolume = usePlayerStore((s) => s.adjustVolume)
  const v = isMuted ? 0 : volume
  const Icon = v === 0 ? VolumeX : v < 0.34 ? Volume : v < 0.67 ? Volume1 : Volume2
  return (
    <div className={cn('flex items-center gap-1', className)} onWheel={(e) => { e.preventDefault(); adjustVolume(e.deltaY < 0 ? 0.05 : -0.05) }}>
      <IconButton label={isMuted ? 'Unmute' : 'Mute'} size="sm" onClick={toggleMute}>
        <Icon />
      </IconButton>
      <Slider value={Math.round(v * 100)} min={0} max={100} onChange={(n) => setVolume(n / 100)} aria-label="Volume" className={cn('w-28', sliderClassName)} />
    </div>
  )
}
