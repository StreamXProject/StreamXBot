import { API_ENDPOINTS } from '@/api/endpoints'
import { http } from '@/api/client'
import type { RecapAvailable, RecapPeriodType, RecapPublicSummary, RecapSnapshot } from './types'

/** Client UTC offset in minutes (east-positive), used so days/hours bucket in local time. */
export const tzOffsetMin = () => -new Date().getTimezoneOffset()

export async function fetchAvailableRecaps(signal?: AbortSignal): Promise<RecapAvailable[]> {
  const data = await http.get<{ items?: RecapAvailable[] }>(API_ENDPOINTS.ME_RECAPS, { params: { tz: tzOffsetMin() }, signal })
  return data?.items ?? []
}

export async function fetchRecap(type: RecapPeriodType, period: string, opts?: { refresh?: boolean }, signal?: AbortSignal): Promise<RecapSnapshot> {
  return http.get<RecapSnapshot>(API_ENDPOINTS.ME_RECAP(type, period), { params: { tz: tzOffsetMin(), ...(opts?.refresh ? { refresh: 'true' } : {}) }, signal, noDedupe: Boolean(opts?.refresh) })
}

export async function shareRecap(type: RecapPeriodType, period: string): Promise<{ token: string; url: string }> {
  const data = await http.post<{ token: string }>(`${API_ENDPOINTS.ME_RECAP_SHARE(type, period)}?tz=${tzOffsetMin()}`, {})
  return { token: data.token, url: `${window.location.origin}/recap/share/${data.token}` }
}

export async function fetchMyRecapShares(signal?: AbortSignal): Promise<{ token: string; type: RecapPeriodType; period: string; label: string | null; created_at: number }[]> {
  const data = await http.get<{ items?: never[] }>(API_ENDPOINTS.ME_RECAP_SHARES, { signal })
  return data?.items ?? []
}

export async function revokeRecapShare(token: string): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_RECAP_SHARE_REVOKE(token))
}

export async function deleteRecapData(): Promise<void> {
  await http.delete(API_ENDPOINTS.ME_RECAP_DATA)
}

export async function fetchPublicRecap(token: string, signal?: AbortSignal): Promise<RecapPublicSummary> {
  return http.get<RecapPublicSummary>(API_ENDPOINTS.RECAP_PUBLIC_SHARE(token), { signal, anonymous: true })
}

export interface ListeningEventPayload {
  id: string
  track_id: string
  played_at: number
  started_at: number
  played_ms: number
  duration_ms: number
  completed: boolean
  skipped: boolean
  source: string
  session_id?: string
}

export async function postListeningEvents(events: ListeningEventPayload[]): Promise<void> {
  await http.post(API_ENDPOINTS.ME_LISTENING_EVENTS, { events })
}
