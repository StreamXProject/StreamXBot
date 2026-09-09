import React, { useEffect, useState } from 'react'
import { Moon } from 'lucide-react'
import { Dialog, Button, Chip, Switch } from '@/components/md3'
import { useUiStore } from '@/stores/uiStore'
import { usePlayerStore } from '@/stores/playerStore'

const PRESETS = [5, 10, 15, 30, 45, 60, 90]

export const SleepTimerDialog: React.FC = () => {
  const open = useUiStore((s) => s.sleepTimerOpen)
  const setOpen = useUiStore((s) => s.setSleepTimerOpen)
  const sleepAt = usePlayerStore((s) => s.sleepAt)
  const sleepAfterTrack = usePlayerStore((s) => s.sleepAfterTrack)
  const setSleepTimer = usePlayerStore((s) => s.setSleepTimer)
  const setSleepAfterTrack = usePlayerStore((s) => s.setSleepAfterTrack)
  const [, tick] = useState(0)

  useEffect(() => {
    if (!open || !sleepAt) return
    const id = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [open, sleepAt])

  const remaining = sleepAt ? Math.max(0, sleepAt - Date.now()) : 0
  const mm = Math.floor(remaining / 60000)
  const ss = Math.floor((remaining % 60000) / 1000)

  return (
    <Dialog
      open={open}
      onClose={() => setOpen(false)}
      title="Sleep timer"
      icon={<Moon />}
      actions={
        <>
          {(sleepAt || sleepAfterTrack) && (
            <Button variant="text" onClick={() => { setSleepTimer(null); setSleepAfterTrack(false) }}>Turn off</Button>
          )}
          <Button variant="text" onClick={() => setOpen(false)}>Done</Button>
        </>
      }
    >
      <div className="space-y-5">
        {sleepAt ? (
          <p className="text-center type-display-sm tabular text-on-surface">{mm}:{ss.toString().padStart(2, '0')}</p>
        ) : (
          <p className="text-center">Playback will pause automatically.</p>
        )}
        <div className="flex flex-wrap gap-2 justify-center">
          {PRESETS.map((m) => (
            <Chip key={m} variant="filter" label={`${m} min`} selected={Boolean(sleepAt) && Math.abs(remaining / 60000 - m) < 1.05} onClick={() => setSleepTimer(m)} />
          ))}
        </div>
        <div className="flex items-center justify-between pt-2 border-t border-outline-variant">
          <div>
            <p className="type-body-lg text-on-surface">Stop after current track</p>
            <p className="type-body-sm">Finish this song, then pause</p>
          </div>
          <Switch checked={sleepAfterTrack} onChange={setSleepAfterTrack} label="Stop after current track" />
        </div>
      </div>
    </Dialog>
  )
}
