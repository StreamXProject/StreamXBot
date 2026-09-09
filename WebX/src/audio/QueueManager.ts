import type { Track } from '@/schemas/track'
import type { RepeatMode } from './AudioState'

export class QueueManager {
  private queue: Track[] = []
  private originalQueue: Track[] = []
  private currentIndex: number = -1
  private isShuffle: boolean = false
  private repeatMode: RepeatMode = 'off'
  private history: Track[] = []

  constructor(initialTracks: Track[] = [], initialIndex: number = -1) {
    if (initialTracks.length > 0) {
      this.setQueue(initialTracks, initialIndex)
    }
  }

  public setQueue(tracks: Track[], startIndex: number = 0): void {
    this.originalQueue = [...tracks]
    if (this.isShuffle) {
      this.queue = this.shuffleArray([...tracks], startIndex)
      this.currentIndex = 0
    } else {
      this.queue = [...tracks]
      this.currentIndex = Math.max(0, Math.min(startIndex, tracks.length - 1))
    }
  }

  public getCurrentTrack(): Track | null {
    if (this.currentIndex >= 0 && this.currentIndex < this.queue.length) {
      return this.queue[this.currentIndex]
    }
    return null
  }

  public getQueue(): Track[] {
    return [...this.queue]
  }

  public getCurrentIndex(): number {
    return this.currentIndex
  }

  public getRepeatMode(): RepeatMode {
    return this.repeatMode
  }

  public getIsShuffle(): boolean {
    return this.isShuffle
  }

  public setRepeatMode(mode: RepeatMode): void {
    this.repeatMode = mode
  }

  public toggleShuffle(): boolean {
    this.isShuffle = !this.isShuffle
    const current = this.getCurrentTrack()

    if (this.isShuffle) {
      this.queue = this.shuffleArray([...this.originalQueue], current ? this.originalQueue.findIndex(t => t.id === current.id) : 0)
      this.currentIndex = current ? this.queue.findIndex(t => t.id === current.id) : 0
    } else {
      this.queue = [...this.originalQueue]
      this.currentIndex = current ? this.queue.findIndex(t => t.id === current.id) : 0
    }

    return this.isShuffle
  }

  public next(): Track | null {
    if (this.queue.length === 0) return null

    if (this.repeatMode === 'one') {
      return this.getCurrentTrack()
    }

    const current = this.getCurrentTrack()
    if (current) {
      this.history.push(current)
      if (this.history.length > 50) this.history.shift()
    }

    if (this.currentIndex < this.queue.length - 1) {
      this.currentIndex++
      return this.queue[this.currentIndex]
    } else if (this.repeatMode === 'all') {
      this.currentIndex = 0
      return this.queue[0]
    }

    return null
  }

  public previous(): Track | null {
    if (this.queue.length === 0) return null

    if (this.currentIndex > 0) {
      this.currentIndex--
      return this.queue[this.currentIndex]
    } else if (this.repeatMode === 'all') {
      this.currentIndex = this.queue.length - 1
      return this.queue[this.currentIndex]
    }

    return this.getCurrentTrack()
  }

  public jumpToIndex(index: number): Track | null {
    if (index >= 0 && index < this.queue.length) {
      this.currentIndex = index
      return this.queue[index]
    }
    return null
  }

  public addTrackNext(track: Track): void {
    if (this.currentIndex === -1) {
      this.setQueue([track], 0)
      return
    }
    this.queue.splice(this.currentIndex + 1, 0, track)
    this.originalQueue.push(track)
  }

  public appendTrack(track: Track): void {
    if (this.queue.length === 0) {
      this.setQueue([track], 0)
      return
    }
    this.queue.push(track)
    this.originalQueue.push(track)
  }

  public removeTrack(index: number): void {
    if (index < 0 || index >= this.queue.length) return
    const [removed] = this.queue.splice(index, 1)
    const origIdx = this.originalQueue.findIndex(t => t.id === removed.id)
    if (origIdx !== -1) {
      this.originalQueue.splice(origIdx, 1)
    }
    if (index < this.currentIndex) {
      this.currentIndex--
    } else if (index === this.currentIndex && this.currentIndex >= this.queue.length) {
      this.currentIndex = this.queue.length - 1
    }
  }

  public reorder(fromIndex: number, toIndex: number): void {
    if (
      fromIndex < 0 ||
      fromIndex >= this.queue.length ||
      toIndex < 0 ||
      toIndex >= this.queue.length
    ) {
      return
    }

    const [moved] = this.queue.splice(fromIndex, 1)
    this.queue.splice(toIndex, 0, moved)

    if (this.currentIndex === fromIndex) {
      this.currentIndex = toIndex
    } else if (fromIndex < this.currentIndex && toIndex >= this.currentIndex) {
      this.currentIndex--
    } else if (fromIndex > this.currentIndex && toIndex <= this.currentIndex) {
      this.currentIndex++
    }
  }

  public clear(): void {
    this.queue = []
    this.originalQueue = []
    this.currentIndex = -1
    this.history = []
  }

  private shuffleArray(array: Track[], keepIndex: number): Track[] {
    const keepItem = array[keepIndex]
    const remaining = array.filter((_, idx) => idx !== keepIndex)

    for (let i = remaining.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [remaining[i], remaining[j]] = [remaining[j], remaining[i]]
    }

    return keepItem ? [keepItem, ...remaining] : remaining
  }
}
