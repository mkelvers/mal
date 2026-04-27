import type { SkipSegment, ParsedSegment } from './types'

export const parseSegments = (segments: SkipSegment[]): ParsedSegment[] => {
  const maxIntroStartSeconds = 180
  const minOutroStartRatio = 0.5
  const minSegmentDurationSeconds = 20
  const maxSegmentDurationSeconds = 240

  return segments
    .map((segment: SkipSegment) => {
      const start = Number(segment.start || 0)
      const end = Number(segment.end || 0)
      if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
        return null
      }
      const rawType = String(segment.type || '').toLowerCase()
      const type = rawType === 'ed' || rawType === 'outro' ? 'ed' : 'op'
      return { type, start: Math.max(0, start), end: Math.max(0, end) }
    })
    .filter((s: unknown): s is ParsedSegment => s !== null)
    .sort((a: ParsedSegment, b: ParsedSegment) => a.start - b.start)
}

export const resolveActiveSegments = (
  parsedSegments: ParsedSegment[],
  video: HTMLVideoElement
): ParsedSegment[] => {
  const bounds = getTimelineBounds(video)
  if (bounds.duration <= 0) {
    return []
  }

  return parsedSegments.filter((segment: ParsedSegment) => {
    const start = segment.start
    const end = segment.end
    const segmentDuration = end - start
    if (segmentDuration < 20 || segmentDuration > 240) return false
    if (start < 0 || end <= start || end > bounds.duration + 1) return false
    if (segment.type === 'op') {
      if (start > 180) return false
      if (start > bounds.duration * 0.5) return false
      return true
    }
    if (segment.type === 'ed') {
      return start >= bounds.duration * 0.5
    }
    return false
  })
}

export const skipLabel = (segmentType: string): string =>
  segmentType === 'ed' ? 'Skip outro' : 'Skip intro'

export const skipActivationTime = (segment: ParsedSegment): number => {
  const length = Math.max(0, segment.end - segment.start)
  const delay = Math.min(1, Math.max(0.25, length * 0.02))
  const boundedDelay = Math.min(delay, length * 0.5)
  return segment.start + boundedDelay
}

const getTimelineBounds = (video: HTMLVideoElement): { start: number; end: number; duration: number } => {
  const duration = Number.isFinite(video.duration) && video.duration > 0 ? video.duration : 0
  let start = 0

  if (video.seekable.length > 0) {
    const seekableStart = video.seekable.start(0)
    if (Number.isFinite(seekableStart) && seekableStart > 0) {
      start = seekableStart
    }
  }

  if (duration > start) {
    return { start, end: duration, duration: duration - start }
  }

  return { start: 0, end: duration, duration: duration }
}
