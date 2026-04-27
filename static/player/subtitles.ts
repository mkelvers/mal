import type { SubtitleItem, SubtitleTrack } from './types'

export const subtitleProxyURL = (track: SubtitleItem): string => {
  if (!track || !track.token) return ''
  return `/watch/proxy/subtitle?token=${encodeURIComponent(track.token)}`
}

export const subtitlesForMode = (
  mode: string,
  modeSources: Record<string, { token: string; subtitles: SubtitleItem[] }>
): SubtitleTrack[] => {
  const modeSource = modeSources[mode]
  if (!modeSource || !Array.isArray(modeSource.subtitles)) return []

  return modeSource.subtitles
    .map((track: SubtitleItem) => ({
      lang: (track.lang || 'unknown').toLowerCase(),
      label: track.lang || 'Unknown',
      url: subtitleProxyURL(track),
    }))
    .filter((track: SubtitleTrack) => track.url !== '')
}

export const parseVttTime = (raw: string): number => {
  const parts = raw.trim().split(':')
  if (parts.length < 2) return 0
  const secPart = parts.pop() || '0'
  const minPart = parts.pop() || '0'
  const hourPart = parts.pop() || '0'
  const seconds = Number(secPart.replace(',', '.'))
  const minutes = Number(minPart)
  const hours = Number(hourPart)
  return (hours * 3600) + (minutes * 60) + seconds
}

export const parseVtt = (text: string): Array<{ start: number; end: number; text: string }> => {
  const lines = text.replace(/\r/g, '').split('\n')
  const cues: Array<{ start: number; end: number; text: string }> = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i].trim()
    if (!line) {
      i += 1
      continue
    }
    let timeLine = line
    if (!line.includes('-->') && i + 1 < lines.length) {
      timeLine = lines[i + 1].trim()
      i += 1
    }
    if (!timeLine.includes('-->')) {
      i += 1
      continue
    }
    const [startRaw, endRaw] = timeLine.split('-->')
    const start = parseVttTime(startRaw)
    const end = parseVttTime(endRaw)
    i += 1
    const payload: string[] = []
    while (i < lines.length && lines[i].trim() !== '') {
      payload.push(lines[i])
      i += 1
    }
    const textContent = payload.join('\n').replace(/<[^>]+>/g, '').trim()
    if (textContent) cues.push({ start, end, text: textContent })
  }
  return cues
}

export const loadSubtitle = async (url: string): Promise<Array<{ start: number; end: number; text: string }>> => {
  try {
    const response = await fetch(url)
    if (!response.ok) return []
    const text = await response.text()
    return parseVtt(text)
  } catch {
    return []
  }
}
