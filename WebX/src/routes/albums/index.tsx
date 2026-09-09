import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/albums/')({
  loader: () => {
    throw redirect({ to: '/explore/albums' })
  },
})
