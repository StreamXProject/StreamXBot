import React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Zap, RotateCcw, Gauge, Mic2, ShieldAlert, Vibrate, Hand } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Slider } from '@/components/md3'
import { useSettingsStore } from '@/stores/settingsStore'
import { usePlayerStore } from '@/stores/playerStore'

export const Route = createFileRoute('/settings/playback')({
  component: PlaybackSettings,
})

function PlaybackSettings() {
  const s = useSettingsStore()
  const playbackRate = usePlayerStore((p) => p.playbackRate)
  const setPlaybackRate = usePlayerStore((p) => p.setPlaybackRate)

  return (
    <SettingsPage title="Playback" description="Streaming and playback behaviour">
      <SettingsSection title="Streaming">
        <SettingRow
          icon={<Zap />}
          label="Prefetch next track"
          description="Pre-buffer the next track"
          control={<Switch checked={s.prefetchNext} onChange={(v) => s.set('prefetchNext', v)} label="Prefetch next track" />}
        />
      </SettingsSection>

      <SettingsSection title="Behavior">
        <SettingRow
          icon={<RotateCcw />}
          label="Resume where you left off"
          description="Restore queue and position on launch"
          control={<Switch checked={s.resumeOnLaunch} onChange={(v) => s.set('resumeOnLaunch', v)} label="Resume on launch" />}
        />
        <SettingRow
          icon={<Gauge />}
          label="Playback speed"
          description={`${playbackRate.toFixed(2).replace(/\.?0+$/, '')}× · this session`}
          stacked
          control={
            <div className="flex items-center gap-3 w-full">
              <Slider value={playbackRate} min={0.5} max={2} step={0.05} onChange={setPlaybackRate} className="flex-1" />
              <button onClick={() => setPlaybackRate(1)} className="type-label-lg text-primary px-2">Reset</button>
            </div>
          }
        />
        <SettingRow icon={<Mic2 />} label="Lyrics button in player bar" control={<Switch checked={s.showLyricsButton} onChange={(v) => s.set('showLyricsButton', v)} label="Lyrics button" />} />
        <SettingRow
          icon={<ShieldAlert />}
          label="Confirm destructive actions"
          description="Confirm queue clears and playlist deletes"
          control={<Switch checked={s.confirmDestructive} onChange={(v) => s.set('confirmDestructive', v)} label="Confirm destructive actions" />}
        />
      </SettingsSection>
      <SettingsSection title="Touch">
        <SettingRow icon={<Hand />} label="Mini player gestures" description="Swipe to skip · swipe up to expand" control={<Switch checked={s.miniPlayerSwipe} onChange={(v) => s.set('miniPlayerSwipe', v)} label="Mini player gestures" />} />
        <SettingRow icon={<Vibrate />} label="Haptic feedback" description="On long-press, drag and swipe" control={<Switch checked={s.haptics} onChange={(v) => s.set('haptics', v)} label="Haptic feedback" />} />
        <SettingRow label="Long-press" description="Hold a track for its menu · hold ≡ in the queue to reorder" />
      </SettingsSection>
    </SettingsPage>
  )
}
