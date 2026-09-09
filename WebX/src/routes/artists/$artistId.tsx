import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/artists/$artistId')({
  loader: ({ params }) => {
    throw redirect({ to: '/artist/$artistId', params: { artistId: params.artistId } })
  },
})
