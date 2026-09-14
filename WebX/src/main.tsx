import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { routeTree } from './routeTree.gen'
import './index.css'
// Apply the persisted theme before the first paint
import './theme/themeStore'
import { ApiError } from './api/client'
import { registerServiceWorker } from './hooks/usePwa'
import { startScrobbler } from './services/lastfm'
import { startDiscordPresence } from './services/discordPresence'
import { startListeningRecorder } from './services/listening'
import { equalizer } from './audio/Equalizer'
import { initAudioUnlock } from './audio/audioUnlock'
import { useSettingsStore } from './stores/settingsStore'

// Register global gesture unlock for browser Autoplay Policy
initAudioUnlock()

// Background services (all idempotent, all opt-in via settings)
startScrobbler()
startDiscordPresence()
startListeningRecorder()
{
  const s = useSettingsStore.getState()
  equalizer.setGains(s.eqGains)
  equalizer.setPreamp(s.eqPreamp)
  if (s.eqEnabled) equalizer.setEnabled(true)
}
if (import.meta.env.PROD) registerServiceWorker()

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        // Never retry auth failures or 4xx; retry network blips once
        if (error instanceof ApiError && (error.isAuth || (error.status >= 400 && error.status < 500))) return false
        return failureCount < 1
      },
    },
  },
})

const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
  scrollRestoration: false,
  context: { queryClient },
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const rootElement = document.getElementById('root')
if (rootElement && !rootElement.innerHTML) {
  ReactDOM.createRoot(rootElement).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>
  )
}
