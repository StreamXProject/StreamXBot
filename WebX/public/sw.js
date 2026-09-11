/* WebX service worker — app shell + asset caching. Audio streams and API JSON are never cached. */
const VERSION = 'webx-v7'
const SHELL_CACHE = `${VERSION}-shell`
const ASSET_CACHE = `${VERSION}-assets`
const IMAGE_CACHE = `${VERSION}-images`
const FONT_CACHE = `${VERSION}-fonts`
const KEEP = new Set([SHELL_CACHE, ASSET_CACHE, IMAGE_CACHE, FONT_CACHE])
const IMAGE_LIMIT = 300

const SHELL = ['/', '/index.html', '/manifest.json', '/favicon.svg', '/icons/icon-192.png', '/icons/icon-512.png']

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(SHELL_CACHE).then((c) => Promise.allSettled(SHELL.map((u) => c.add(u)))).then(() => self.skipWaiting())
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const names = await caches.keys()
      await Promise.all(names.filter((n) => !KEEP.has(n)).map((n) => caches.delete(n)))
      if (self.registration.navigationPreload) await self.registration.navigationPreload.enable()
      await self.clients.claim()
    })()
  )
})

self.addEventListener('message', (event) => {
  if (event.data === 'SKIP_WAITING') self.skipWaiting()
  if (event.data === 'CLEAR_CACHES') event.waitUntil(caches.keys().then((ks) => Promise.all(ks.map((k) => caches.delete(k)))))
})

const isStream = (url) => /\/tracks\/[^/]+\/(stream|download|warm)(\/|$|\?)/.test(url.pathname)
const isApi = (url, req) =>
  req.headers.get('accept')?.includes('application/json') ||
  /^\/(me|auth|browse|tracks|search|albums|artists|topics|share|jam|health|lyrics|playlists|friends|presence|notifications)(\/|$)/.test(url.pathname)
const isHashedAsset = (url) => url.pathname.startsWith('/assets/')
const isImage = (req) => req.destination === 'image'
const isFont = (url, req) => req.destination === 'font' || /fonts\.(googleapis|gstatic)\.com$/.test(url.hostname)

async function trimCache(name, limit) {
  const cache = await caches.open(name)
  const keys = await cache.keys()
  if (keys.length <= limit) return
  await Promise.all(keys.slice(0, keys.length - limit).map((k) => cache.delete(k)))
}

async function cacheFirst(req, name) {
  const cache = await caches.open(name)
  const hit = await cache.match(req)
  if (hit) return hit
  const res = await fetch(req)
  if (res.ok) cache.put(req, res.clone())
  return res
}

async function staleWhileRevalidate(req, name, limit) {
  const cache = await caches.open(name)
  const hit = await cache.match(req)
  const network = fetch(req)
    .then((res) => {
      if (res.ok || res.type === 'opaque') {
        cache.put(req, res.clone()).then(() => limit && trimCache(name, limit))
      }
      return res
    })
    .catch(() => hit)
  return hit || network
}

async function navigation(event) {
  try {
    const preload = await event.preloadResponse
    if (preload) return preload
    const res = await fetch(event.request)
    const cache = await caches.open(SHELL_CACHE)
    cache.put('/index.html', res.clone())
    return res
  } catch {
    const cache = await caches.open(SHELL_CACHE)
    return (await cache.match('/index.html')) || (await cache.match('/')) || Response.error()
  }
}

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return

  // Never touch audio (range requests) or API calls — always live.
  if (isStream(url) || (url.origin === self.location.origin && isApi(url, req))) return

  if (req.mode === 'navigate') {
    event.respondWith(navigation(event))
    return
  }
  if (url.origin === self.location.origin && isHashedAsset(url)) {
    event.respondWith(cacheFirst(req, ASSET_CACHE))
    return
  }
  if (isFont(url, req)) {
    event.respondWith(staleWhileRevalidate(req, FONT_CACHE))
    return
  }
  if (isImage(req)) {
    event.respondWith(staleWhileRevalidate(req, IMAGE_CACHE, IMAGE_LIMIT))
  }
})
