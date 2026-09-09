import { z } from 'zod'
import { TrackSchema } from './track'

export const AlbumCoreSchema = z.object({
  id: z.string().optional(),
  _id: z.string().optional(),
  title: z.string(),
  artist: z.string().default('Various Artists'),
  artist_id: z.string().optional().nullable(),
  cover_url: z.string().optional().nullable(),
  year: z.number().optional().nullable(),
  track_count: z.number().optional().default(0),
  tracks_count: z.number().optional(),
})

export const AlbumSchema = AlbumCoreSchema.transform((data) => ({
  id: data.id || data._id || '',
  title: data.title,
  artist: data.artist,
  artist_id: data.artist_id,
  cover_url: data.cover_url,
  year: data.year,
  track_count: data.track_count || data.tracks_count || 0,
}))

export type Album = z.infer<typeof AlbumSchema>

export const AlbumDetailSchema = AlbumCoreSchema.extend({
  tracks: z.array(TrackSchema).default([]),
}).transform((data) => ({
  id: data.id || data._id || '',
  title: data.title,
  artist: data.artist,
  artist_id: data.artist_id,
  cover_url: data.cover_url,
  year: data.year,
  track_count: data.track_count || data.tracks_count || 0,
  tracks: data.tracks,
}))

export type AlbumDetail = z.infer<typeof AlbumDetailSchema>
