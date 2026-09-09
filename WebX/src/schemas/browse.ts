import { z } from 'zod'
import { TrackSchema } from './track'

export const BrowseResponseSchema = z.object({
  page: z.number().default(1),
  per_page: z.number().default(20),
  total: z.number().default(0),
  items: z.array(z.any()).default([]),
  cover_url: z.string().optional().nullable(),
}).transform((d) => ({
  ...d,
  items: d.items.map((i) => TrackSchema.safeParse(i)).filter((r) => r.success).map((r) => r.data!).filter((t) => t.id),
}))

export type BrowseResponse = z.infer<typeof BrowseResponseSchema>
