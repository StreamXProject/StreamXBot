import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/artists/')({
  loader: () => {
    throw redirect({ to: '/explore/artists' })
  },
})
