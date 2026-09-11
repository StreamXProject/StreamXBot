/**
 * Discord Remote Auth client service.
 *
 * Connects to the Stream backend (/ws/discord/remote-auth) to initiate
 * and monitor a Discord QR Code Login session. Falls back to REST polling
 * if WebSockets are blocked.
 */
import { useSettingsStore } from '@/stores/settingsStore'

export interface DiscordRemoteUser {
  id: string
  username: string
  discriminator?: string
  avatar?: string | null
}

export type RemoteAuthStage =
  | 'idle'
  | 'connecting'
  | 'qr'
  | 'scanned'
  | 'confirming'
  | 'success'
  | 'error'
  | 'cancelled'

export interface RemoteAuthCallbacks {
  onStageChange?: (stage: RemoteAuthStage) => void
  onQrUrl?: (url: string) => void
  onScanned?: (user: DiscordRemoteUser) => void
  onSuccess?: (token: string, user?: DiscordRemoteUser) => void
  onError?: (error: string) => void
}

function getWsUrl(): string {
  const base = useSettingsStore.getState().apiBaseUrl || (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8000')
  const wsProto = base.startsWith('https') ? 'wss:' : 'ws:'
  const host = base.replace(/^https?:\/\//, '').replace(/\/+$/, '')
  return `${wsProto}//${host}/ws/discord/remote-auth`
}

function getRestBase(): string {
  return useSettingsStore.getState().apiBaseUrl || (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8000')
}

export class DiscordRemoteAuthClient {
  private ws: WebSocket | null = null
  private pollTimer: number | null = null
  private activeSessionId: string | null = null
  private cancelled = false
  private callbacks: RemoteAuthCallbacks = {}

  constructor(callbacks: RemoteAuthCallbacks = {}) {
    this.callbacks = callbacks
  }

  public start(): void {
    this.cancelled = false
    this.callbacks.onStageChange?.('connecting')

    try {
      const wsUrl = getWsUrl()
      this.ws = new WebSocket(wsUrl)
    } catch {
      // Direct WS failed, fallback to REST
      void this.startRestPolling()
      return
    }

    let opened = false

    this.ws.onopen = () => {
      opened = true
    }

    this.ws.onmessage = (event) => {
      if (this.cancelled) return
      try {
        const data = JSON.parse(event.data as string)
        const type = data.type

        switch (type) {
          case 'status':
            if (data.status === 'connecting') this.callbacks.onStageChange?.('connecting')
            if (data.status === 'confirming') this.callbacks.onStageChange?.('confirming')
            break

          case 'qr':
            if (data.url) {
              this.callbacks.onStageChange?.('qr')
              this.callbacks.onQrUrl?.(data.url)
            }
            break

          case 'scanned':
            this.callbacks.onStageChange?.('scanned')
            if (data.user) this.callbacks.onScanned?.(data.user)
            break

          case 'success':
            this.callbacks.onStageChange?.('success')
            if (data.token) this.callbacks.onSuccess?.(data.token, data.user)
            this.cleanup()
            break

          case 'error':
            this.callbacks.onStageChange?.('error')
            this.callbacks.onError?.(data.message || 'Remote authentication error')
            this.cleanup()
            break

          case 'cancelled':
            this.callbacks.onStageChange?.('cancelled')
            this.cleanup()
            break
        }
      } catch {
        /* ignore JSON parse errors */
      }
    }

    this.ws.onerror = () => {
      if (!opened && !this.cancelled) {
        // Fallback to REST polling if WS couldn't establish
        this.cleanup()
        void this.startRestPolling()
      }
    }

    this.ws.onclose = () => {
      if (!this.cancelled && !opened) {
        void this.startRestPolling()
      }
    }
  }

  private async startRestPolling(): Promise<void> {
    if (this.cancelled) return
    const base = getRestBase()

    try {
      const res = await fetch(`${base}/api/discord/remote-auth/start`, { method: 'POST' })
      if (!res.ok) {
        const errText = await res.text()
        throw new Error(`Failed to start session (${res.status}): ${errText}`)
      }

      const initData = (await res.json()) as { session_id: string; url?: string; fingerprint?: string }
      this.activeSessionId = initData.session_id

      if (initData.url) {
        this.callbacks.onStageChange?.('qr')
        this.callbacks.onQrUrl?.(initData.url)
      }

      // Poll every 1.5s
      this.pollTimer = window.setInterval(async () => {
        if (this.cancelled || !this.activeSessionId) return
        try {
          const pollRes = await fetch(`${base}/api/discord/remote-auth/status/${this.activeSessionId}`)
          if (!pollRes.ok) return
          const statusData = (await pollRes.json()) as {
            status: string
            url?: string
            user?: DiscordRemoteUser
            token?: string
            error?: string
          }

          if (statusData.url) this.callbacks.onQrUrl?.(statusData.url)

          if (statusData.status === 'scanned') {
            this.callbacks.onStageChange?.('scanned')
            if (statusData.user) this.callbacks.onScanned?.(statusData.user)
          } else if (statusData.status === 'confirming') {
            this.callbacks.onStageChange?.('confirming')
          } else if (statusData.status === 'success' && statusData.token) {
            this.callbacks.onStageChange?.('success')
            this.callbacks.onSuccess?.(statusData.token, statusData.user)
            this.cancel()
          } else if (statusData.status === 'error') {
            this.callbacks.onStageChange?.('error')
            this.callbacks.onError?.(statusData.error || 'Authentication failed')
            this.cancel()
          }
        } catch {
          /* ignore transient network blips while polling */
        }
      }, 1500)
    } catch (exc) {
      if (!this.cancelled) {
        this.callbacks.onStageChange?.('error')
        this.callbacks.onError?.((exc as Error).message || 'Failed to connect to backend')
      }
    }
  }

  public cancel(): void {
    this.cancelled = true
    if (this.activeSessionId) {
      const base = getRestBase()
      fetch(`${base}/api/discord/remote-auth/cancel/${this.activeSessionId}`, { method: 'POST' }).catch(() => {
        /* ignore */
      })
      this.activeSessionId = null
    }
    this.cleanup()
  }

  private cleanup(): void {
    if (this.pollTimer !== null) {
      window.clearInterval(this.pollTimer)
      this.pollTimer = null
    }
    if (this.ws) {
      try {
        this.ws.close()
      } catch {
        /* ignore */
      }
      this.ws = null
    }
  }
}
