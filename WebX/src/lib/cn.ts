export type ClassValue = string | number | bigint | boolean | null | undefined | ClassValue[] | Record<string, boolean | null | undefined>

/** Tiny className joiner (no dependency). */
export function cn(...inputs: ClassValue[]): string {
  const out: string[] = []
  for (const i of inputs) {
    if (!i || i === true) continue
    if (typeof i === 'string' || typeof i === 'number') out.push(String(i))
    else if (Array.isArray(i)) {
      const s = cn(...i)
      if (s) out.push(s)
    } else if (typeof i === 'object') {
      for (const [k, v] of Object.entries(i)) if (v) out.push(k)
    }
  }
  return out.join(' ')
}
