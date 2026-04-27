import type { TimelineBounds } from './types'

export const timelineBounds = (video: HTMLVideoElement): TimelineBounds => {
  const duration = Number.isFinite(video.duration) && video.duration > 0 ? video.duration : 0

  let start = 0
  if (video.seekable.length > 0) {
    const seekableStart = video.seekable.start(0)
    if (Number.isFinite(seekableStart) && seekableStart > 0) {
      start = seekableStart
    }
  }

  if (duration > start) {
    return {
      start,
      end: duration,
      duration: duration - start,
    }
  }

  if (video.seekable.length > 0) {
    const seekableEnd = video.seekable.end(video.seekable.length - 1)
    if (Number.isFinite(seekableEnd) && seekableEnd > start) {
      return {
        start,
        end: seekableEnd,
        duration: seekableEnd - start,
      }
    }
  }

  return {
    start: 0,
    end: duration,
    duration,
  }
}

export const displayTimeFromAbsolute = (absoluteTime: number, video: HTMLVideoElement): number => {
  const bounds = timelineBounds(video)
  if (!Number.isFinite(absoluteTime) || bounds.duration <= 0) {
    return 0
  }

  const safeAbsoluteTime = Math.max(bounds.start, Math.min(bounds.end, absoluteTime))
  return safeAbsoluteTime - bounds.start
}

export const absoluteTimeFromDisplay = (displayTime: number, video: HTMLVideoElement): number => {
  const bounds = timelineBounds(video)
  if (!Number.isFinite(displayTime) || bounds.duration <= 0) {
    return 0
  }

  const safeDisplayTime = Math.max(0, Math.min(bounds.duration, displayTime))
  return bounds.start + safeDisplayTime
}

export const absoluteTimeFromRatio = (ratio: number, video: HTMLVideoElement): number => {
  const bounds = timelineBounds(video)
  if (!Number.isFinite(ratio) || bounds.duration <= 0) {
    return 0
  }

  const safeRatio = Math.max(0, Math.min(1, ratio))
  return bounds.start + (safeRatio * bounds.duration)
}
