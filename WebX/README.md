# WebX — web client for StreamX

React 19 · Vite 8 · TypeScript · TanStack Router/Query · Zustand · Tailwind 4 · Material Design 3 tokens.

```bash
npm install
npm run dev        # http://localhost:5173  (point Settings → Server at your StreamX API)
npm run build      # tsc + vite build → dist/  (served by Api/main.py when placed at WebX/dist)
npm run test       # vitest
npm run typecheck  # tsc --noEmit
```

## What changed in this revision

### Design system (MD3)
- `src/index.css` — full `--md-sys-*` token set (color roles, shape scale, type scale, motion easings, state-layer opacities) mapped to Tailwind v4 utilities (`bg-primary`, `text-on-surface-variant`, `bg-surface-low`, `rounded-lg`, `ease-emphasized`…).
- `src/components/md3/` — Button (filled/tonal/outlined/text/elevated), IconButton, Switch, Slider, Card, ListItem, SegmentedButton, Chip, TextField, Dialog, Menu, Tabs, Progress, Divider. All token-driven; no hard-coded colors anywhere in the app.

### Theme engine (`src/theme/`)
- A theme is a small declarative `ThemeDefinition` (seed color, palette variant, contrast, typeface, shape scale, effects, optional overrides). `scheme.ts` expands it into complete light + dark MD3 schemes with the official `@material/material-color-utilities` (HCT).
- **Modular**: drop a file in `src/theme/themes/*.ts` → auto-registered. Ships with WebX (default crimson), Material Baseline, Tidal, Moss, Amber Dusk, Graphite OLED, Paper.
- User themes: create/edit in-app (live preview), duplicate, import/export `.webx-theme.json` (zod-validated), persisted in localStorage.
- Light / Dark / System mode, reduced-motion preference, **color-from-artwork** ("Material You" from the playing cover).

### Endpoint integration (ported from StreamXWeb, verified against `Api/routers/*.py`)
- `src/api/client.ts` — single HTTP client: normalized base URL, Bearer + `X-Auth-Token`, cookies, GET de-duplication, timeouts, `ApiError`, 401 → session-expired event.
- `src/api/endpoints.ts` — every route in use.
- **Streaming fixed**: `<audio>` can't send headers, so stream URLs now carry `?token=`; ALAC → FLAC transcode when the browser can't decode; `/warm` before transcoded plays and prefetch of the next track.
- **Lyrics fixed**: LRC parser for the plain-text/`{lyrics}` responses; synced + unsynced rendering.
- Favourites, playlists (create/rename/delete/add/remove), history, top-played, followed artists, `/auth/me`, setup status, owner password setup/change, credentials, shared playlists, topics, jam API.
- Demo mode (Settings → Server) uses the bundled sample catalog instead of silently faking data on errors; every page has real loading / empty / error states.

### Settings → sub-settings (`src/routes/settings/*`)
List-detail layout (rail on wide windows, list → page on compact): **Appearance**, Playback, Server & endpoints (test connection, recent servers, endpoint reference), Account, Library & data, Keyboard shortcuts, About.

### Full-screen player performance
Root cause of the desktop lag: a full-viewport `filter: blur(40px)` layer + the whole tree mounting through `AnimatePresence` on every open, while lyrics and the scrubber re-rendered React at 10 Hz.
- Player is mounted once; open/close is a pure CSS transform on a `contain: paint` layer.
- Ambient backdrop blurs a 128 px image and scales it on the GPU (crossfades between covers).
- Scrubber writes to the DOM directly from the engine; lyrics re-render only when the active line index changes (binary search).
- The shell behind is made `inert` and hidden after the transition so it stops painting.
- Measured with Playwright: open ≈130 ms, 0 dropped frames over 60 sampled.
- New layout: artwork + controls left, Lyrics / Up next / Details pane right; on phones a segmented switch, drag-down to dismiss.

### Added features
Ctrl+K command palette (search + actions), track context menu (play next, queue, favourite, playlist, radio, album/artist, download, share/copy link), add-to-playlist dialog, toasts, keyboard shortcuts (`?`), sleep timer, playback speed, resume-where-you-left-off (queue + position), Media Session actions (lock-screen controls), server health indicator, first-run server picker + owner setup on the login screen, quality badges (FLAC/ALAC/Hi-Res), topics browsing, deep links `/track/:id`, public `/share/playlist/:id`, PWA meta.

## Layout
```
src/
  api/          client, endpoints, stream, lyrics, favourites, playlists, auth, browse, search, albums, artists, health, jam
  audio/        AudioEngine (resolver + warm + prefetch + MediaSession), QueueManager
  components/   md3/ (design system) · common/ · player/ · shell/ · overlays/ · settings/ · auth/
  hooks/        useQueries, useKeyboardShortcuts, useMediaQuery, useServerStatus
  routes/       file-based routes (settings/* are sub-settings)
  stores/       auth, settings (persisted), player, progress, queue, library (server-backed), ui
  theme/        tokens, scheme, registry, apply, themeStore, themes/*
  test/         vitest suites (engine, queue, theme, lyrics, client)
```
