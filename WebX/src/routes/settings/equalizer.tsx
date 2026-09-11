import React, { useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { AudioLines, RotateCcw } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Chip, Slider } from '@/components/md3'
import { useSettingsStore } from '@/stores/settingsStore'
import { equalizer, EQ_FREQUENCIES, EQ_PRESETS, EQ_MIN_DB, EQ_MAX_DB } from '@/audio/Equalizer'
import { toast } from '@/stores/uiStore'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/settings/equalizer')({
  component: EqualizerSettings,
})

const fmtHz = (hz: number) => (hz >= 1000 ? `${hz / 1000}k` : String(hz))
const fmtDb = (db: number) => `${db > 0 ? '+' : ''}${db.toFixed(db % 1 === 0 ? 0 : 1)}`

function EqualizerSettings() {
  const s = useSettingsStore()
  const set = s.set
  const gains = s.eqGains.length === EQ_FREQUENCIES.length ? s.eqGains : EQ_PRESETS[0]!.gains
  const supported = equalizer.isSupported

  // keep the audio graph in sync with persisted settings while this page is open
  useEffect(() => {
    equalizer.setGains(gains)
    equalizer.setPreamp(s.eqPreamp)
  }, [gains, s.eqPreamp])

  const toggle = (on: boolean) => {
    const ok = equalizer.setEnabled(on)
    set('eqEnabled', ok)
    if (on && !ok) toast('Equalizer is not available in this browser', { variant: 'error' })
  }
  const setBand = (i: number, v: number) => {
    const next = gains.slice()
    next[i] = v
    set('eqGains', next)
    set('eqPreset', 'custom')
  }
  const applyPreset = (id: string) => {
    const p = EQ_PRESETS.find((x) => x.id === id)
    if (!p) return
    set('eqGains', p.gains.slice())
    set('eqPreset', id)
  }

  return (
    <SettingsPage title="Equalizer" description="10-band EQ · this device only">
      <SettingsSection>
        <SettingRow
          icon={<AudioLines />}
          label="Enable equalizer"
          description={supported ? (s.eqEnabled ? `${EQ_PRESETS.find((p) => p.id === s.eqPreset)?.name ?? 'Custom'} curve active` : 'Off — audio passes through untouched') : 'Web Audio is not available in this browser'}
          control={<Switch checked={s.eqEnabled} disabled={!supported} onChange={toggle} label="Enable equalizer" />}
        />
      </SettingsSection>

      <SettingsSection title="Presets">
        <div className="flex flex-wrap gap-2 p-4">
          {EQ_PRESETS.map((p) => (
            <Chip key={p.id} variant="filter" label={p.name} selected={s.eqPreset === p.id} onClick={() => applyPreset(p.id)} />
          ))}
          {s.eqPreset === 'custom' && <Chip variant="filter" label="Custom" selected />}
        </div>
      </SettingsSection>

      <SettingsSection title="Bands" description="Drag to adjust · double-click value to reset">
        <div className={cn('p-4 sm:p-6 transition-opacity', !s.eqEnabled && 'opacity-50')}>
          <div className="eq-grid grid gap-1.5 sm:gap-3" style={{ gridTemplateColumns: `repeat(${EQ_FREQUENCIES.length}, minmax(0, 1fr))` }}>
            {EQ_FREQUENCIES.map((hz, i) => {
              const v = gains[i] ?? 0
              return (
                <div key={hz} className="flex flex-col items-center gap-2 min-w-0">
                  <button type="button" onDoubleClick={() => setBand(i, 0)} className={cn('type-label-md tabular', v === 0 ? 'text-on-surface-variant' : 'text-primary')} title="Double-click to reset">
                    {fmtDb(v)}
                  </button>
                  <div className="relative h-44 sm:h-56 w-full flex items-center justify-center">
                    <span className="absolute left-1/2 top-1/2 -translate-x-1/2 h-px w-3 bg-outline-variant" aria-hidden />
                    <input
                      type="range"
                      className="eq-slider"
                      min={EQ_MIN_DB}
                      max={EQ_MAX_DB}
                      step={0.5}
                      value={v}
                      aria-label={`${fmtHz(hz)} Hz`}
                      onChange={(e) => setBand(i, Number(e.target.value))}
                    />
                  </div>
                  <span className="type-label-sm text-on-surface-variant">{fmtHz(hz)}</span>
                </div>
              )
            })}
          </div>
        </div>
        <SettingRow
          label="Preamp"
          description={`${fmtDb(s.eqPreamp)} dB · lower if boosted bands clip`}
          stacked
          control={
            <div className="flex items-center gap-3 w-full">
              <Slider value={s.eqPreamp} min={EQ_MIN_DB} max={EQ_MAX_DB} step={0.5} onChange={(v) => set('eqPreamp', v)} aria-label="Preamp" className="flex-1" />
              <button onClick={() => set('eqPreamp', 0)} className="type-label-lg text-primary px-2 inline-flex items-center gap-1"><RotateCcw className="size-4" /> Reset</button>
            </div>
          }
        />
      </SettingsSection>
    </SettingsPage>
  )
}
