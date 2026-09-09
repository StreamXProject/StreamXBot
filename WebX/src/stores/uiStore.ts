import { create } from 'zustand'
import type { Track } from '@/schemas/track'

export type FullPlayerPane = 'lyrics' | 'queue' | 'info'

export interface Toast {
  id: number
  message: string
  action?: { label: string; onClick: () => void }
  duration?: number
  variant?: 'default' | 'error'
}

export interface ContextMenuState {
  track: Track
  tracks?: Track[]
  index?: number
  anchor: HTMLElement | { x: number; y: number }
}

interface UiStoreState {
  sidebarCollapsed: boolean
  fullPlayerOpen: boolean
  fullPlayerPane: FullPlayerPane
  /** True when the pane was requested explicitly (e.g. lyrics button) rather than defaulted */
  fullPlayerPaneExplicit: boolean
  queueDrawerOpen: boolean
  commandPaletteOpen: boolean
  shortcutsOpen: boolean
  sleepTimerOpen: boolean
  addToPlaylist: Track[] | null
  contextMenu: ContextMenuState | null
  toasts: Toast[]
  /** Online status of the API server */
  serverStatus: 'unknown' | 'online' | 'offline'
  serverLatencyMs: number | null

  toggleSidebar: () => void
  setSidebarCollapsed: (collapsed: boolean) => void
  openFullPlayer: (pane?: FullPlayerPane) => void
  closeFullPlayer: () => void
  toggleFullPlayer: () => void
  setFullPlayerPane: (pane: FullPlayerPane) => void
  toggleLyrics: () => void
  openQueueDrawer: () => void
  closeQueueDrawer: () => void
  toggleQueueDrawer: () => void
  setCommandPalette: (open: boolean) => void
  setShortcutsOpen: (open: boolean) => void
  setSleepTimerOpen: (open: boolean) => void
  openAddToPlaylist: (tracks: Track[]) => void
  closeAddToPlaylist: () => void
  openContextMenu: (state: ContextMenuState) => void
  closeContextMenu: () => void
  toast: (message: string, opts?: Omit<Toast, 'id' | 'message'>) => number
  dismissToast: (id: number) => void
  setServerStatus: (status: UiStoreState['serverStatus'], latency?: number | null) => void
}

let toastId = 0
const SIDEBAR_KEY = 'webx.ui.sidebarCollapsed'

export const useUiStore = create<UiStoreState>((set, get) => ({
  sidebarCollapsed: typeof window !== 'undefined' && localStorage.getItem(SIDEBAR_KEY) === 'true',
  fullPlayerOpen: false,
  fullPlayerPane: 'lyrics',
  fullPlayerPaneExplicit: false,
  queueDrawerOpen: false,
  commandPaletteOpen: false,
  shortcutsOpen: false,
  sleepTimerOpen: false,
  addToPlaylist: null,
  contextMenu: null,
  toasts: [],
  serverStatus: 'unknown',
  serverLatencyMs: null,

  toggleSidebar: () => {
    const next = !get().sidebarCollapsed
    localStorage.setItem(SIDEBAR_KEY, String(next))
    set({ sidebarCollapsed: next })
  },
  setSidebarCollapsed: (collapsed) => {
    localStorage.setItem(SIDEBAR_KEY, String(collapsed))
    set({ sidebarCollapsed: collapsed })
  },

  openFullPlayer: (pane) => set({ fullPlayerOpen: true, queueDrawerOpen: false, fullPlayerPaneExplicit: Boolean(pane), ...(pane ? { fullPlayerPane: pane } : {}) }),
  closeFullPlayer: () => set({ fullPlayerOpen: false }),
  toggleFullPlayer: () => set((s) => ({ fullPlayerOpen: !s.fullPlayerOpen, queueDrawerOpen: false, fullPlayerPaneExplicit: false })),
  setFullPlayerPane: (pane) => set({ fullPlayerPane: pane }),
  toggleLyrics: () => {
    const s = get()
    if (s.fullPlayerOpen && s.fullPlayerPane === 'lyrics') set({ fullPlayerOpen: false })
    else set({ fullPlayerOpen: true, fullPlayerPane: 'lyrics', fullPlayerPaneExplicit: true, queueDrawerOpen: false })
  },

  openQueueDrawer: () => set({ queueDrawerOpen: true }),
  closeQueueDrawer: () => set({ queueDrawerOpen: false }),
  toggleQueueDrawer: () => {
    const s = get()
    if (s.fullPlayerOpen) {
      set({ fullPlayerPane: s.fullPlayerPane === 'queue' ? 'lyrics' : 'queue', fullPlayerPaneExplicit: true })
    } else set({ queueDrawerOpen: !s.queueDrawerOpen })
  },

  setCommandPalette: (open) => set({ commandPaletteOpen: open }),
  setShortcutsOpen: (open) => set({ shortcutsOpen: open }),
  setSleepTimerOpen: (open) => set({ sleepTimerOpen: open }),
  openAddToPlaylist: (tracks) => set({ addToPlaylist: tracks, contextMenu: null }),
  closeAddToPlaylist: () => set({ addToPlaylist: null }),
  openContextMenu: (state) => set({ contextMenu: state }),
  closeContextMenu: () => set({ contextMenu: null }),

  toast: (message, opts) => {
    const id = ++toastId
    const toast: Toast = { id, message, duration: 4000, ...opts }
    set((s) => ({ toasts: [...s.toasts.slice(-2), toast] }))
    if (toast.duration && toast.duration > 0) setTimeout(() => get().dismissToast(id), toast.duration)
    return id
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  setServerStatus: (status, latency = null) => set({ serverStatus: status, serverLatencyMs: latency }),
}))

export const toast = (message: string, opts?: Omit<Toast, 'id' | 'message'>) => useUiStore.getState().toast(message, opts)
