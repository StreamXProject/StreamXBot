import React, { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Download, RefreshCw, Trash2, Smartphone, Share, CheckCircle2, WifiOff, Bell } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Switch, Button } from '@/components/md3'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { usePwa, applyUpdate, checkForUpdate, clearOfflineCache, cacheUsage } from '@/hooks/usePwa'

export const Route = createFileRoute('/settings/app')({
  component: AppSettings,
})

const fmtBytes = (n: number) => (n > 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.round(n / 1024)} KB`)

function AppSettings() {
  const s = useSettingsStore()
  const pwa = usePwa()
  const [usage, setUsage] = useState<{ usage: number; quota: number } | null>(null)
  const [checking, setChecking] = useState(false)
  const [online, setOnline] = useState(typeof navigator === 'undefined' ? true : navigator.onLine)
  const swSupported = typeof navigator !== 'undefined' && 'serviceWorker' in navigator

  useEffect(() => {
    void cacheUsage().then(setUsage)
    const on = () => setOnline(true)
    const off = () => setOnline(false)
    window.addEventListener('online', on)
    window.addEventListener('offline', off)
    return () => {
      window.removeEventListener('online', on)
      window.removeEventListener('offline', off)
    }
  }, [])

  const check = async () => {
    setChecking(true)
    try {
      const ready = await checkForUpdate()
      toast(ready ? 'Update downloaded — reloading' : 'You’re on the latest version')
      if (ready) applyUpdate()
    } catch {
      toast('Could not check for updates', { variant: 'error' })
    } finally {
      setChecking(false)
    }
  }

  return (
    <SettingsPage title="App & offline" description="Install, updates and offline storage">
      <SettingsSection title="Install">
        {pwa.installed ? (
          <SettingRow icon={<CheckCircle2 className="text-tertiary" />} label="Installed" description="You’re running WebX as an app." />
        ) : pwa.canInstall ? (
          <SettingRow icon={<Smartphone />} label="Install WebX" description="Home screen / dock, own window" control={<Button icon={<Download />} onClick={() => void pwa.install()}>Install</Button>} />
        ) : pwa.ios ? (
          <SettingRow icon={<Share />} label="Add to Home Screen" description="Safari → Share → Add to Home Screen" />
        ) : (
          <SettingRow icon={<Smartphone />} label="Install WebX" description="Browser menu → Install app" />
        )}
      </SettingsSection>

      <SettingsSection title="Updates">
        <SettingRow
          icon={<RefreshCw />}
          label="Check for updates"
          description={pwa.updateReady ? 'New version ready' : 'Checked automatically on reopen'}
          control={pwa.updateReady ? <Button onClick={applyUpdate}>Reload now</Button> : <Button variant="tonal" loading={checking} disabled={!swSupported} onClick={() => void check()}>Check</Button>}
        />
        <SettingRow label="Apply updates automatically" description="Reload when a new version downloads" control={<Switch checked={s.pwaAutoUpdate} onChange={(v) => s.set('pwaAutoUpdate', v)} label="Auto-update" />} />
      </SettingsSection>

      <SettingsSection title="Offline & storage" description="App shell, fonts, artwork · streams stay live">
        <SettingRow
          icon={online ? <CheckCircle2 className="text-tertiary" /> : <WifiOff className="text-error" />}
          label={online ? 'Online' : 'Offline'}
          description={usage ? `${fmtBytes(usage.usage)} used of ${fmtBytes(usage.quota)} available` : swSupported ? 'Measuring storage…' : 'Service workers are not supported in this browser'}
        />
        <SettingRow
          icon={<Trash2 />}
          label="Clear cached files"
          description="Artwork, fonts, app files · settings kept"
          control={<Button variant="outlined" onClick={() => void clearOfflineCache().then(() => { toast('Cache cleared'); void cacheUsage().then(setUsage) })}>Clear</Button>}
        />
      </SettingsSection>

      <SettingsSection title="System">
        <SettingRow
          icon={<Bell />}
          label="Media controls"
          description="Lock screen and media keys · always on"
        />
      </SettingsSection>
    </SettingsPage>
  )
}
