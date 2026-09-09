import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/albums/$albumId')({
  loader: ({ params }) => {
    throw redirect({ to: '/album/$albumId', params: { albumId: params.albumId } })
  },
})
