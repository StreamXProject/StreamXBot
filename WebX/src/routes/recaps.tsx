import React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CalendarDays, CalendarRange, Calendar, ChevronRight, FlaskConical } from 'lucide-react'
import { RecapIcon } from '@/components/common/RecapIcon'
import { Button } from '@/components/md3'
import { DEMO_TYPE } from '@/features/recap/mock'
import { PageContainer } from '@/components/common/PageContainer'
import { SectionHeader } from '@/components/common/SectionHeader'
import { EmptyState } from '@/components/common/EmptyState'
import { ErrorState } from '@/components/common/ErrorState'
import { Skeleton } from '@/components/common/Skeleton'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { fetchAvailableRecaps } from '@/features/recap/api'
import type { RecapAvailable, RecapPeriodType } from '@/features/recap/types'
import { cn } from '@/lib/cn'

export const Route = createFileRoute('/recaps')({
  component: RecapsPage,
})

const ICON: Record<RecapPeriodType, React.FC<{ className?: string }>> = { weekly: CalendarDays, monthly: CalendarRange, yearly: Calendar }
const TITLE: Record<RecapPeriodType, string> = { weekly: 'Weekly', monthly: 'Monthly', yearly: 'Yearly' }

function RecapCard({ r, featured }: { r: RecapAvailable; featured?: boolean }) {
  const navigate = useNavigate()
  const Icon = ICON[r.type]
  return (
    <button
      onClick={() => navigate({ to: '/recap/$type/$period', params: { type: r.type, period: r.period } })}
      className={cn(
        'state-layer group text-left rounded-2xl p-4 flex items-center gap-4 w-full',
        featured ? 'bg-primary-container text-on-primary-container min-h-28' : 'bg-surface-low text-on-surface'
      )}
    >
      <span className={cn('size-12 rounded-xl flex items-center justify-center shrink-0', featured ? 'bg-on-primary-container/15' : 'bg-surface-highest text-on-surface-variant')}>
        {featured ? <RecapIcon className="size-6" /> : <Icon className="size-5" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className={cn('block type-label-md', featured ? 'opacity-80' : 'text-on-surface-variant')}>{TITLE[r.type]}{r.ongoing ? ' · in progress' : ''}</span>
        <span className={cn('block truncate', featured ? 'type-headline-sm' : 'type-title-md')}>{r.label}</span>
      </span>
      <ChevronRight className={cn('size-5 shrink-0 transition-transform group-hover:translate-x-0.5', featured ? 'opacity-80' : 'text-on-surface-variant')} />
    </button>
  )
}

function RecapsPage() {
  const navigate = useNavigate()
  const token = useAuthStore((s) => s.token)
  const user = useAuthStore((s) => s.user)
  const isUser = sessionKind(token, user) === 'user'
  const q = useQuery({ queryKey: ['recaps', 'available'], queryFn: ({ signal }) => fetchAvailableRecaps(signal), enabled: isUser, staleTime: 5 * 60_000 })

  const SamplePicker = (
    <section className="rounded-2xl bg-surface-low p-4 flex flex-col sm:flex-row sm:items-center gap-3">
      <span className="size-10 rounded-xl bg-surface-highest text-on-surface-variant flex items-center justify-center shrink-0"><FlaskConical className="size-5" /></span>
      <div className="min-w-0 flex-1">
        <p className="type-title-sm text-on-surface">Preview a sample recap</p>
        <p className="type-body-sm text-on-surface-variant">Dummy numbers on your recent tracks · shows the full story flow</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {(['weekly', 'monthly', 'yearly'] as RecapPeriodType[]).map((t) => (
          <Button key={t} variant="tonal" size="sm" onClick={() => navigate({ to: '/recap/$type/$period', params: { type: DEMO_TYPE, period: t } })}>{TITLE[t]}</Button>
        ))}
      </div>
    </section>
  )

  if (!isUser) {
    return (
      <PageContainer className="space-y-6">
        <EmptyState icon={<RecapIcon />} title="Recaps need an account" description="Listening history is tracked per account so your recap follows you across devices." />
        {SamplePicker}
      </PageContainer>
    )
  }
  if (q.isError) return <PageContainer><ErrorState error={q.error} onRetry={() => q.refetch()} /></PageContainer>

  const items = q.data ?? []
  const latestMonth = items.find((r) => r.type === 'monthly' && !r.ongoing) ?? items.find((r) => r.type === 'monthly')
  const rest = items.filter((r) => r !== latestMonth)
  const byType = (t: RecapPeriodType) => rest.filter((r) => r.type === t)

  return (
    <PageContainer className="space-y-8">
      <div>
        <p className="type-label-lg text-primary">Recaps</p>
        <h1 className="type-headline-lg text-on-surface mt-1">Your listening, in stories</h1>
        <p className="type-body-md text-on-surface-variant mt-1">Weekly, monthly and yearly recaps built from what you actually played.</p>
      </div>

      {q.isLoading ? (
        <div className="space-y-3"><Skeleton className="h-28 rounded-2xl" /><Skeleton className="h-20 rounded-2xl" /><Skeleton className="h-20 rounded-2xl" /></div>
      ) : items.length === 0 ? (
        <>
          <EmptyState icon={<RecapIcon />} title="No recaps yet" description="Play some music — your first weekly recap appears after your first listens." />
          {SamplePicker}
        </>
      ) : (
        <>
          {latestMonth && <RecapCard r={latestMonth} featured />}
          {(['weekly', 'monthly', 'yearly'] as RecapPeriodType[]).map((t) =>
            byType(t).length ? (
              <section key={t}>
                <SectionHeader title={TITLE[t]} />
                <div className="grid sm:grid-cols-2 gap-3">
                  {byType(t).map((r) => <RecapCard key={`${r.type}-${r.period}`} r={r} />)}
                </div>
              </section>
            ) : null
          )}
          {SamplePicker}
        </>
      )}
    </PageContainer>
  )
}
