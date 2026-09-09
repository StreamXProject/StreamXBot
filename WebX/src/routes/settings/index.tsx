import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useIsDesktop } from '@/hooks/useMediaQuery'

export const Route = createFileRoute('/settings/')({
  component: SettingsIndex,
})

/** On wide windows the list-detail layout needs a default detail: Appearance. */
function SettingsIndex() {
  const isDesktop = useIsDesktop()
  const navigate = useNavigate()
  useEffect(() => {
    if (isDesktop) navigate({ to: '/settings/appearance', replace: true })
  }, [isDesktop, navigate])
  return null
}
