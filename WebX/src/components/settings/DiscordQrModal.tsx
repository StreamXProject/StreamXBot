import React, { useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'
import { QrCode, Smartphone, CheckCircle2, AlertCircle, RefreshCw, Loader2 } from 'lucide-react'
import { Dialog } from '@/components/md3/Dialog'
import { Button } from '@/components/md3/Button'
import {
  DiscordRemoteAuthClient,
  type RemoteAuthStage,
  type DiscordRemoteUser,
} from '@/services/discordRemoteAuth'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'
import { saveIntegrationsToServer } from '@/services/integrationsSync'

export interface DiscordQrModalProps {
  open: boolean
  onClose: () => void
}

export const DiscordQrModal: React.FC<DiscordQrModalProps> = ({ open, onClose }) => {
  const [stage, setStage] = useState<RemoteAuthStage>('idle')
  const [qrUrl, setQrUrl] = useState<string | null>(null)
  const [user, setUser] = useState<DiscordRemoteUser | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const clientRef = useRef<DiscordRemoteAuthClient | null>(null)
  const setSetting = useSettingsStore((s) => s.set)

  const startSession = () => {
    setStage('connecting')
    setQrUrl(null)
    setUser(null)
    setErrorMsg(null)

    if (clientRef.current) {
      clientRef.current.cancel()
    }

    const client = new DiscordRemoteAuthClient({
      onStageChange: (s) => setStage(s),
      onQrUrl: (url) => setQrUrl(url),
      onScanned: (u) => setUser(u),
      onSuccess: (token, u) => {
        setSetting('discordUserToken', token)
        setSetting('discordEnabled', true)
        void saveIntegrationsToServer({ discord: { token, enabled: true } })
        toast(u ? `Logged in to Discord as @${u.username}!` : 'Discord account connected!')
        setTimeout(() => {
          onClose()
        }, 1500)
      },
      onError: (err) => {
        setErrorMsg(err)
      },
    })

    clientRef.current = client
    client.start()
  }

  useEffect(() => {
    if (open) {
      startSession()
    } else {
      if (clientRef.current) {
        clientRef.current.cancel()
        clientRef.current = null
      }
      setStage('idle')
      setQrUrl(null)
      setUser(null)
      setErrorMsg(null)
    }
    return () => {
      if (clientRef.current) {
        clientRef.current.cancel()
        clientRef.current = null
      }
    }
  }, [open])

  // Draw QR code whenever qrUrl changes or canvas mounts
  useEffect(() => {
    if (qrUrl && canvasRef.current) {
      QRCode.toCanvas(canvasRef.current, qrUrl, {
        width: 220,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#ffffff',
        },
      }).catch((err) => {
        console.error('Failed to render QR canvas', err)
      })
    }
  }, [qrUrl, stage])

  const avatarUrl = user?.avatar
    ? `https://cdn.discordapp.com/avatars/${user.id}/${user.avatar}.png?size=128`
    : null

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={
        stage === 'success'
          ? 'Connected!'
          : stage === 'scanned' || stage === 'confirming'
            ? 'Confirm on Mobile'
            : 'Log in with Discord QR'
      }
      icon={
        stage === 'success' ? (
          <CheckCircle2 className="size-8 text-tertiary" />
        ) : (
          <QrCode className="size-8 text-primary" />
        )
      }
      actions={
        <div className="flex gap-2">
          {stage === 'error' && (
            <Button variant="tonal" icon={<RefreshCw className="size-4" />} onClick={startSession}>
              Retry
            </Button>
          )}
          <Button variant="text" onClick={onClose}>
            {stage === 'success' ? 'Close' : 'Cancel'}
          </Button>
        </div>
      }
    >
      <div className="flex flex-col items-center text-center py-2">
        {/* Loading state */}
        {stage === 'connecting' && (
          <div className="flex flex-col items-center justify-center py-10 gap-3">
            <Loader2 className="size-10 animate-spin text-primary" />
            <p className="type-body-md text-on-surface-variant">Connecting to Discord Remote Auth…</p>
          </div>
        )}

        {/* QR Code display */}
        {stage === 'qr' && (
          <div className="flex flex-col items-center gap-4">
            <div className="p-3 bg-white rounded-xl shadow-md3-1 flex items-center justify-center">
              <canvas ref={canvasRef} className="size-[220px]" />
            </div>

            <div className="flex flex-col items-center gap-2 max-w-xs text-[13px] text-on-surface-variant">
              <div className="flex items-center gap-2 font-medium text-on-surface">
                <Smartphone className="size-4 text-primary" />
                <span>Scan with the Discord mobile app:</span>
              </div>
              <ol className="text-left list-decimal list-inside space-y-1 text-on-surface-variant/90 leading-normal text-[12px]">
                <li>Open <strong>Discord</strong> on your phone</li>
                <li>Go to <strong>User Settings → Scan QR Code</strong></li>
                <li>Point your camera at this screen</li>
              </ol>
            </div>
          </div>
        )}

        {/* Scanned / Confirming state */}
        {(stage === 'scanned' || stage === 'confirming') && (
          <div className="flex flex-col items-center gap-4 py-6">
            <div className="relative">
              {avatarUrl ? (
                <img
                  src={avatarUrl}
                  alt={user?.username ?? 'Discord User'}
                  className="size-20 rounded-full border-2 border-primary shadow-md"
                />
              ) : (
                <div className="size-20 rounded-full bg-surface-highest flex items-center justify-center text-2xl font-bold text-primary">
                  {user?.username?.charAt(0).toUpperCase() ?? 'D'}
                </div>
              )}
              <span className="absolute bottom-0 right-0 size-5 rounded-full bg-emerald-500 border-2 border-surface" />
            </div>

            <div className="flex flex-col items-center gap-1">
              <h3 className="type-title-md font-semibold text-on-surface">
                @{user?.username ?? 'Discord User'}
              </h3>
              <p className="type-body-sm text-primary animate-pulse flex items-center gap-1.5 font-medium">
                <Loader2 className="size-3.5 animate-spin" /> Tap “Yes, log me in” on your phone
              </p>
            </div>
          </div>
        )}

        {/* Success state */}
        {stage === 'success' && (
          <div className="flex flex-col items-center gap-3 py-6">
            <div className="size-16 rounded-full bg-tertiary/20 text-tertiary flex items-center justify-center animate-bounce">
              <CheckCircle2 className="size-10" />
            </div>
            <p className="type-body-md font-medium text-on-surface">
              {user ? `Connected as @${user.username}!` : 'Discord Connected!'}
            </p>
            <p className="type-body-sm text-on-surface-variant">
              Rich Presence is now active and ready to stream.
            </p>
          </div>
        )}

        {/* Error state */}
        {stage === 'error' && (
          <div className="flex flex-col items-center gap-3 py-6">
            <div className="size-14 rounded-full bg-error/20 text-error flex items-center justify-center">
              <AlertCircle className="size-8" />
            </div>
            <p className="type-body-md font-medium text-error">Authentication Failed</p>
            <p className="type-body-sm text-on-surface-variant max-w-xs">
              {errorMsg || 'Could not complete Discord QR login. Please try again.'}
            </p>
          </div>
        )}
      </div>
    </Dialog>
  )
}
