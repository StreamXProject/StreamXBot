import { API_ENDPOINTS } from './endpoints'
import { http } from './client'

export interface HealthResult {
  ok: boolean
  latencyMs: number
  version?: string
  detail?: string
}

export async function checkHealth(baseUrl?: string, timeoutMs = 6000): Promise<HealthResult> {
  const t0 = performance.now()
  try {
    const data = await http.get<Record<string, unknown>>(API_ENDPOINTS.HEALTH, { baseUrl, timeoutMs, anonymous: true, noDedupe: true })
    return {
      ok: true,
      latencyMs: Math.round(performance.now() - t0),
      version: typeof data?.version === 'string' ? data.version : undefined,
    }
  } catch (err) {
    return { ok: false, latencyMs: Math.round(performance.now() - t0), detail: (err as Error).message }
  }
}
