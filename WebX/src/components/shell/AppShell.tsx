import React, { useEffect, useRef } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { NavigationRail } from './NavigationRail'
import { TopAppBar } from './TopAppBar'
import { NavigationBar } from './NavigationBar'
import { PersistentPlayer } from '../player/PersistentPlayer'
import { FullScreenPlayer } from '../player/FullScreenPlayer'
import { QueueDrawer } from '../player/QueueDrawer'
import { TrackContextMenu } from '../overlays/TrackContextMenu'
import { AddToPlaylistDialog } from '../overlays/AddToPlaylistDialog'
import { ToastHost } from '../overlays/ToastHost'
import { CommandPalette } from '../overlays/CommandPalette'
import { ShortcutsDialog } from '../overlays/ShortcutsDialog'
import { SleepTimerDialog } from '../overlays/SleepTimerDialog'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'
import { useServerStatus } from '@/hooks/useServerStatus'
import { useAuthStore } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { useUiStore, toast } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useQueueStore } from '@/stores/queueStore'
import { audioEngine } from '@/audio/AudioEngine'
import '@/theme/themeStore'

const PUBLIC_ROUTES = ['/login', '/signup', '/setup']

export const AppShell: React.FC = () => {
  useKeyboardShortcuts()
  useServerStatus()

  // Interface size: CSS zoom on <body> scales the whole UI on desktop without breaking layouts.
  // On iOS/iPadOS/WebKit touch devices, CSS zoom breaks position:fixed coordinates, so we skip body zoom there.
  const uiScale = useSettingsStore((s) => s.uiScale)
  useEffect(() => {
    const isWebKitTouch = typeof navigator !== 'undefined' && (/iPhone|iPad|iPod/i.test(navigator.userAgent) || (navigator.maxTouchPoints > 1 && /Macintosh/i.test(navigator.userAgent)))
    if (isWebKitTouch) {
      ;(document.body.style as CSSStyleDeclaration & { zoom?: string }).zoom = ''
      return
    }
    const z = Math.min(1.3, Math.max(0.8, Number(uiScale) || 1))
    ;(document.body.style as CSSStyleDeclaration & { zoom?: string }).zoom = z === 1 ? '' : String(z)
  }, [uiScale])

  // Disable browser zoom (ctrl/cmd + wheel, ctrl/cmd + -/=/0, pinch) — interface size is controlled in Appearance
  useEffect(() => {
    const onWheel = (e: WheelEvent) => { if (e.ctrlKey || e.metaKey) e.preventDefault() }
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && ['-', '=', '+', '0', 'Minus', 'Equal'].includes(e.key)) e.preventDefault()
    }
    const onGesture = (e: Event) => e.preventDefault()
    window.addEventListener('wheel', onWheel, { passive: false })
    window.addEventListener('keydown', onKey)
    document.addEventListener('gesturestart', onGesture)
    document.addEventListener('gesturechange', onGesture)
    return () => {
      window.removeEventListener('wheel', onWheel)
      window.removeEventListener('keydown', onKey)
      document.removeEventListener('gesturestart', onGesture)
      document.removeEventListener('gesturechange', onGesture)
    }
  }, [])

  const token = useAuthStore((s) => s.token)
  const sessionExpired = useAuthStore((s) => s.sessionExpired)
  const clearExpired = useAuthStore((s) => s.clearExpired)
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const fullPlayerOpen = useUiStore((s) => s.fullPlayerOpen)
  const scrollRef = useRef<HTMLElement>(null)
  const booted = useRef(false)

  const isPublic = PUBLIC_ROUTES.some((r) => pathname.startsWith(r)) || pathname.startsWith('/share')

  // Auth gate
  useEffect(() => {
    if (!token && !isPublic) navigate({ to: '/login', replace: true })
  }, [token, isPublic, navigate])

  useEffect(() => {
    if (sessionExpired) {
      toast('Your session expired. Please sign in again.', { variant: 'error', duration: 6000 })
      clearExpired()
    }
  }, [sessionExpired, clearExpired])

  // One-time boot: sync library, restore queue, record history, surface playback errors
  useEffect(() => {
    if (booted.current || !token) return
    booted.current = true
    void useLibraryStore.getState().sync()
    void useQueueStore.getState().restoreSession()
    const unsubEnd = audioEngine.subscribeState((s) => {
      if (s.currentTrack && s.isPlaying) useLibraryStore.getState().addToRecent(s.currentTrack)
    })
    const unsubErr = audioEngine.onError((msg, track) => {
      toast(`${msg}${track ? ` — ${track.title}` : ''}`, {
        variant: 'error',
        duration: 6000,
        action: { label: 'Skip', onClick: () => void useQueueStore.getState().nextTrack() },
      })
    })
    return () => {
      unsubEnd()
      unsubErr()
    }
  }, [token])

  // Remember scroll position per route; restore it when coming back, otherwise start at top
  const scrollPositions = useRef<Map<string, number>>(new Map())
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const saved = scrollPositions.current.get(pathname) ?? 0
    if (saved <= 0) {
      el.scrollTo({ top: 0 })
      return
    }
    // Content may still be loading — retry for a short while until the page is tall enough
    let raf = 0
    let tries = 0
    const attempt = () => {
      const node = scrollRef.current
      if (!node) return
      node.scrollTop = saved
      const reached = Math.abs(node.scrollTop - saved) < 2
      if (!reached && tries++ < 90) raf = requestAnimationFrame(attempt)
    }
    raf = requestAnimationFrame(attempt)
    return () => cancelAnimationFrame(raf)
  }, [pathname])

  // Keep the saved position fresh while the user scrolls
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const onScroll = () => {
      scrollPositions.current.set(pathname, el.scrollTop)
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [pathname])

  // Stop painting the shell while the full player covers it
  const shellRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = shellRef.current
    if (!el) return
    if (fullPlayerOpen) {
      const t = setTimeout(() => {
        el.style.visibility = 'hidden'
      }, 450)
      return () => clearTimeout(t)
    }
    el.style.visibility = ''
  }, [fullPlayerOpen])

  if (isPublic) {
    return (
      <div className="h-full min-h-full w-full flex overflow-y-auto bg-surface text-on-surface pt-[env(safe-area-inset-top,0px)] pb-[env(safe-area-inset-bottom,0px)] pl-[env(safe-area-inset-left,0px)] pr-[env(safe-area-inset-right,0px)]">
        <main className="w-full min-h-full flex items-center justify-center p-4">
          <Outlet />
        </main>
        <ToastHost />
      </div>
    )
  }

  return (
    <div className="fixed inset-0 h-full h-[100dvh] w-full flex flex-col bg-surface text-on-surface overflow-hidden pl-[env(safe-area-inset-left,0px)] pr-[env(safe-area-inset-right,0px)]">
      <div
        ref={shellRef}
        className="flex-1 min-h-0 flex flex-col"
        // @ts-expect-error React 19 supports inert
        inert={fullPlayerOpen ? '' : undefined}
      >
        <div className="flex-1 min-h-0 flex">
          <NavigationRail />
          <div className="flex-1 min-w-0 min-h-0 flex flex-col relative">
            <TopAppBar />
            <main id="app-scroll" ref={scrollRef} className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden relative pb-[calc(var(--webx-mini-player-height)+var(--webx-nav-height)+env(safe-area-inset-bottom,0px))] md:pb-0" style={{ scrollbarGutter: 'stable' }}>
              <Outlet />
            </main>
          </div>
        </div>
        <PersistentPlayer />
        <NavigationBar />
      </div>

      {/* Overlays */}
      <QueueDrawer />
      <FullScreenPlayer />
      <TrackContextMenu />
      <AddToPlaylistDialog />
      <CommandPalette />
      <ShortcutsDialog />
      <SleepTimerDialog />
      <ToastHost />
    </div>
  )
}
