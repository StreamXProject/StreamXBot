import React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch } from '@/components/md3'
import { useSettingsStore } from '@/stores/settingsStore'
import { SHORTCUTS } from '@/hooks/useKeyboardShortcuts'
import { Kbd } from '@/components/overlays/ShortcutsDialog'

export const Route = createFileRoute('/settings/shortcuts')({
  component: ShortcutsSettings,
})

function ShortcutsSettings() {
  const enabled = useSettingsStore((s) => s.keyboardShortcuts)
  const set = useSettingsStore((s) => s.set)
  const groups = ['Playback', 'View', 'Navigation'] as const
  return (
    <SettingsPage title="Keyboard shortcuts" description="Ignored while typing · press ? to view">
      <SettingsSection>
        <SettingRow label="Enable shortcuts" control={<Switch checked={enabled} onChange={(v) => set('keyboardShortcuts', v)} label="Enable shortcuts" />} />
      </SettingsSection>
      {groups.map((g) => (
        <SettingsSection key={g} title={g}>
          {SHORTCUTS.filter((s) => s.group === g).map((s) => (
            <SettingRow
              key={s.description}
              label={s.description}
              control={
                <span className="flex items-center gap-1">
                  {s.keys.map((k, i) => (
                    <React.Fragment key={k}>
                      {i > 0 && <span className="text-on-surface-variant text-[10px]">+</span>}
                      <Kbd>{k}</Kbd>
                    </React.Fragment>
                  ))}
                </span>
              }
            />
          ))}
        </SettingsSection>
      ))}
    </SettingsPage>
  )
}
