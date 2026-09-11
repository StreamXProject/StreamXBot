import { useEffect, useState } from 'react'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

let deferredPrompt: BeforeInstallPromptEvent | null = null
let waitingWorker: ServiceWorker | null = null
const listeners = new Set<() => void>()
const notify = () => listeners.forEach((fn) => fn())

export function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone === true
}

export function isIOS(): boolean {
  if (typeof navigator === 'undefined') return false
  return /iphone|ipad|ipod/i.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}

/** Call once at startup (production only). */
export function registerServiceWorker(): void {
  if (typeof window === 'undefined' || !('serviceWorker' in navigator)) return

  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault()
    deferredPrompt = e as BeforeInstallPromptEvent
    notify()
  })
  window.addEventListener('appinstalled', () => {
    deferredPrompt = null
    notify()
    toast('WebX installed')
  })

  navigator.serviceWorker
    .register('/sw.js', { scope: '/' })
    .then((reg) => {
      const track = (sw: ServiceWorker | null) => {
        if (!sw) return
        sw.addEventListener('statechange', () => {
          if (sw.state === 'installed' && navigator.serviceWorker.controller) {
            waitingWorker = sw
            notify()
            if (useSettingsStore.getState().pwaAutoUpdate) applyUpdate()
            else toast('A new version of WebX is ready', { duration: 10_000, action: { label: 'Reload', onClick: applyUpdate } })
          }
        })
      }
      track(reg.installing)
      reg.addEventListener('updatefound', () => track(reg.installing))
      // check for updates when the tab regains focus
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') void reg.update().catch(() => {})
      })
    })
    .catch((err) => console.warn('[PWA] service worker registration failed', err))

  let refreshing = false
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (refreshing) return
    refreshing = true
    window.location.reload()
  })
}

export function applyUpdate(): void {
  waitingWorker?.postMessage('SKIP_WAITING')
}

export async function checkForUpdate(): Promise<boolean> {
  const reg = await navigator.serviceWorker?.getRegistration()
  if (!reg) return false
  await reg.update()
  return Boolean(reg.waiting)
}

export async function clearOfflineCache(): Promise<void> {
  const keys = await caches.keys()
  await Promise.all(keys.map((k) => caches.delete(k)))
}

export async function cacheUsage(): Promise<{ usage: number; quota: number } | null> {
  if (!navigator.storage?.estimate) return null
  const e = await navigator.storage.estimate()
  return { usage: e.usage ?? 0, quota: e.quota ?? 0 }
}

export function usePwa() {
  const [, tick] = useState(0)
  useEffect(() => {
    const fn = () => tick((n) => n + 1)
    listeners.add(fn)
    return () => {
      listeners.delete(fn)
    }
  }, [])
  return {
    canInstall: Boolean(deferredPrompt),
    installed: isStandalone(),
    updateReady: Boolean(waitingWorker),
    ios: isIOS(),
    install: async () => {
      if (!deferredPrompt) return false
      await deferredPrompt.prompt()
      const { outcome } = await deferredPrompt.userChoice
      deferredPrompt = null
      notify()
      return outcome === 'accepted'
    },
  }
}
