import React, { useEffect, useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { fetchTrackById } from '@/api/tracks'
import { useQueueStore } from '@/stores/queueStore'
import { useUiStore } from '@/stores/uiStore'
import { PageContainer } from '@/components/common/PageContainer'
import { ErrorState } from '@/components/common/ErrorState'
import { CircularProgress } from '@/components/md3'

/** Deep link: /track/:id → play it and open the full player */
export const Route = createFileRoute('/track/$trackId')({
  component: TrackDeepLink,
})

function TrackDeepLink() {
  const { trackId } = Route.useParams()
  const navigate = useNavigate()
  const [error, setError] = useState<unknown>(null)
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const t = await fetchTrackById(trackId)
        if (cancelled) return
        await useQueueStore.getState().playTrackWithQueue([t], 0, { type: 'custom', title: 'Shared track' })
        useUiStore.getState().openFullPlayer()
        navigate({ to: '/', replace: true })
      } catch (e) {
        if (!cancelled) setError(e)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [trackId, navigate])
  return (
    <PageContainer className="flex items-center justify-center min-h-[50vh]">
      {error ? <ErrorState error={error} /> : <CircularProgress size={40} className="text-primary" />}
    </PageContainer>
  )
}
