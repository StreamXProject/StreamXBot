import { useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { usePlayerStore } from '@/stores/playerStore'
import { useQueueStore } from '@/stores/queueStore'
import { useUiStore } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { audioEngine } from '@/audio/AudioEngine'

export interface ShortcutDef {
  keys: string[]
  description: string
  group: 'Playback' | 'Navigation' | 'View'
}

export const SHORTCUTS: ShortcutDef[] = [
  { keys: ['Space', 'K'], description: 'Play / pause', group: 'Playback' },
  { keys: ['→', '←'], description: 'Seek ±5 s (Shift: ±30 s)', group: 'Playback' },
  { keys: ['Shift', 'N'], description: 'Next track', group: 'Playback' },
  { keys: ['Shift', 'P'], description: 'Previous track', group: 'Playback' },
  { keys: ['↑', '↓'], description: 'Volume ±5 %', group: 'Playback' },
  { keys: ['M'], description: 'Mute', group: 'Playback' },
  { keys: ['L'], description: 'Like current track', group: 'Playback' },
  { keys: ['S'], description: 'Toggle shuffle', group: 'Playback' },
  { keys: ['R'], description: 'Cycle repeat', group: 'Playback' },
  { keys: ['F'], description: 'Full-screen player', group: 'View' },
  { keys: ['Q'], description: 'Queue', group: 'View' },
  { keys: ['I'], description: 'Lyrics', group: 'View' },
  { keys: ['Esc'], description: 'Close overlays', group: 'View' },
  { keys: ['Ctrl', 'K'], description: 'Search anything', group: 'Navigation' },
  { keys: ['/'], description: 'Focus search', group: 'Navigation' },
  { keys: ['G', 'H'], description: 'Go home', group: 'Navigation' },
  { keys: ['G', 'L'], description: 'Go to library', group: 'Navigation' },
  { keys: ['G', 'S'], description: 'Go to settings', group: 'Navigation' },
  { keys: ['?'], description: 'Show shortcuts', group: 'Navigation' },
]

const isEditable = (el: EventTarget | null) => {
  const t = el as HTMLElement | null
  if (!t) return false
  const tag = t.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || t.isContentEditable
}

export function useKeyboardShortcuts() {
  const navigate = useNavigate()
  useEffect(() => {
    let pendingG = 0
    const onKey = (e: KeyboardEvent) => {
      if (!useSettingsStore.getState().keyboardShortcuts) return
      const ui = useUiStore.getState()
      const mod = e.ctrlKey || e.metaKey

      if (mod && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        ui.setCommandPalette(!ui.commandPaletteOpen)
        return
      }
      if (isEditable(e.target)) return
      if (e.altKey) return

      const player = usePlayerStore.getState()
      const queue = useQueueStore.getState()

      if (e.key === 'Escape') {
        if (ui.contextMenu) ui.closeContextMenu()
        else if (ui.commandPaletteOpen) ui.setCommandPalette(false)
        else if (ui.shortcutsOpen) ui.setShortcutsOpen(false)
        else if (ui.addToPlaylist) ui.closeAddToPlaylist()
        else if (ui.queueDrawerOpen) ui.closeQueueDrawer()
        else if (ui.fullPlayerOpen) ui.closeFullPlayer()
        return
      }

      // two-key "g" sequences
      if (pendingG && Date.now() - pendingG < 900 && !mod) {
        pendingG = 0
        const k = e.key.toLowerCase()
        if (k === 'h') return void navigate({ to: '/' })
        if (k === 'l') return void navigate({ to: '/library' })
        if (k === 's') return void navigate({ to: '/settings' })
        if (k === 'e') return void navigate({ to: '/search' })
      }

      switch (e.code) {
        case 'Space':
          e.preventDefault()
          player.togglePlay()
          break
        case 'KeyK':
          if (!mod) player.togglePlay()
          break
        case 'ArrowRight':
          e.preventDefault()
          audioEngine.seekBy(e.shiftKey ? 30 : 5)
          break
        case 'ArrowLeft':
          e.preventDefault()
          audioEngine.seekBy(e.shiftKey ? -30 : -5)
          break
        case 'ArrowUp':
          e.preventDefault()
          player.adjustVolume(0.05)
          break
        case 'ArrowDown':
          e.preventDefault()
          player.adjustVolume(-0.05)
          break
        case 'KeyN':
          if (e.shiftKey || mod) {
            e.preventDefault()
            void queue.nextTrack()
          }
          break
        case 'KeyP':
          if (e.shiftKey || mod) {
            e.preventDefault()
            void queue.previousTrack()
          }
          break
        case 'KeyM':
          if (!mod) player.toggleMute()
          break
        case 'KeyL':
          if (!mod && player.currentTrack) void useLibraryStore.getState().toggleLike(player.currentTrack)
          break
        case 'KeyS':
          if (!mod) queue.toggleShuffle()
          break
        case 'KeyR':
          if (!mod) queue.cycleRepeatMode()
          break
        case 'KeyF':
          if (!mod && player.currentTrack) ui.toggleFullPlayer()
          break
        case 'KeyQ':
          if (!mod) ui.toggleQueueDrawer()
          break
        case 'KeyI':
          if (!mod && player.currentTrack) ui.toggleLyrics()
          break
        case 'KeyG':
          if (!mod) pendingG = Date.now()
          break
        case 'Slash':
          if (!mod) {
            e.preventDefault()
            ui.setCommandPalette(true)
          }
          break
        default:
          if (e.key === '?') {
            e.preventDefault()
            ui.setShortcutsOpen(!ui.shortcutsOpen)
          }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate])
}
