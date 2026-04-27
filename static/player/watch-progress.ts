import type { WatchProgressPayload } from './types'

const watchProgressURL = '/api/watch-progress'

export const buildWatchProgressPayload = (
  malId: number,
  episodeNumber: number,
  timeSeconds: number
): string => {
  return JSON.stringify({
    mal_id: malId,
    episode: episodeNumber,
    time_seconds: timeSeconds,
  } as WatchProgressPayload)
}

export const sendWatchProgressBeacon = (payload: string): boolean => {
  if (!navigator.sendBeacon) {
    return false
  }

  const blob = new Blob([payload], { type: 'application/json' })
  navigator.sendBeacon(watchProgressURL, blob)
  return true
}

export const saveProgress = async (
  video: HTMLVideoElement,
  malID: number,
  currentEpisode: string,
  lastSavedProgress: { episode: string; seconds: number }
): Promise<{ episode: string; seconds: number }> => {
  if (!Number.isInteger(malID) || malID <= 0) return lastSavedProgress

  const bounds = getTimelineBounds(video)
  if (bounds.duration <= 0) return lastSavedProgress

  const episodeNumber = Number.parseInt(currentEpisode, 10)
  if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return lastSavedProgress

  const safeTime = displayTimeFromAbsolute(video.currentTime, video)
  if (lastSavedProgress.episode === currentEpisode && Math.abs(lastSavedProgress.seconds - safeTime) < 5) {
    return lastSavedProgress
  }

  const payload = buildWatchProgressPayload(malID, episodeNumber, safeTime)

  try {
    const response = await fetch(watchProgressURL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: payload,
    })
    if (!response.ok) return lastSavedProgress
    return {
      episode: currentEpisode,
      seconds: safeTime,
    }
  } catch {
    return lastSavedProgress
  }
}

export const markEpisodeTransition = (
  malID: number,
  episodeNumber: number
): void => {
  if (!Number.isInteger(malID) || malID <= 0) return
  if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

  const payload = buildWatchProgressPayload(malID, episodeNumber, 0)

  if (sendWatchProgressBeacon(payload)) {
    return
  }

  fetch(watchProgressURL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    keepalive: true,
    body: payload,
  }).catch(() => {})
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

const displayTimeFromAbsolute = (absoluteTime: number, video: HTMLVideoElement): number => {
  const bounds = getTimelineBounds(video)
  if (!Number.isFinite(absoluteTime) || bounds.duration <= 0) {
    return 0
  }

  const safeAbsoluteTime = Math.max(bounds.start, Math.min(bounds.end, absoluteTime))
  return safeAbsoluteTime - bounds.start
}
