import { useEffect } from 'react'
import { checkHealth } from '@/api/health'
import { useUiStore } from '@/stores/uiStore'
import { useSettingsStore } from '@/stores/settingsStore'

/** Periodically pings /health and keeps `serverStatus` in the UI store fresh. */
export function useServerStatus(intervalMs = 45_000) {
  const apiBaseUrl = useSettingsStore((s) => s.apiBaseUrl)
  const setServerStatus = useUiStore((s) => s.setServerStatus)

  useEffect(() => {
    let cancelled = false
    let timer: number | undefined
    const ping = async () => {
      const res = await checkHealth(undefined, 6000)
      if (cancelled) return
      setServerStatus(res.ok ? 'online' : 'offline', res.ok ? res.latencyMs : null)
      timer = window.setTimeout(ping, res.ok ? intervalMs : 15_000)
    }
    void ping()
    const onOnline = () => void ping()
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', () => setServerStatus('offline'))
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      window.removeEventListener('online', onOnline)
    }
  }, [apiBaseUrl, intervalMs, setServerStatus])
}
