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
            applyUpdate()
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
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return false
  const reg = await navigator.serviceWorker.getRegistration()
  if (!reg) return false

  return new Promise<boolean>((resolve) => {
    let resolved = false
    const done = (val: boolean) => {
      if (!resolved) {
        resolved = true
        resolve(val)
      }
    }

    if (reg.waiting || reg.installing) {
      done(true)
      return
    }

    reg.addEventListener('updatefound', () => done(true), { once: true })

    reg
      .update()
      .then(() => {
        setTimeout(() => {
          done(Boolean(reg.waiting || reg.installing))
        }, 1200)
      })
      .catch(() => done(false))
  })
}

/**
 * Hard-bypasses all caches, unregisters stale service workers, and forces
 * the browser to fetch the currently deployed frontend build directly from the server.
 */
export async function forceFetchLatestFrontend(): Promise<void> {
  if (typeof window === 'undefined') return

  // 1. Purge all CacheStorage
  if (typeof caches !== 'undefined') {
    try {
      const keys = await caches.keys()
      await Promise.all(keys.map((k) => caches.delete(k)))
    } catch (e) {
      console.warn('[PWA] Error clearing cache storage:', e)
    }
  }

  // 2. Unregister all service workers so they don't intercept the reload
  if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
    try {
      const registrations = await navigator.serviceWorker.getRegistrations()
      await Promise.all(registrations.map((r) => r.unregister()))
    } catch (e) {
      console.warn('[PWA] Error unregistering service workers:', e)
    }
  }

  // 3. Pre-fetch the live index.html with a unique cache-busting timestamp
  try {
    const bust = `_t=${Date.now()}`
    await fetch(`/?${bust}`, {
      cache: 'reload',
      headers: { 'Cache-Control': 'no-cache, no-store, must-revalidate', Pragma: 'no-cache' },
    })
  } catch (e) {
    console.warn('[PWA] Error prefetching live index:', e)
  }

  // 4. Force hard-reload with cache-buster query so iOS WebKit drops memory cache
  const url = new URL(window.location.href)
  url.searchParams.set('_v', String(Date.now()))
  window.location.replace(url.toString())
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
