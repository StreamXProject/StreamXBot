import React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Music2, Code2, Heart } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { useUiStore } from '@/stores/uiStore'
import { useThemeStore } from '@/theme/themeStore'

export const Route = createFileRoute('/settings/about')({
  component: AboutSettings,
})

const VERSION = '1.0.0'

function AboutSettings() {
  const latency = useUiStore((s) => s.serverLatencyMs)
  const status = useUiStore((s) => s.serverStatus)
  const theme = useThemeStore((s) => s.activeTheme)
  return (
    <SettingsPage title="About">
      <div className="flex items-center gap-4 p-5 rounded-lg bg-surface-low">
        <span className="size-14 rounded-lg bg-primary text-on-primary flex items-center justify-center"><Music2 className="size-7" strokeWidth={2.5} /></span>
        <div>
          <p className="type-headline-sm text-on-surface">WebX</p>
          <p className="type-body-md text-on-surface-variant">Web client for StreamX · v{VERSION}</p>
        </div>
      </div>
      <SettingsSection title="Build">
        <SettingRow label="Stack" description="React 19 · Vite 8 · TanStack Router & Query · Zustand · Tailwind 4 · Material Color Utilities" />
        <SettingRow label="Design system" description={`Material Design 3 tokens · theme “${theme.name}” (${theme.variant})`} />
        <SettingRow label="Server" description={status === 'online' ? `Connected${latency != null ? ` · ${latency} ms` : ''}` : status === 'offline' ? 'Unreachable' : 'Checking…'} />
      </SettingsSection>
      <SettingsSection title="Credits">
        <SettingRow icon={<Heart />} label="Lyrics" description="LRCLIB, via the StreamX server" />
        <SettingRow icon={<Code2 />} label="Open source" description="Icons by Lucide · Fonts from Google Fonts" />
      </SettingsSection>
    </SettingsPage>
  )
}
