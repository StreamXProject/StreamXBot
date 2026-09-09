import React, { useRef, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Sun, Moon, MonitorSmartphone, Plus, Upload, Sparkles, Pencil, Copy, Download, Trash2 } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { ThemeCard } from '@/components/settings/ThemeCard'
import { ThemeEditorDialog } from '@/components/settings/ThemeEditorDialog'
import { SegmentedButton, Switch, Button, Menu, Dialog, TextField } from '@/components/md3'
import { useThemeStore, type MotionPref } from '@/theme/themeStore'
import type { ThemeDefinition, ThemeMode } from '@/theme/tokens'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { usePlayerStore } from '@/stores/playerStore'

export const Route = createFileRoute('/settings/appearance')({
  component: AppearanceSettings,
})

function AppearanceSettings() {
  const themes = useThemeStore((s) => s.themes)
  const activeId = useThemeStore((s) => s.activeThemeId)
  const setTheme = useThemeStore((s) => s.setTheme)
  const mode = useThemeStore((s) => s.mode)
  const setMode = useThemeStore((s) => s.setMode)
  const resolvedMode = useThemeStore((s) => s.resolvedMode)
  const dynamicColor = useThemeStore((s) => s.dynamicColor)
  const setDynamicColor = useThemeStore((s) => s.setDynamicColor)
  const setDynamicSeedFromImage = useThemeStore((s) => s.setDynamicSeedFromImage)
  const motion = useThemeStore((s) => s.motion)
  const setMotion = useThemeStore((s) => s.setMotion)
  const duplicateTheme = useThemeStore((s) => s.duplicateTheme)
  const removeUserTheme = useThemeStore((s) => s.removeUserTheme)
  const importThemeJson = useThemeStore((s) => s.importThemeJson)
  const exportThemeJson = useThemeStore((s) => s.exportThemeJson)
  const compactRows = useSettingsStore((s) => s.compactRows)
  const showQualityBadges = useSettingsStore((s) => s.showQualityBadges)
  const setSetting = useSettingsStore((s) => s.set)

  const [editor, setEditor] = useState<{ open: boolean; base: ThemeDefinition | null }>({ open: false, base: null })
  const [menu, setMenu] = useState<{ anchor: HTMLElement; theme: ThemeDefinition } | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')
  const [importError, setImportError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const builtIns = themes.filter((t) => t.builtIn)
  const custom = themes.filter((t) => !t.builtIn)

  const doImport = (text: string) => {
    try {
      const t = importThemeJson(text)
      setTheme(t.id)
      toast(`Imported “${t.name}”`)
      setImportOpen(false)
      setImportText('')
      setImportError(null)
    } catch (err) {
      setImportError((err as Error).message)
    }
  }

  const exportTheme = (t: ThemeDefinition) => {
    const json = exportThemeJson(t.id)
    const blob = new Blob([json], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${t.id}.webx-theme.json`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const toggleDynamic = (on: boolean) => {
    setDynamicColor(on)
    if (on) void setDynamicSeedFromImage(usePlayerStore.getState().currentTrack?.cover_url ?? null)
  }

  return (
    <SettingsPage
      title="Appearance"
      description="Every theme is a small seed → the engine derives the full Material 3 tonal palette, so light and dark stay consistent."
      actions={
        <div className="flex gap-2 shrink-0">
          <Button variant="tonal" icon={<Upload />} onClick={() => setImportOpen(true)}>Import</Button>
          <Button icon={<Plus />} onClick={() => setEditor({ open: true, base: null })}>Create</Button>
        </div>
      }
    >
      <SettingsSection title="Color mode">
        <SettingRow
          label="Mode"
          description={mode === 'system' ? `Following your system (${resolvedMode})` : 'Applies to every theme'}
          control={
            <SegmentedButton<ThemeMode>
              value={mode}
              onChange={setMode}
              showCheck={false}
              options={[
                { value: 'light', label: 'Light', icon: <Sun /> },
                { value: 'dark', label: 'Dark', icon: <Moon /> },
                { value: 'system', label: 'Auto', icon: <MonitorSmartphone /> },
              ]}
            />
          }
        />
        <SettingRow
          icon={<Sparkles />}
          label="Color from artwork"
          description="Derive the palette from the playing track's cover — like Material You wallpaper color."
          control={<Switch checked={dynamicColor} onChange={toggleDynamic} label="Color from artwork" />}
        />
      </SettingsSection>

      <section>
        <div className="mb-3 px-1 flex items-end justify-between">
          <div>
            <h2 className="type-title-sm text-primary">Themes</h2>
            <p className="type-body-sm text-on-surface-variant">Previews show the {resolvedMode} scheme. Right-click or use ⋮ for options.</p>
          </div>
        </div>
        <div role="radiogroup" className="grid grid-cols-2 sm:grid-cols-3 gap-4">
          {builtIns.map((t) => (
            <ThemeCard key={t.id} theme={t} mode={resolvedMode} active={t.id === activeId} onSelect={() => setTheme(t.id)} onMenu={(a) => setMenu({ anchor: a, theme: t })} />
          ))}
        </div>
        {custom.length > 0 && (
          <>
            <h3 className="type-title-sm text-on-surface-variant mt-6 mb-3 px-1">Your themes</h3>
            <div role="radiogroup" className="grid grid-cols-2 sm:grid-cols-3 gap-4">
              {custom.map((t) => (
                <ThemeCard key={t.id} theme={t} mode={resolvedMode} active={t.id === activeId} onSelect={() => setTheme(t.id)} onMenu={(a) => setMenu({ anchor: a, theme: t })} />
              ))}
            </div>
          </>
        )}
        <button
          onClick={() => setEditor({ open: true, base: null })}
          className="mt-4 w-full h-14 rounded-lg border border-dashed border-outline text-on-surface-variant type-label-lg inline-flex items-center justify-center gap-2 state-layer"
        >
          <Plus className="size-5" /> Create a theme from a color
        </button>
      </section>

      <SettingsSection title="Motion & density">
        <SettingRow
          label="Motion"
          description="Reduced motion removes transitions and marquee text"
          control={
            <SegmentedButton<MotionPref>
              size="sm"
              value={motion}
              onChange={setMotion}
              showCheck={false}
              options={[{ value: 'system', label: 'System' }, { value: 'full', label: 'Full' }, { value: 'reduced', label: 'Reduced' }]}
            />
          }
        />
        <SettingRow label="Compact track rows" description="Fit more tracks on screen" control={<Switch checked={compactRows} onChange={(v) => setSetting('compactRows', v)} label="Compact rows" />} />
        <SettingRow label="Quality badges" description="Show FLAC / ALAC / MP3 tags next to tracks" control={<Switch checked={showQualityBadges} onChange={(v) => setSetting('showQualityBadges', v)} label="Quality badges" />} />
      </SettingsSection>

      {/* Theme card menu */}
      <Menu
        open={Boolean(menu)}
        onClose={() => setMenu(null)}
        anchor={menu?.anchor ?? null}
        align="end"
        items={
          menu
            ? [
                { id: 'apply', label: 'Apply', icon: <Sparkles />, onSelect: () => setTheme(menu.theme.id) },
                { id: 'edit', label: menu.theme.builtIn ? 'Customize (creates a copy)' : 'Edit', icon: <Pencil />, onSelect: () => setEditor({ open: true, base: menu.theme }) },
                { id: 'dup', label: 'Duplicate', icon: <Copy />, onSelect: () => { const c = duplicateTheme(menu.theme.id); if (c) toast(`Created “${c.name}”`) } },
                { id: 'export', label: 'Export JSON', icon: <Download />, onSelect: () => exportTheme(menu.theme) },
                ...(!menu.theme.builtIn ? [{ id: 'd', label: '', divider: true }, { id: 'del', label: 'Delete', icon: <Trash2 />, destructive: true, onSelect: () => { removeUserTheme(menu.theme.id); toast('Theme deleted') } }] : []),
              ]
            : []
        }
      />

      {editor.open && <ThemeEditorDialog open onClose={() => setEditor({ open: false, base: null })} base={editor.base} />}

      <Dialog
        open={importOpen}
        onClose={() => setImportOpen(false)}
        title="Import theme"
        actions={
          <>
            <Button variant="text" onClick={() => setImportOpen(false)}>Cancel</Button>
            <Button variant="tonal" icon={<Upload />} onClick={() => fileRef.current?.click()}>Choose file</Button>
            <Button onClick={() => doImport(importText)} disabled={!importText.trim()}>Import</Button>
          </>
        }
      >
        <p className="mb-3">Paste a <code className="font-mono text-primary">.webx-theme.json</code> document or choose a file. Themes are validated before they're applied.</p>
        <textarea
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
          spellCheck={false}
          placeholder='{ "id": "my-theme", "name": "My theme", "seed": "#FA2D48", "typography": { "body": "Manrope, sans-serif" } }'
          className="w-full h-40 rounded-xs bg-surface-highest text-on-surface font-mono text-xs p-3 outline-none focus:ring-1 ring-primary resize-y"
        />
        {importError && <p className="type-body-sm text-error mt-2">{importError}</p>}
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={async (e) => {
            const f = e.target.files?.[0]
            if (f) doImport(await f.text())
            e.target.value = ''
          }}
        />
        <TextField containerClassName="hidden" />
      </Dialog>
    </SettingsPage>
  )
}
