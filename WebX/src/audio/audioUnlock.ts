/**
 * Browser Audio Unlocker
 *
 * Browsers (Chrome, Safari, Firefox, Edge) restrict AudioContext and HTMLAudioElement playback
 * under strict Autoplay Policies until the user interacts with the page.
 *
 * This module:
 *  1. Plays a tiny 44-byte silent WAV audio sample on the first user interaction (pointerdown, touchstart, click, keydown).
 *  2. Initializes/resumes a dummy AudioContext with a 0-gain oscillator to un-suspend the Web Audio API.
 *  3. Resumes the Equalizer's AudioContext.
 *  4. Primes the provided HTMLAudioElement if idle so subsequent async playback is never blocked.
 */
import { equalizer } from './Equalizer'

export const SILENT_SAMPLE_AUDIO =
  'data:audio/wav;base64,UklGRigAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQIAAAAAAA=='

const AUDIO_UNLOCK_KEY = '__webx_audio_unlocked__'

let isUnlocked = false

export async function unlockAudioOnce(): Promise<void> {
  if (typeof window === 'undefined') return
  const w = window as unknown as Record<string, unknown>
  if (isUnlocked || w[AUDIO_UNLOCK_KEY] === true) {
    // If already unlocked, still ensure Equalizer context is resumed
    equalizer.resume()
    return
  }
  isUnlocked = true
  w[AUDIO_UNLOCK_KEY] = true

  try {
    const AudioContextCtor =
      (window as unknown as { AudioContext?: typeof AudioContext }).AudioContext ||
      (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext

    if (AudioContextCtor) {
      const ctx = new AudioContextCtor()
      try {
        if (ctx.state !== 'running') {
          await ctx.resume()
        }
        const osc = ctx.createOscillator()
        const gain = ctx.createGain()
        gain.gain.value = 0
        osc.connect(gain)
        gain.connect(ctx.destination)
        osc.start()
        osc.stop(ctx.currentTime + 0.01)
      } finally {
        try {
          await ctx.close()
        } catch {}
      }
    }
  } catch {}

  equalizer.resume()

  // 2. Play silent audio element sample to permanently unlock HTMLMediaElement playback
  try {
    const a = document.createElement('audio')
    a.muted = true
    a.setAttribute('playsinline', 'true')
    a.setAttribute('webkit-playsinline', 'true')
    a.src = SILENT_SAMPLE_AUDIO
    await a.play()
    a.pause()
    a.remove()
  } catch {}
}

/**
 * Register global listeners so the very first tap/click/keypress on the document
 * unlocks audio capabilities before any user-triggered playback starts.
 */
export function initAudioUnlock(): void {
  if (typeof window === 'undefined') return

  const onFirstGesture = () => {
    void unlockAudioOnce()
  }

  const opts: AddEventListenerOptions = { passive: true, capture: true, once: true }

  window.addEventListener('pointerdown', onFirstGesture, opts)
  window.addEventListener('touchstart', onFirstGesture, opts)
  window.addEventListener('mousedown', onFirstGesture, opts)
  window.addEventListener('keydown', onFirstGesture, opts)
}
