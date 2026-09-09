import { z } from 'zod'
import { TrackSchema } from './track'
import { AlbumSchema } from './album'

export const ArtistCoreSchema = z.object({
  id: z.string().optional(),
  _id: z.string().optional(),
  name: z.string(),
  avatar_url: z.string().optional().nullable(),
  cover_url: z.string().optional().nullable(),
  monthly_listeners: z.number().optional().nullable(),
  followers: z.number().optional().nullable(),
  bio: z.string().optional().nullable(),
})

export const ArtistSchema = ArtistCoreSchema.transform((data) => ({
  id: data.id || data._id || '',
  name: data.name,
  avatar_url: data.avatar_url || data.cover_url || null,
  monthly_listeners: data.monthly_listeners ?? data.followers ?? null,
  bio: data.bio || null,
}))

export type Artist = z.infer<typeof ArtistSchema>

export const ArtistDetailSchema = ArtistCoreSchema.extend({
  top_tracks: z.array(TrackSchema).default([]),
  all_tracks: z.array(TrackSchema).default([]),
  albums: z.array(AlbumSchema).default([]),
}).transform((data) => ({
  id: data.id || data._id || '',
  name: data.name,
  avatar_url: data.avatar_url || data.cover_url || null,
  monthly_listeners: data.monthly_listeners ?? data.followers ?? null,
  bio: data.bio || null,
  top_tracks: data.top_tracks,
  all_tracks: data.all_tracks,
  albums: data.albums,
}))

export type ArtistDetail = z.infer<typeof ArtistDetailSchema>

