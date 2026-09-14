import React, { useMemo } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { RecapPlayer } from '@/features/recap/components/RecapPlayer'
import { fetchRecap } from '@/features/recap/api'
import type { RecapPeriodType } from '@/features/recap/types'
import { buildSampleSnapshot, DEMO_TYPE } from '@/features/recap/mock'
import { ErrorState } from '@/components/common/ErrorState'
import { Button } from '@/components/md3'

export const Route = createFileRoute('/recap/$type/$period')({
  component: RecapRoute,
})

const TYPES: RecapPeriodType[] = ['weekly', 'monthly', 'yearly']

function RecapRoute() {
  const { type, period } = Route.useParams()
  const navigate = useNavigate()
  const isDemo = type === DEMO_TYPE
  const demoType: RecapPeriodType = TYPES.includes(period as RecapPeriodType) ? (period as RecapPeriodType) : 'monthly'
  const demo = useMemo(() => (isDemo ? buildSampleSnapshot(demoType) : null), [isDemo, demoType])
  const ptype = TYPES.includes(type as RecapPeriodType) ? (type as RecapPeriodType) : null
  const q = useQuery({
    queryKey: ['recaps', type, period],
    queryFn: ({ signal }) => fetchRecap(ptype!, period, undefined, signal),
    enabled: Boolean(ptype) && !isDemo,
    staleTime: 10 * 60_000,
  })

  if (demo) return <RecapPlayer snapshot={demo} />
  if (!ptype || q.isError) {
    return (
      <div className="fixed inset-0 z-50 bg-surface flex items-center justify-center p-6">
        <div className="max-w-sm w-full space-y-4 text-center">
          <ErrorState error={q.error ?? new Error('Unknown recap period')} onRetry={() => q.refetch()} />
          <Button variant="text" onClick={() => navigate({ to: '/recaps' })}>Back to recaps</Button>
        </div>
      </div>
    )
  }
  if (!q.data) {
    return (
      <div className="fixed inset-0 z-50 bg-surface flex flex-col items-center justify-center gap-4 text-on-surface-variant">
        <span className="size-10 border-[3px] border-primary/30 border-t-primary rounded-full animate-spin" />
        <p className="type-body-lg">Building your recap…</p>
      </div>
    )
  }
  return <RecapPlayer snapshot={q.data} />
}
