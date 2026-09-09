import React from 'react'
import { WifiOff, ServerCrash, Lock, RefreshCw, Settings } from 'lucide-react'
import { useNavigate } from '@tanstack/react-router'
import { ApiError } from '@/api/client'
import { Button } from '@/components/md3'
import { EmptyState } from './EmptyState'

export const ErrorState: React.FC<{ error: unknown; onRetry?: () => void; compact?: boolean }> = ({ error, onRetry, compact }) => {
  const navigate = useNavigate()
  const e = error as Partial<ApiError> & { message?: string }
  const isAuth = e instanceof ApiError && e.isAuth
  const isNetwork = e instanceof ApiError && e.isNetwork
  const icon = isAuth ? <Lock /> : isNetwork ? <WifiOff /> : <ServerCrash />
  const title = isAuth ? 'Sign in required' : isNetwork ? 'Can\u2019t reach the server' : 'Something went wrong'
  const desc = isAuth
    ? 'Your session is missing or expired.'
    : isNetwork
      ? 'Check that the StreamX server is running and the address in Settings is correct.'
      : e?.message || 'Unexpected error'

  return (
    <EmptyState
      compact={compact}
      icon={icon}
      title={title}
      description={desc}
      action={
        <>
          {onRetry && (
            <Button variant="tonal" icon={<RefreshCw />} onClick={onRetry}>
              Retry
            </Button>
          )}
          {isAuth ? (
            <Button variant="text" onClick={() => navigate({ to: '/login' })}>Sign in</Button>
          ) : (
            <Button variant="text" icon={<Settings />} onClick={() => navigate({ to: '/settings/server' })}>
              Server settings
            </Button>
          )}
        </>
      }
    />
  )
}
