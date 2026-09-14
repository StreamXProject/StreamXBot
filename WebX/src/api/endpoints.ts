/**
 * Every backend route WebX talks to. Verified against Api/routers/*.py.
 * Paths are relative to the configured server base URL.
 */
const enc = encodeURIComponent

export const API_ENDPOINTS = {
  // Health
  HEALTH: '/health',

  // Auth
  AUTH_LOGIN: '/auth/login',
  AUTH_REGISTER: '/auth/register',
  AUTH_VALIDATE: '/auth/validate',
  AUTH_PASSWORD: '/auth/password',
  AUTH_PASSWORD_CHANGE: '/auth/password/change',
  AUTH_CREDENTIALS: '/auth/credentials',
  AUTH_SETUP_STATUS: '/auth/setup/status',
  AUTH_SETUP: '/auth/setup',
  AUTH_COOKIE: '/auth/cookie',
  AUTH_LOGOUT: '/auth/logout',
  AUTH_ME: '/auth/me',
  AUTH_TG_LOGIN: '/auth/tg/login',
  AUTH_TELEGRAM_START: '/auth/telegram/start',
  AUTH_TELEGRAM_CONFIG: '/auth/telegram/config',
  AUTH_TELEGRAM_WIDGET: '/auth/telegram/widget',
  AUTH_TELEGRAM_VALIDATE_TOKEN: '/auth/telegram/validate-token',
  AUTH_INTEGRATIONS: '/auth/integrations',
  WEBAPP_VERIFY: '/webapp/verify',

  // Catalog
  BROWSE: '/browse',
  SEARCH: '/search',
  SEARCH_ARTISTS: '/search/artists',
  TRACKS_SHUFFLE: '/tracks/shuffle',
  TRACKS_RANDOM: '/tracks/random',
  TRACK_DETAILS: (id: string) => `/tracks/${enc(id)}`,
  TRACK_STREAM: (id: string) => `/tracks/${enc(id)}/stream`,
  TRACK_DOWNLOAD: (id: string) => `/tracks/${enc(id)}/download`,
  TRACK_WARM: (id: string) => `/tracks/${enc(id)}/warm`,
  TRACK_LYRICS: (id: string) => `/tracks/${enc(id)}/lyrics`,
  ALBUMS: '/albums',
  ALBUM_DETAILS: (id: string) => `/albums/${enc(id)}`,
  ALBUM_TRACKS: (id: string) => `/albums/${enc(id)}/tracks`,
  ARTISTS: '/artists',
  ARTIST_DETAILS: (id: string) => `/artists/${enc(id)}`,
  ARTIST_TRACKS: (id: string) => `/artists/${enc(id)}/tracks`,
  TOPICS: '/topics',
  TOPIC_TRACKS: (name: string) => `/topics/${enc(name)}/tracks`,
  CHANNEL_IDS: '/channelids',

  // Curated
  PLAYLISTS_AVAILABLE: '/playlists/available',
  DAILY_PLAYLIST: (key: string) => `/daily-playlist/${enc(key)}`,

  // User (authenticated)
  ME_FAVOURITES: '/me/favourites',
  ME_FAVOURITE: (trackId: string) => `/me/favourites/${enc(trackId)}`,
  ME_FAVOURITE_IDS: '/me/favourites/ids',
  ME_TOP_PLAYED: '/me/top-played',
  ME_HISTORY: '/me/history',

  // Recaps
  ME_LISTENING_EVENTS: '/me/listening-events',
  ME_RECAPS: '/me/recaps',
  ME_RECAP: (type: string, period: string) => `/me/recaps/${enc(type)}/${enc(period)}`,
  ME_RECAP_SHARE: (type: string, period: string) => `/me/recaps/${enc(type)}/${enc(period)}/share`,
  ME_RECAP_SHARES: '/me/recaps/shares',
  ME_RECAP_SHARE_REVOKE: (token: string) => `/me/recaps/shares/${enc(token)}`,
  ME_RECAP_DATA: '/me/recaps/data',
  RECAP_PUBLIC_SHARE: (token: string) => `/recaps/share/${enc(token)}`,
  ME_PLAYLISTS: '/me/playlists',
  ME_PLAYLIST: (id: string) => `/me/playlists/${enc(id)}`,
  ME_PLAYLIST_TRACKS: (id: string) => `/me/playlists/${enc(id)}/tracks`,
  ME_PLAYLIST_TRACK: (id: string, trackId: string) => `/me/playlists/${enc(id)}/tracks/${enc(trackId)}`,
  ME_ALBUMS: '/me/albums',
  ME_ARTIST_FAVOURITES: '/me/artists/favourites',
  ME_ARTIST_FAVOURITE: (id: string) => `/me/artists/favourites/${enc(id)}`,
  ME_ARTIST_FAVOURITE_IDS: '/me/artists/favourites/ids',

  // Share (public)
  SHARE_PLAYLIST: (id: string) => `/share/playlists/${enc(id)}`,

  // Jam sessions
  JAM_CREATE: '/jam/create',
  JAM: (id: string) => `/jam/${enc(id)}`,
  JAM_JOIN: (id: string) => `/jam/${enc(id)}/join`,
  JAM_LEAVE: (id: string) => `/jam/${enc(id)}/leave`,
  JAM_PLAY: (id: string) => `/jam/${enc(id)}/play`,
  JAM_PAUSE: (id: string) => `/jam/${enc(id)}/pause`,
  JAM_SEEK: (id: string) => `/jam/${enc(id)}/seek`,
  JAM_NEXT: (id: string) => `/jam/${enc(id)}/next`,
  JAM_QUEUE_ADD: (id: string) => `/jam/${enc(id)}/queue/add`,
  JAM_QUEUE_REORDER: (id: string) => `/jam/${enc(id)}/queue/reorder`,
  JAM_SETTINGS: (id: string) => `/jam/${enc(id)}/settings`,
} as const
