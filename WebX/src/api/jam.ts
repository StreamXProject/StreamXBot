/** Jam sessions — listen together. Thin port of StreamXWeb/services/jamApi.ts */
import { API_ENDPOINTS } from './endpoints'
import { http } from './client'

export interface JamPlayback { track_id: string; position_sec: number; started_at: number; is_playing: boolean }
export interface JamSession {
  _id: string
  host_user_id: number
  playback: JamPlayback
  queue: string[]
  settings: { allow_seek?: boolean; allow_queue_edit?: boolean }
  members?: Array<{ user_id: number; role: string }>
}
export interface JamResponse { ok: boolean; jam: JamSession }

export const jamApi = {
  create: (p: { track_id: string; position_sec?: number; is_playing?: boolean; queue?: string[] }) =>
    http.post<JamResponse>(API_ENDPOINTS.JAM_CREATE, { position_sec: 0, is_playing: true, queue: [], settings: { allow_seek: false, allow_queue_edit: false }, ...p }),
  get: (id: string) => http.get<JamResponse>(API_ENDPOINTS.JAM(id), { noDedupe: true }),
  join: (id: string) => http.post<JamResponse>(API_ENDPOINTS.JAM_JOIN(id)),
  leave: (id: string) => http.post(API_ENDPOINTS.JAM_LEAVE(id)),
  play: (id: string) => http.post(API_ENDPOINTS.JAM_PLAY(id)),
  pause: (id: string) => http.post(API_ENDPOINTS.JAM_PAUSE(id)),
  seek: (id: string, position_sec: number) => http.post(API_ENDPOINTS.JAM_SEEK(id), { position_sec }),
  next: (id: string) => http.post(API_ENDPOINTS.JAM_NEXT(id)),
  addToQueue: (id: string, track_id: string, position: number | null = null) => http.post(API_ENDPOINTS.JAM_QUEUE_ADD(id), { track_id, position }),
  reorder: (id: string, queue: string[]) => http.post(API_ENDPOINTS.JAM_QUEUE_REORDER(id), { queue }),
}
