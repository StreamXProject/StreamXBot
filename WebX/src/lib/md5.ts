/* Compact MD5 (RFC 1321) — Web Crypto has no MD5, and Last.fm signatures require it. UTF-8 input, hex output. */
export function md5(input: string): string {
  const bytes = new TextEncoder().encode(input)
  const len = bytes.length
  const words = new Array<number>(((len + 8) >> 6) + 1 << 4).fill(0)
  for (let i = 0; i < len; i++) words[i >> 2]! |= bytes[i]! << ((i % 4) * 8)
  words[len >> 2]! |= 0x80 << ((len % 4) * 8)
  words[words.length - 2] = (len * 8) & 0xffffffff
  words[words.length - 1] = Math.floor((len * 8) / 0x100000000)

  const S = [7, 12, 17, 22, 5, 9, 14, 20, 4, 11, 16, 23, 6, 10, 15, 21]
  const K = new Array<number>(64)
  for (let i = 0; i < 64; i++) K[i] = Math.floor(Math.abs(Math.sin(i + 1)) * 0x100000000)

  let a0 = 0x67452301, b0 = 0xefcdab89, c0 = 0x98badcfe, d0 = 0x10325476
  const rotl = (x: number, c: number) => (x << c) | (x >>> (32 - c))

  for (let i = 0; i < words.length; i += 16) {
    let A = a0, B = b0, C = c0, D = d0
    for (let j = 0; j < 64; j++) {
      let F: number, g: number
      if (j < 16) { F = (B & C) | (~B & D); g = j }
      else if (j < 32) { F = (D & B) | (~D & C); g = (5 * j + 1) % 16 }
      else if (j < 48) { F = B ^ C ^ D; g = (3 * j + 5) % 16 }
      else { F = C ^ (B | ~D); g = (7 * j) % 16 }
      const tmp = D
      D = C
      C = B
      B = (B + rotl((A + F + K[j]! + words[i + g]!) | 0, S[(j >> 4) * 4 + (j % 4)]!)) | 0
      A = tmp
    }
    a0 = (a0 + A) | 0; b0 = (b0 + B) | 0; c0 = (c0 + C) | 0; d0 = (d0 + D) | 0
  }
  const hex = (n: number) => {
    let out = ''
    for (let i = 0; i < 4; i++) out += ((n >>> (i * 8)) & 0xff).toString(16).padStart(2, '0')
    return out
  }
  return hex(a0) + hex(b0) + hex(c0) + hex(d0)
}
