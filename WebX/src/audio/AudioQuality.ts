export type AudioQualityLevel = 'low' | 'medium' | 'high' | 'lossless'

export interface AudioQualityOption {
  id: AudioQualityLevel
  label: string
  bitrate: string
  description: string
}

export const AUDIO_QUALITY_OPTIONS: AudioQualityOption[] = [
  { id: 'low', label: 'Data Saver', bitrate: '96 kbps', description: 'Lowest data usage, optimized for cellular' },
  { id: 'medium', label: 'Standard', bitrate: '160 kbps', description: 'Balanced sound quality and speed' },
  { id: 'high', label: 'High Quality', bitrate: '320 kbps', description: 'Crystal clear MP3 / AAC' },
  { id: 'lossless', label: 'Hi-Res Lossless', bitrate: 'FLAC / 24-bit', description: 'Studio master quality' },
]
