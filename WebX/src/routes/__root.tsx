import { createRootRoute } from '@tanstack/react-router'
import { AppShell } from '@/components/shell/AppShell'

export const Route = createRootRoute({
  component: AppShell,
})
