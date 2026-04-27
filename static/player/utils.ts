export const safeJsonParse = <T>(raw: string | null, fallback: T): T => {
  if (!raw) return fallback
  try {
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

export const formatTime = (seconds: number): string => {
  if (!Number.isFinite(seconds) || seconds < 0) return '00:00'
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}

export const parseEpisodeFromWatchHref = (href: string, malID: number): number | null => {
  if (!Number.isInteger(malID) || malID <= 0) return null

  try {
    const targetURL = new URL(href, window.location.origin)
    const pathParts = targetURL.pathname.split('/').filter(Boolean)
    if (pathParts.length < 3 || pathParts[0] !== 'watch') return null

    const targetMalID = Number.parseInt(pathParts[1] || '', 10)
    const targetEpisode = Number.parseInt(pathParts[2] || '', 10)
    if (!Number.isInteger(targetMalID) || targetMalID !== malID) return null
    if (!Number.isInteger(targetEpisode) || targetEpisode <= 0) return null

    return targetEpisode
  } catch {
    return null
  }
}
