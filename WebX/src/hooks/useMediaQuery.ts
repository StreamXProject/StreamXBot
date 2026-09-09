import { useEffect, useState } from 'react'

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => (typeof window !== 'undefined' ? window.matchMedia(query).matches : false))
  useEffect(() => {
    const mq = window.matchMedia(query)
    const handler = (e: MediaQueryListEvent) => setMatches(e.matches)
    setMatches(mq.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [query])
  return matches
}

/** MD3 window size classes */
export function useWindowClass(): 'compact' | 'medium' | 'expanded' | 'large' {
  const medium = useMediaQuery('(min-width: 600px)')
  const expanded = useMediaQuery('(min-width: 840px)')
  const large = useMediaQuery('(min-width: 1200px)')
  if (large) return 'large'
  if (expanded) return 'expanded'
  if (medium) return 'medium'
  return 'compact'
}

export const useIsDesktop = () => useMediaQuery('(min-width: 840px)')
