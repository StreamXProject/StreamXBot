import React, { useMemo, useState } from 'react'
import { Palette } from 'lucide-react'
import { Dialog, Button, TextField, Chip, Slider, Switch, SegmentedButton } from '@/components/md3'
import { ThemeDefinitionSchema, type ThemeDefinition, type SchemeVariant } from '@/theme/tokens'
import { previewColors, useThemeStore } from '@/theme/themeStore'
import { isValidHex } from '@/theme/scheme'
import { cn } from '@/lib/cn'

const VARIANTS: Array<{ id: SchemeVariant; label: string }> = [
  { id: 'tonalSpot', label: 'Tonal' },
  { id: 'vibrant', label: 'Vibrant' },
  { id: 'expressive', label: 'Expressive' },
  { id: 'fidelity', label: 'Fidelity' },
  { id: 'content', label: 'Content' },
  { id: 'neutral', label: 'Neutral' },
  { id: 'monochrome', label: 'Mono' },
  { id: 'rainbow', label: 'Rainbow' },
  { id: 'fruitSalad', label: 'Fruit salad' },
]

const FONTS: Array<{ label: string; body: string; google: string[] }> = [
  { label: 'Manrope', body: "'Manrope', system-ui, sans-serif", google: ['Manrope:wght@300..800'] },
  { label: 'Roboto Flex', body: "'Roboto Flex', Roboto, system-ui, sans-serif", google: ['Roboto+Flex:wght@300..800'] },
  { label: 'Inter', body: "'Inter', system-ui, sans-serif", google: ['Inter:wght@300..800'] },
  { label: 'Plus Jakarta Sans', body: "'Plus Jakarta Sans', system-ui, sans-serif", google: ['Plus+Jakarta+Sans:wght@300..800'] },
  { label: 'Space Grotesk', body: "'Space Grotesk', system-ui, sans-serif", google: ['Space+Grotesk:wght@300..700'] },
  { label: 'Sora', body: "'Sora', system-ui, sans-serif", google: ['Sora:wght@300..800'] },
  { label: 'System', body: 'system-ui, -apple-system, Segoe UI, Roboto, sans-serif', google: [] },
]

const SWATCHES = ['#FA2D48', '#6750A4', '#0A84FF', '#4C8C4A', '#FFB300', '#E91E63', '#00BFA5', '#FF6D00', '#9E9E9E', '#3F51B5']

export const ThemeEditorDialog: React.FC<{
  open: boolean
  onClose: () => void
  /** Existing theme to edit; omit to create */
  base?: ThemeDefinition | null
}> = ({ open, onClose, base }) => {
  const addUserTheme = useThemeStore((s) => s.addUserTheme)
  const updateUserTheme = useThemeStore((s) => s.updateUserTheme)
  const setTheme = useThemeStore((s) => s.setTheme)
  const mode = useThemeStore((s) => s.resolvedMode)
  const editing = Boolean(base && !base.builtIn)

  const [name, setName] = useState(base?.name ? (base.builtIn ? `${base.name} remix` : base.name) : 'My theme')
  const [seed, setSeed] = useState(base?.seed ?? '#FA2D48')
  const [variant, setVariant] = useState<SchemeVariant>(base?.variant ?? 'tonalSpot')
  const [contrast, setContrast] = useState(base?.contrast ?? 0)
  const [shape, setShape] = useState(base?.shape.scale ?? 1)
  const [font, setFont] = useState(FONTS.findIndex((f) => f.body === base?.typography.body) >= 0 ? FONTS.findIndex((f) => f.body === base?.typography.body) : 0)
  const [glass, setGlass] = useState(base?.effects.glass ?? true)
  const [pureBlack, setPureBlack] = useState(base?.effects.pureBlack ?? false)
  const [ambient, setAmbient] = useState(base?.effects.ambientBackdrop ?? true)
  const [preferredMode, setPreferredMode] = useState<'light' | 'dark' | 'system'>(base?.preferredMode ?? 'dark')
  const [hexInput, setHexInput] = useState(seed)

  const draft = useMemo(() => {
    const id = editing ? base!.id : `custom-${name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'theme'}-${Date.now().toString(36).slice(-4)}`
    const r = ThemeDefinitionSchema.safeParse({
      id,
      name: name.trim() || 'My theme',
      description: 'Custom theme',
      seed: isValidHex(seed) ? seed : '#FA2D48',
      variant,
      contrast,
      preferredMode,
      typography: { body: FONTS[font].body, googleFonts: FONTS[font].google },
      shape: { scale: shape },
      effects: { glass, pureBlack, ambientBackdrop: ambient, tonalElevation: true },
      builtIn: false,
    })
    return r.success ? r.data : null
  }, [name, seed, variant, contrast, shape, font, glass, pureBlack, ambient, preferredMode, editing, base])

  const preview = draft ? previewColors(draft, mode) : null

  const save = () => {
    if (!draft) return
    if (editing) updateUserTheme(draft)
    else addUserTheme(draft)
    setTheme(draft.id)
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={editing ? 'Edit theme' : 'Create theme'}
      icon={<Palette />}
      size="xl"
      fullscreenOnMobile
      actions={
        <>
          <Button variant="text" onClick={onClose}>Cancel</Button>
          <Button onClick={save} disabled={!draft}>{editing ? 'Save' : 'Create & apply'}</Button>
        </>
      }
    >
      <div className="grid md:grid-cols-[minmax(0,1fr)_240px] gap-8">
        <div className="space-y-6">
          <TextField label="Theme name" value={name} onChange={(e) => setName(e.target.value)} />

          <div>
            <p className="type-label-lg text-on-surface mb-2">Seed color</p>
            <div className="flex flex-wrap items-center gap-2">
              {SWATCHES.map((h) => (
                <button
                  key={h}
                  aria-label={h}
                  onClick={() => { setSeed(h); setHexInput(h) }}
                  className={cn('size-8 rounded-full ring-2 ring-offset-2 ring-offset-surface transition-transform', seed.toUpperCase() === h ? 'ring-primary scale-110' : 'ring-transparent hover:scale-105')}
                  style={{ background: h }}
                />
              ))}
              <label className="relative size-8 rounded-full ring-2 ring-outline-variant overflow-hidden cursor-pointer" title="Custom color" style={{ background: 'conic-gradient(red, yellow, lime, aqua, blue, magenta, red)' }}>
                <input type="color" value={isValidHex(seed) ? seed : '#FA2D48'} onChange={(e) => { setSeed(e.target.value.toUpperCase()); setHexInput(e.target.value.toUpperCase()) }} className="absolute inset-0 opacity-0 cursor-pointer" />
              </label>
              <TextField
                value={hexInput}
                onChange={(e) => {
                  const v = e.target.value.toUpperCase()
                  setHexInput(v)
                  if (isValidHex(v)) setSeed(v)
                }}
                placeholder="#RRGGBB"
                containerClassName="w-32"
                className="font-mono"
                error={hexInput && !isValidHex(hexInput) ? 'Invalid hex' : undefined}
              />
            </div>
          </div>

          <div>
            <p className="type-label-lg text-on-surface mb-2">Palette style</p>
            <div className="flex flex-wrap gap-2">
              {VARIANTS.map((v) => <Chip key={v.id} variant="filter" label={v.label} selected={variant === v.id} onClick={() => setVariant(v.id)} />)}
            </div>
          </div>

          <div className="grid sm:grid-cols-2 gap-6">
            <div className="min-w-0">
              <div className="flex justify-between gap-3 type-label-lg text-on-surface mb-1"><span>Contrast</span><span className="text-on-surface-variant">{contrast === 0 ? 'Standard' : contrast > 0 ? `+${contrast}` : contrast}</span></div>
              <Slider value={contrast} min={-1} max={1} step={0.25} onChange={setContrast} />
            </div>
            <div className="min-w-0">
              <div className="flex justify-between gap-3 type-label-lg text-on-surface mb-1"><span>Roundness</span><span className="text-on-surface-variant">{shape === 0 ? 'Sharp' : shape === 1 ? 'Default' : `${shape}×`}</span></div>
              <Slider value={shape} min={0} max={2} step={0.25} onChange={setShape} />
            </div>
          </div>

          <div>
            <p className="type-label-lg text-on-surface mb-2">Typeface</p>
            <div className="flex flex-wrap gap-2">
              {FONTS.map((f, i) => <Chip key={f.label} variant="filter" label={f.label} selected={font === i} onClick={() => setFont(i)} />)}
            </div>
          </div>

          <div>
            <p className="type-label-lg text-on-surface mb-2">Designed for</p>
            <SegmentedButton size="sm" value={preferredMode} onChange={setPreferredMode} options={[{ value: 'dark', label: 'Dark' }, { value: 'light', label: 'Light' }, { value: 'system', label: 'Both' }]} />
          </div>

          <div className="space-y-3">
            {[
              ['Frosted glass surfaces', 'Backdrop blur on bars and sheets', glass, setGlass],
              ['Pure black (OLED)', 'Use #000 surfaces in dark mode', pureBlack, setPureBlack],
              ['Ambient artwork backdrop', 'Blurred cover behind the full player', ambient, setAmbient],
            ].map(([l, d, v, set]) => (
              <div key={l as string} className="flex items-center justify-between gap-4">
                <div><p className="type-body-lg text-on-surface">{l as string}</p><p className="type-body-sm">{d as string}</p></div>
                <Switch checked={v as boolean} onChange={set as (b: boolean) => void} label={l as string} />
              </div>
            ))}
          </div>
        </div>

        <div className="md:sticky md:top-0 self-start">
          <p className="type-label-lg text-on-surface mb-2">Preview · {mode}</p>
          {preview && (
            <div className="rounded-lg overflow-hidden ring-1 ring-outline-variant" style={{ background: preview.surface, color: preview.onSurface, fontFamily: FONTS[font].body }}>
              <div className="p-3 space-y-3">
                <div className="flex items-center gap-2">
                  <span className="size-9 shrink-0" style={{ background: preview.primaryContainer, borderRadius: 12 * shape }} />
                  <div className="min-w-0"><p className="text-sm font-semibold truncate">Midnight Reverie</p><p className="text-xs truncate" style={{ color: preview.onSurfaceVariant }}>Luna Horizon</p></div>
                </div>
                <div className="h-1.5 rounded-full" style={{ background: preview.surfaceContainerHighest }}><div className="h-full w-2/5 rounded-full" style={{ background: preview.primary }} /></div>
                <div className="flex items-center justify-center gap-3">
                  <span className="size-7 rounded-full" style={{ background: preview.surfaceContainerHigh }} />
                  <span className="size-10 rounded-full" style={{ background: preview.primary }} />
                  <span className="size-7 rounded-full" style={{ background: preview.surfaceContainerHigh }} />
                </div>
                <div className="grid grid-cols-2 gap-2 text-[11px] font-semibold">
                  <span className="h-8 flex items-center justify-center rounded-full" style={{ background: preview.primary, color: preview.onPrimary }}>Filled</span>
                  <span className="h-8 flex items-center justify-center rounded-full" style={{ background: preview.secondaryContainer, color: preview.onSecondaryContainer }}>Tonal</span>
                  <span className="h-8 flex items-center justify-center rounded-full" style={{ background: preview.tertiaryContainer, color: preview.onTertiaryContainer }}>Tertiary</span>
                  <span className="h-8 flex items-center justify-center rounded-full border" style={{ borderColor: preview.outline, color: preview.primary }}>Outlined</span>
                </div>
                <div className="p-2 text-xs" style={{ background: preview.surfaceContainerLow, borderRadius: 12 * shape, color: preview.onSurfaceVariant }}>
                  Surfaces are tinted from the seed. Text stays legible at every contrast level.
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </Dialog>
  )
}
