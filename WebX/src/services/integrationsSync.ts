import { useSettingsStore, type DiscordMode } from '@/stores/settingsStore'
import { updateUserIntegrations, type UserIntegrations } from '@/api/auth'
import { useAuthStore, sessionKind } from '@/stores/authStore'

let _isHydrating = false

/**
 * Hydrates local settings store with integrations saved in MongoDB user profile.
 * Only applies if current session is a full user account (not guest).
 */
export function hydrateIntegrationsFromUser(integrations?: UserIntegrations): void {
  if (!integrations) return
  const token = useAuthStore.getState().token
  const user = useAuthStore.getState().user
  if (sessionKind(token, user) !== 'user') return

  _isHydrating = true
  try {
    const s = useSettingsStore.getState()

    // 1. Hydrate Discord
    if (integrations.discord) {
      const d = integrations.discord
      if (typeof d.token === 'string' && d.token.trim()) {
        s.set('discordUserToken', d.token.trim())
      }
      if (typeof d.enabled === 'boolean') {
        s.set('discordEnabled', d.enabled)
      }
      if (d.mode === 'gateway' || d.mode === 'daemon') {
        s.set('discordMode', d.mode as DiscordMode)
      }
      if (typeof d.client_id === 'string' && d.client_id.trim()) {
        s.set('discordClientId', d.client_id.trim())
      }
      if (typeof d.daemon_url === 'string' && d.daemon_url.trim()) {
        s.set('discordDaemonUrl', d.daemon_url.trim())
      }
      if (typeof d.show_artwork === 'boolean') {
        s.set('discordShowArtwork', d.show_artwork)
      }
    }

    // 2. Hydrate Last.fm
    if (integrations.lastfm) {
      const l = integrations.lastfm
      if (typeof l.session_key === 'string' && l.session_key.trim()) {
        s.set('lastfmSessionKey', l.session_key.trim())
      }
      if (typeof l.username === 'string' && l.username.trim()) {
        s.set('lastfmUsername', l.username.trim())
      }
      if (typeof l.api_key === 'string') {
        s.set('lastfmApiKey', l.api_key.trim())
      }
      if (typeof l.api_secret === 'string') {
        s.set('lastfmApiSecret', l.api_secret.trim())
      }
      if (typeof l.enabled === 'boolean') {
        s.set('lastfmEnabled', l.enabled)
      }
      if (typeof l.scrobble_at === 'number' && l.scrobble_at >= 0.1 && l.scrobble_at <= 0.95) {
        s.set('lastfmScrobbleAt', l.scrobble_at)
      }
      if (typeof l.now_playing === 'boolean') {
        s.set('lastfmNowPlaying', l.now_playing)
      }
    }
  } finally {
    _isHydrating = false
  }
}

/**
 * Saves current local Discord and Last.fm settings to MongoDB for the current user.
 */
export async function saveIntegrationsToServer(customPatch?: Partial<UserIntegrations>): Promise<void> {
  if (_isHydrating) return
  const token = useAuthStore.getState().token
  const user = useAuthStore.getState().user
  if (sessionKind(token, user) !== 'user') return

  const s = useSettingsStore.getState()

  const payload: UserIntegrations = {
    discord: {
      enabled: s.discordEnabled,
      token: s.discordUserToken,
      mode: s.discordMode,
      client_id: s.discordClientId,
      daemon_url: s.discordDaemonUrl,
      show_artwork: s.discordShowArtwork,
      ...(customPatch?.discord || {}),
    },
    lastfm: {
      enabled: s.lastfmEnabled,
      api_key: s.lastfmApiKey,
      api_secret: s.lastfmApiSecret,
      session_key: s.lastfmSessionKey,
      username: s.lastfmUsername,
      scrobble_at: s.lastfmScrobbleAt,
      now_playing: s.lastfmNowPlaying,
      ...(customPatch?.lastfm || {}),
    },
  }

  try {
    await updateUserIntegrations(payload)
  } catch (err) {
    console.warn('[IntegrationsSync] Failed to persist integrations to MongoDB:', err)
  }
}
