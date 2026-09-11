/**
 * 10-band graphic equalizer on top of the Web Audio API.
 *
 * The <audio> element is routed through: source → preamp → [10 × biquad] → destination.
 * `createMediaElementSource` can only be called once per element and permanently re-routes
 * its output, so the graph is created lazily on first enable and then kept; "off" just
 * flattens every band (no audible difference from bypass, and no re-routing churn).
 *
 * Note: the media must be same-origin or served with CORS (`crossOrigin="anonymous"`),
 * otherwise the Web Audio graph outputs silence for tainted media.
 */
export const EQ_FREQUENCIES = [32, 64, 125, 250, 500, 1000, 2000, 4000, 8000, 16000] as const
export type EqGains = number[] // dB per band, -12..12

export interface EqPreset { id: string; name: string; gains: EqGains }
export const EQ_PRESETS: EqPreset[] = [
  { id: 'flat', name: 'Flat', gains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] },
  { id: 'bass', name: 'Bass boost', gains: [6, 5, 4, 2, 0, 0, 0, 0, 0, 0] },
  { id: 'treble', name: 'Treble boost', gains: [0, 0, 0, 0, 0, 1, 2, 4, 5, 6] },
  { id: 'vocal', name: 'Vocal', gains: [-2, -1, 0, 1, 3, 4, 3, 1, 0, -1] },
  { id: 'rock', name: 'Rock', gains: [4, 3, 1, -1, -2, -1, 1, 3, 4, 4] },
  { id: 'pop', name: 'Pop', gains: [-1, 0, 2, 3, 4, 3, 1, 0, -1, -1] },
  { id: 'electronic', name: 'Electronic', gains: [4, 3, 1, 0, -1, 1, 0, 1, 3, 4] },
  { id: 'acoustic', name: 'Acoustic', gains: [3, 2, 1, 0, 1, 1, 2, 3, 2, 1] },
  { id: 'loudness', name: 'Loudness', gains: [5, 4, 0, 0, -1, 0, 0, 2, 4, 5] },
]

export const EQ_MIN_DB = -12
export const EQ_MAX_DB = 12

const clampDb = (v: number) => Math.max(EQ_MIN_DB, Math.min(EQ_MAX_DB, Number.isFinite(v) ? v : 0))

class EqualizerImpl {
  private ctx: AudioContext | null = null
  private source: MediaElementAudioSourceNode | null = null
  private preamp: GainNode | null = null
  private filters: BiquadFilterNode[] = []
  private element: HTMLMediaElement | null = null
  private enabled = false
  private gains: EqGains = EQ_PRESETS[0]!.gains.slice()
  private preampDb = 0

  get isSupported(): boolean {
    return typeof window !== 'undefined' && typeof (window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext) !== 'undefined'
  }

  get isAttached(): boolean {
    return this.source !== null
  }

  /** Bind the player's <audio>; the graph is only built once EQ is first enabled. */
  attach(el: HTMLMediaElement): void {
    this.element = el
    if (this.enabled) this.ensureGraph()
  }

  private ensureGraph(): boolean {
    if (!this.isSupported || !this.element) return false
    if (this.source) return true
    try {
      const Ctor = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      this.ctx = new Ctor()
      this.source = this.ctx.createMediaElementSource(this.element)
      this.preamp = this.ctx.createGain()
      this.filters = EQ_FREQUENCIES.map((f, i) => {
        const node = this.ctx!.createBiquadFilter()
        node.type = i === 0 ? 'lowshelf' : i === EQ_FREQUENCIES.length - 1 ? 'highshelf' : 'peaking'
        node.frequency.value = f
        node.Q.value = 1.1
        node.gain.value = 0
        return node
      })
      let prev: AudioNode = this.source
      prev.connect(this.preamp)
      prev = this.preamp
      for (const f of this.filters) {
        prev.connect(f)
        prev = f
      }
      prev.connect(this.ctx.destination)
      this.apply()
      return true
    } catch (err) {
      console.warn('[EQ] Could not build audio graph', err)
      this.teardownRefs()
      return false
    }
  }

  private teardownRefs() {
    this.source = null
    this.preamp = null
    this.filters = []
    this.ctx = null
  }

  /** Browsers suspend AudioContexts until a user gesture — call on play. */
  resume(): void {
    if (this.ctx && this.ctx.state === 'suspended') void this.ctx.resume().catch(() => {})
  }

  setEnabled(on: boolean): boolean {
    this.enabled = on
    if (on && !this.ensureGraph()) {
      this.enabled = false
      return false
    }
    this.apply()
    return this.enabled
  }

  setGains(gains: EqGains): void {
    this.gains = EQ_FREQUENCIES.map((_, i) => clampDb(gains[i] ?? 0))
    this.apply()
  }

  setBand(index: number, db: number): void {
    if (index < 0 || index >= EQ_FREQUENCIES.length) return
    this.gains[index] = clampDb(db)
    this.apply()
  }

  setPreamp(db: number): void {
    this.preampDb = clampDb(db)
    this.apply()
  }

  getGains(): EqGains {
    return this.gains.slice()
  }

  private apply(): void {
    if (!this.ctx || !this.preamp) return
    const t = this.ctx.currentTime
    const ramp = 0.05
    this.preamp.gain.setTargetAtTime(this.enabled ? Math.pow(10, this.preampDb / 20) : 1, t, ramp)
    this.filters.forEach((f, i) => f.gain.setTargetAtTime(this.enabled ? this.gains[i] ?? 0 : 0, t, ramp))
  }
}

export const equalizer = new EqualizerImpl()
