import React, { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { RefreshCw, Trash2, Download, Heart, ListMusic, Clock, RotateCcw } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Button, SegmentedButton } from '@/components/md3'
import { useLibraryStore } from '@/stores/libraryStore'
import { useSettingsStore, type LibraryTab, type StartPage } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { useAuthStore, sessionKind } from '@/stores/authStore'

export const Route = createFileRoute('/settings/library')({
  component: LibrarySettings,
})

function LibrarySettings() {
  const qc = useQueryClient()
  const lib = useLibraryStore()
  const settings = useSettingsStore()
  const kind = useAuthStore((s) => sessionKind(s.token, s.user))
  const [syncing, setSyncing] = useState(false)

  const sync = async () => {
    setSyncing(true)
    await lib.sync()
    await qc.invalidateQueries()
    setSyncing(false)
    toast(lib.lastError ? `Sync finished with errors: ${lib.lastError}` : 'Library synced', { variant: lib.lastError ? 'error' : 'default' })
  }

  const exportLibrary = () => {
    const data = { exportedAt: new Date().toISOString(), likedTracks: lib.likedTracks, playlists: lib.playlists, recent: lib.recentTracks }
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `webx-library-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const clearCache = () => {
    if (!confirm('Clear cached data on this device? Your account data on the server is not affected.')) return
    for (const k of Object.keys(localStorage)) {
      if (k.startsWith('webx.queue') || k.startsWith('webx.playback') || k.startsWith('webx.library') || k.startsWith('webx.search')) localStorage.removeItem(k)
    }
    qc.clear()
    toast('Local cache cleared')
  }

  const resetSettings = () => {
    if (!confirm('Reset all settings to defaults? Server address and themes are kept.')) return
    settings.reset()
    toast('Settings reset')
  }

  return (
    <SettingsPage title="Library & data">
      <SettingsSection title="Overview">
        <SettingRow icon={<Heart />} label="Favourites" description={`${lib.likedTracks.length} tracks${kind === 'user' ? ' · synced with your account' : ' · stored on this device'}`} />
        <SettingRow icon={<ListMusic />} label="Playlists" description={kind === 'user' ? `${lib.playlists.length} on your account` : 'Sign in to create playlists'} />
        <SettingRow icon={<Clock />} label="Recently played" description={`${lib.recentTracks.length} tracks remembered locally`} />
        <SettingRow
          icon={<RefreshCw />}
          label="Sync now"
          description={lib.lastError ? `Last error: ${lib.lastError}` : 'Refresh favourites, playlists and cached pages'}
          control={<Button variant="tonal" size="sm" loading={syncing} onClick={() => void sync()} disabled={kind === 'none'}>Sync</Button>}
        />
      </SettingsSection>

      <SettingsSection title="Defaults">
        <SettingRow
          label="Start page"
          control={<SegmentedButton<StartPage> size="sm" showCheck={false} value={settings.startPage} onChange={(v) => settings.set('startPage', v)} options={[{ value: '/', label: 'Home' }, { value: '/search', label: 'Search' }, { value: '/library', label: 'Library' }]} />}
        />
        <SettingRow
          label="Default library tab"
          control={<SegmentedButton<LibraryTab> size="sm" showCheck={false} value={settings.defaultLibraryTab} onChange={(v) => settings.set('defaultLibraryTab', v)} options={[{ value: 'liked', label: 'Liked' }, { value: 'playlists', label: 'Playlists' }, { value: 'history', label: 'History' }]} />}
        />
      </SettingsSection>

      <SettingsSection title="Data">
        <SettingRow icon={<Download />} label="Export library" description="Download favourites, playlists and history as JSON" onClick={exportLibrary} />
        <SettingRow icon={<Trash2 />} label="Clear local cache" description="Queue, playback position, cached pages and recent searches" onClick={clearCache} />
        <SettingRow icon={<RotateCcw />} label="Reset settings" description="Restore all preferences to their defaults" onClick={resetSettings} />
      </SettingsSection>
    </SettingsPage>
  )
}
