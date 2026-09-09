import { z } from 'zod'

export const PlaylistSchema = z
  .object({
    playlist_id: z.string().optional(),
    id: z.string().optional(),
    name: z.string().default('Untitled'),
    thumbnails: z.array(z.string()).default([]),
    cover_url: z.string().optional().nullable(),
    normal_thumbnail: z.string().optional().nullable(),
    track_count: z.number().optional().nullable(),
    created_at: z.number().optional().nullable(),
    updated_at: z.number().optional().nullable(),
  })
  .transform((d) => ({
    id: d.playlist_id || d.id || '',
    name: d.name,
    thumbnails: d.thumbnails,
    cover_url: d.cover_url || d.normal_thumbnail || d.thumbnails[0] || null,
    track_count: d.track_count ?? null,
    created_at: d.created_at ?? null,
    updated_at: d.updated_at ?? null,
  }))

export type Playlist = z.infer<typeof PlaylistSchema>

export const AvailablePlaylistSchema = z
  .object({
    id: z.string(),
    kind: z.string().default('daily'),
    name: z.string(),
    thumbnails: z.array(z.string()).default([]),
    thumbnail_url: z.string().optional().nullable(),
    normal_thumbnail: z.string().optional().nullable(),
    endpoint: z.string(),
    requires_auth: z.boolean().default(false),
  })
  .transform((d) => ({
    id: d.id,
    kind: d.kind,
    title: d.name,
    thumbnails: d.thumbnails,
    cover_url: d.thumbnail_url || d.normal_thumbnail || d.thumbnails[0] || '',
    endpoint: d.endpoint,
    requires_auth: d.requires_auth,
  }))

export type AvailablePlaylist = z.infer<typeof AvailablePlaylistSchema>
