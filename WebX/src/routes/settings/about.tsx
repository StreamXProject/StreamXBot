import { createFileRoute } from '@tanstack/react-router'
import {
  Music2,
  Code2,
  Heart,
  Headset,
} from 'lucide-react'
import {
  SettingsPage,
  SettingsSection,
  SettingRow,
} from '@/components/settings/SettingsPrimitives'
import { useUiStore } from '@/stores/uiStore'

export const Route = createFileRoute('/settings/about')({
  component: AboutSettings,
})

const VERSION = '1.0.0'

const SUPPORT_URL = 'https://t.me/RaidenEiSupport'
const SOURCE_URL = 'https://github.com/SriKousik/StreamXBot'

function AboutSettings() {
  const latency = useUiStore((s) => s.serverLatencyMs)
  const status = useUiStore((s) => s.serverStatus)

  const openExternal = (url: string) => {
    window.open(url, '_blank', 'noopener,noreferrer')
  }

  return (
    <SettingsPage title="About">
      <div className="flex items-center gap-4 p-5 rounded-lg bg-surface-low">
        <span className="size-14 rounded-lg bg-primary text-on-primary flex items-center justify-center">
          <Music2 className="size-7" strokeWidth={2.5} />
        </span>

        <div>
          <p className="type-headline-sm text-on-surface">WebX</p>
          <p className="type-body-md text-on-surface-variant">
            Web client for StreamX · v{VERSION}
          </p>
        </div>
      </div>

      <SettingsSection title="Build">
        <SettingRow
          label="Server"
          description={
            status === 'online'
              ? `Connected${latency != null ? ` · ${latency} ms` : ''}`
              : status === 'offline'
                ? 'Unreachable'
                : 'Checking…'
          }
        />
      </SettingsSection>

      <SettingsSection title="Support & Open Source">
        <SettingRow
          icon={<Headset />}
          label="Support"
          description="Get help, report issues, or ask questions on Telegram"
          onClick={() => openExternal(SUPPORT_URL)}
        />

        <SettingRow
          icon={<Code2 />}
          label="Open source"
          description="View the StreamXBot source code on GitHub"
          onClick={() => openExternal(SOURCE_URL)}
        />
      </SettingsSection>
    </SettingsPage>
  )
}