declare const htmx: {
  ajax(verb: string, path: string, target: HTMLElement): Promise<void>
}

export {}

import DOMPurify from 'dompurify'

interface ModeSource {
  token: string
  subtitles: SubtitleItem[]
}

interface SubtitleItem {
  lang: string
  token: string
}

interface SkipSegment {
  type: string
  start: number
  end: number
}

interface EpisodeData {
  mal_id: number
  title: string
  current_episode: string
  total_episodes: number
  initial_mode: string
  token: string
  available_modes: string[]
  mode_sources: Record<string, ModeSource>
  segments: SkipSegment[]
  episode_title: string
}

interface EpisodeData {
  mal_id: number
  title: string
  current_episode: string
  total_episodes: number
  initial_mode: string
  token: string
  available_modes: string[]
  mode_sources: Record<string, ModeSource>
  segments: SkipSegment[]
  episode_title: string
}

let playerInitialized = false

const initPlayer = (): void => {
  const container = document.querySelector('[data-video-player]')
  if (!container) return

  if (playerInitialized) return

  const shouldAutoPlay = sessionStorage.getItem('mal:autoplay-next') === 'true'
  sessionStorage.removeItem('mal:autoplay-next')

  const video = container.querySelector('video') as HTMLVideoElement
  const loading = container.querySelector('[data-loading]') as HTMLElement
  const playPause = container.querySelector('[data-play-pause]') as HTMLButtonElement
  const iconPlay = container.querySelector('[data-icon-play]') as SVGElement
  const iconPause = container.querySelector('[data-icon-pause]') as SVGElement
  const muteBtn = container.querySelector('[data-mute]') as HTMLButtonElement
  const volumeWrap = container.querySelector('[data-volume-wrap]') as HTMLElement
  const volumePanel = container.querySelector('[data-volume-panel]') as HTMLElement
  const volumeRange = container.querySelector('[data-volume-range]') as HTMLInputElement
  const iconVolume = container.querySelector('[data-icon-volume]') as SVGElement
  const iconMuted = container.querySelector('[data-icon-muted]') as SVGElement
  const volumeUnderline = container.querySelector('[data-volume-underline]') as HTMLElement
  const timeDisplay = container.querySelector('[data-time]') as HTMLElement
  const durationDisplay = container.querySelector('[data-duration]') as HTMLElement
  const progressWrap = container.querySelector('[data-progress-wrap]') as HTMLElement
  const progress = container.querySelector('[data-progress]') as HTMLElement
  const scrubber = container.querySelector('[data-scrubber]') as HTMLElement
  const segmentsTrack = container.querySelector('[data-segments]') as HTMLElement
  const subtitleSelect = container.querySelector('[data-subtitle-select]') as HTMLSelectElement
  const backwardBtn = container.querySelector('[data-backward]') as HTMLButtonElement
  const forwardBtn = container.querySelector('[data-forward]') as HTMLButtonElement
  const fullscreenBtn = container.querySelector('[data-fullscreen]') as HTMLButtonElement
  const skipSegmentBtn = container.querySelector('[data-skip]') as HTMLButtonElement
  const autoplayBtn = document.querySelector('[data-autoplay]') as HTMLButtonElement
  const subtitleText = container.querySelector('[data-subtitle-text]') as HTMLElement

  const streamURL = container.getAttribute('data-stream-url') || '/watch/proxy/stream'
  const initialStreamToken = container.getAttribute('data-stream-token') || ''
  let currentEpisode = container.getAttribute('data-current-episode') || '1'
  const malID = Number.parseInt(container.getAttribute('data-mal-id') || '', 10)
  let totalEpisodes = Number.parseInt(container.getAttribute('data-total-episodes') || '0', 10)
  const animeTitle = container.getAttribute('data-anime-title') || ''
  const animeTitleEnglish = container.getAttribute('data-anime-title-english') || ''
  const animeTitleJapanese = container.getAttribute('data-anime-title-japanese') || ''
  const animeImage = container.getAttribute('data-anime-image') || ''
  const animeAiring = (container.getAttribute('data-anime-airing') || '').toLowerCase() === 'true'
  const safeJsonParse = <T>(raw: string | null, fallback: T): T => {
    if (!raw) return fallback
    try {
      return JSON.parse(raw) as T
    } catch {
      return fallback
    }
  }

  let modeSources = safeJsonParse(container.getAttribute('data-mode-sources'), {} as Record<string, ModeSource>)
  let availableModes = safeJsonParse(container.getAttribute('data-available-modes'), [] as string[])
  const initialMode = container.getAttribute('data-initial-mode') || 'dub'
  const segments = safeJsonParse(container.getAttribute('data-segments'), [] as SkipSegment[])
  const maxIntroStartSeconds = 180
  const minOutroStartRatio = 0.5
  const minSegmentDurationSeconds = 20
  const maxSegmentDurationSeconds = 240

  let parsedSegments = segments
    .map((segment: SkipSegment) => {
      const start = Number(segment.start || 0)
      const end = Number(segment.end || 0)
      if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
        return { ...segment, start: 0, end: 0 }
      }
      return { ...segment, start, end }
    })
    .filter((s: SkipSegment) => s.start > 0 || s.end > 0)

  let activeSegments: Array<{ type: string, start: number, end: number }> = []
  let lastSavedProgress = { episode: currentEpisode, seconds: -1 }
  let progressSaveTimer: number | undefined
  let transitionEpisode: number | null = null
  let completionSent = false
  let completionAttempts = 0
  let playerControlsTimeout: number | undefined
  let isScrubbing = false
  let lastKnownVolume = 1
  let pendingSeekTime: number | null = null
      let activeSkipSegment: { type: string, start: number, end: number } | null = null
  let activeSubtitles: Array<{ start: number, end: number, text: string }> = []
  let currentSubtitleTracks: Array<{ lang: string, label: string, url: string }> = []

  let currentMode = availableModes.includes(initialMode) ? initialMode : (availableModes[0] || 'dub')
  const fallbackMode = Object.keys(modeSources).find((mode) => typeof modeSources[mode]?.token === 'string' && modeSources[mode].token !== '')
  if ((!modeSources[currentMode] || !modeSources[currentMode].token) && fallbackMode) {
    currentMode = fallbackMode
  }
  const watchProgressURL = '/api/watch-progress'

  const previewPopover = container.querySelector('[data-preview-popover]') as HTMLElement
  const previewTime = container.querySelector('[data-preview-time]') as HTMLElement
  const videoOverlay = container.querySelector('[data-video-overlay]') as HTMLElement
  const streamUrlForMode = (mode: string): string => {
    const modeParam = encodeURIComponent(mode)
    const modeSource = modeSources[mode]
    const token = modeSource?.token
    if (!token) return ''
    const tokenParam = encodeURIComponent(token)
    return `${streamURL}?mode=${modeParam}&token=${tokenParam}`
  }

  const subtitleProxyURL = (track: SubtitleItem): string => {
    if (!track || !track.token) return ''
    return `/watch/proxy/subtitle?token=${encodeURIComponent(track.token)}`
  }

  const subtitlesForMode = (mode: string): Array<{ lang: string, label: string, url: string }> => {
    const modeSource = modeSources[mode]
    if (!modeSource || !Array.isArray(modeSource.subtitles)) return []
    return modeSource.subtitles
      .map((track: SubtitleItem) => ({
        lang: (track.lang || 'unknown').toLowerCase(),
        label: track.lang || 'Unknown',
        url: subtitleProxyURL(track),
      }))
      .filter((track: { url: string }) => track.url !== '')
  }

  const skipLabel = (segmentType: string): string => segmentType === 'ed' ? 'Skip outro' : 'Skip intro'

  const isAutoplayEnabled = (): boolean => localStorage.getItem('mal:autoplay-enabled') !== 'false'

  const updateAutoplayButton = (): void => {
    if (!autoplayBtn) return
    const enabled = isAutoplayEnabled()
    const checkbox = autoplayBtn as unknown as HTMLInputElement
    checkbox.checked = enabled
  }

  const timelineBounds = (): { start: number, end: number, duration: number } => {
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

  const displayTimeFromAbsolute = (absoluteTime: number): number => {
    const bounds = timelineBounds()
    if (!Number.isFinite(absoluteTime) || bounds.duration <= 0) {
      return 0
    }

    const safeAbsoluteTime = Math.max(bounds.start, Math.min(bounds.end, absoluteTime))
    return safeAbsoluteTime - bounds.start
  }

  const absoluteTimeFromDisplay = (displayTime: number): number => {
    const bounds = timelineBounds()
    if (!Number.isFinite(displayTime) || bounds.duration <= 0) {
      return 0
    }

    const safeDisplayTime = Math.max(0, Math.min(bounds.duration, displayTime))
    return bounds.start + safeDisplayTime
  }

  const absoluteTimeFromRatio = (ratio: number): number => {
    const bounds = timelineBounds()
    if (!Number.isFinite(ratio) || bounds.duration <= 0) {
      return 0
    }

    const safeRatio = Math.max(0, Math.min(1, ratio))
    return bounds.start + (safeRatio * bounds.duration)
  }

  const resolveActiveSegments = (): void => {
    const bounds = timelineBounds()
    if (bounds.duration <= 0) {
      activeSegments = []
      return
    }

    activeSegments = parsedSegments.filter((segment: { start: number, end: number, type: string }) => {
      const start = segment.start
      const end = segment.end
      const segmentDuration = end - start
      if (segmentDuration < minSegmentDurationSeconds || segmentDuration > maxSegmentDurationSeconds) return false
      if (start < 0 || end <= start || end > bounds.duration + 1) return false
      if (segment.type === 'op') {
        if (start > maxIntroStartSeconds) return false
        if (start > bounds.duration * 0.5) return false
        return true
      }
      if (segment.type === 'ed') {
        return start >= bounds.duration * minOutroStartRatio
      }
      return false
    })
  }

  const skipActivationTime = (segment: { start: number, end: number }): number => {
    const length = Math.max(0, segment.end - segment.start)
    const delay = Math.min(1, Math.max(0.25, length * 0.02))
    const boundedDelay = Math.min(delay, length * 0.5)
    return segment.start + boundedDelay
  }

  const updateSkipButton = (currentTime: number): void => {
    const currentDisplayTime = displayTimeFromAbsolute(currentTime)
    const segment = activeSegments.find((item: { start: number, end: number }) => {
      const activationTime = skipActivationTime(item)
      return currentDisplayTime >= activationTime && currentDisplayTime < item.end
    })
    if (!segment) {
      activeSkipSegment = null
      skipSegmentBtn?.classList.add('hidden')
      skipSegmentBtn?.classList.remove('block')
      return
    }
    activeSkipSegment = segment
    if (skipSegmentBtn) {
      skipSegmentBtn.textContent = skipLabel(segment.type)
      skipSegmentBtn.title = skipLabel(segment.type)
      skipSegmentBtn.classList.remove('hidden')
      skipSegmentBtn.classList.add('block')
    }
  }

  const renderSegments = (): void => {
    if (!segmentsTrack) return
    segmentsTrack.innerHTML = ''

    const bounds = timelineBounds()
    if (bounds.duration <= 0) return

    activeSegments.forEach((segment: { start: number, end: number }) => {
      const left = (segment.start / bounds.duration) * 100
      const width = ((segment.end - segment.start) / bounds.duration) * 100
      const bar = document.createElement('div')
      bar.className = 'absolute top-0 h-full bg-yellow-400'
      bar.style.left = `${left}%`
      bar.style.width = `${width}%`
      segmentsTrack.appendChild(bar)
    })
  }

  const updateVideoOverlay = (episode: string, episodeTitle: string): void => {
    if (!videoOverlay) return
    const episodeText = episodeTitle
      ? `Episode ${episode}, ${episodeTitle}`
      : `Episode ${episode}`
    const secondLine = videoOverlay.querySelector('p')
    if (secondLine) {
      secondLine.textContent = episodeText
    }
  }

  const formatTime = (seconds: number): string => {
    if (!Number.isFinite(seconds) || seconds < 0) return '00:00'
    const mins = Math.floor(seconds / 60)
    const secs = Math.floor(seconds % 60)
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  }

  const updateTimeline = (currentTime: number): void => {
    if (!timeDisplay || !progress) return

    const bounds = timelineBounds()
    if (bounds.duration <= 0) {
      progress.style.width = '0%'
      if (scrubber) scrubber.style.left = '0%'
      timeDisplay.textContent = '00:00'
      if (durationDisplay) durationDisplay.textContent = '00:00'
      return
    }

    const currentDisplayTime = displayTimeFromAbsolute(currentTime)
    const pct = Math.max(0, Math.min(100, (currentDisplayTime / bounds.duration) * 100))
    progress.style.width = `${pct}%`
    if (scrubber) scrubber.style.left = `${pct}%`
    timeDisplay.textContent = formatTime(currentDisplayTime)
    if (durationDisplay) durationDisplay.textContent = formatTime(bounds.duration)
  }

  const seekBy = (delta: number): void => {
    const bounds = timelineBounds()
    if (bounds.duration <= 0) return

    const next = Math.max(bounds.start, Math.min(bounds.end, video.currentTime + delta))
    video.currentTime = next
    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  }

  const hidePreviewPopover = (): void => {
    if (!previewPopover) return
    previewPopover.style.left = '0px'
    previewPopover.classList.remove('block')
    previewPopover.classList.add('hidden')
  }

  const showPreviewPopover = (): void => {
    if (!previewPopover) return
    previewPopover.classList.remove('hidden')
    previewPopover.classList.add('block')
  }

  const updatePreviewUI = (ratio: number): void => {
    if (!progressWrap || !previewPopover || !previewTime) return

    const bounds = timelineBounds()
    if (bounds.duration <= 0) {
      hidePreviewPopover()
      return
    }

    const targetTime = Math.max(0, Math.min(bounds.duration, ratio * bounds.duration))
    previewTime.textContent = formatTime(targetTime)
    const barWidth = progressWrap.clientWidth
    if (barWidth <= 0) {
      hidePreviewPopover()
      return
    }

    showPreviewPopover()
    let popoverWidth = 72
    if (previewPopover.offsetWidth > 0) {
      popoverWidth = previewPopover.offsetWidth
    }

    const popoverOffset = ratio * barWidth
    const halfWidth = popoverWidth / 2
    const clampedOffset = Math.max(halfWidth, Math.min(barWidth - halfWidth, popoverOffset))
    previewPopover.style.left = `${clampedOffset}px`
  }

  const buildWatchProgressPayload = (episodeNumber: number, timeSeconds: number): string => {
    return JSON.stringify({
      mal_id: malID,
      episode: episodeNumber,
      time_seconds: timeSeconds,
    })
  }

  const sendWatchProgressBeacon = (payload: string): boolean => {
    if (!navigator.sendBeacon) {
      return false
    }

    const blob = new Blob([payload], { type: 'application/json' })
    navigator.sendBeacon(watchProgressURL, blob)
    return true
  }

  const saveProgress = async (): Promise<void> => {
    if (!Number.isInteger(malID) || malID <= 0) return

    const bounds = timelineBounds()
    if (bounds.duration <= 0) return

    const episodeNumber = Number.parseInt(currentEpisode, 10)
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    const safeTime = displayTimeFromAbsolute(video.currentTime)
    if (lastSavedProgress.episode === currentEpisode && Math.abs(lastSavedProgress.seconds - safeTime) < 5) {
      return
    }

    const payload = buildWatchProgressPayload(episodeNumber, safeTime)

    try {
      const response = await fetch(watchProgressURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: payload,
      })
      if (!response.ok) return
      lastSavedProgress = {
        episode: currentEpisode,
        seconds: safeTime,
      }
    } catch {
      return
    }
  }

  const scheduleProgressSave = (): void => {
    if (progressSaveTimer !== undefined) return
    progressSaveTimer = window.setTimeout(() => {
      progressSaveTimer = undefined
      saveProgress()
    }, 30000)
  }

  const parseEpisodeFromWatchHref = (href: string): number | null => {
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

  const markEpisodeTransition = (episodeNumber: number): void => {
    if (!Number.isInteger(malID) || malID <= 0) return
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    transitionEpisode = episodeNumber
    const payload = buildWatchProgressPayload(episodeNumber, 0)

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

  const parseVttTime = (raw: string): number => {
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

  const parseVtt = (text: string): Array<{ start: number, end: number, text: string }> => {
    const lines = text.replace(/\r/g, '').split('\n')
    const cues: Array<{ start: number, end: number, text: string }> = []
    let i = 0
    while (i < lines.length) {
      const line = lines[i].trim()
      if (!line) { i += 1; continue }
      let timeLine = line
      if (!line.includes('-->') && i + 1 < lines.length) {
        timeLine = lines[i + 1].trim()
        i += 1
      }
      if (!timeLine.includes('-->')) { i += 1; continue }
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

  const loadSubtitle = async (url: string): Promise<Array<{ start: number, end: number, text: string }>> => {
    try {
      const response = await fetch(url)
      if (!response.ok) return []
      const text = await response.text()
      return parseVtt(text)
    } catch {
      return []
    }
  }

  const hideSubtitleText = (): void => {
    if (!subtitleText) return
    subtitleText.textContent = ''
    subtitleText.classList.remove('block')
    subtitleText.classList.add('hidden')
  }

  const updateSubtitleRender = (currentTime: number): void => {
    if (!subtitleText) return
    if (!activeSubtitles.length) {
      hideSubtitleText()
      return
    }
    const cue = activeSubtitles.find(item => currentTime >= item.start && currentTime <= item.end)
    if (!cue) {
      hideSubtitleText()
      return
    }
    subtitleText.textContent = cue.text
    subtitleText.classList.remove('hidden')
    subtitleText.classList.add('block')
  }

  const updateSubtitleOptions = (): void => {
    if (!subtitleSelect) return
    currentSubtitleTracks = subtitlesForMode(currentMode)
    subtitleSelect.innerHTML = ''
    const none = document.createElement('option')
    none.value = 'none'
    none.textContent = 'Off'
    subtitleSelect.appendChild(none)
    subtitleSelect.value = 'none'
    currentSubtitleTracks.forEach((track, idx) => {
      const option = document.createElement('option')
      option.value = String(idx)
      option.textContent = track.label
      subtitleSelect.appendChild(option)
    })
    subtitleSelect.style.display = currentSubtitleTracks.length > 0 ? 'block' : 'none'
    activeSubtitles = []
    hideSubtitleText()
  }

  const modeDub = container.querySelector('[data-mode-dub]') as HTMLButtonElement
  const modeSub = container.querySelector('[data-mode-sub]') as HTMLButtonElement

  const updateModeButtons = (mode: string): void => {
    if (modeDub) {
      modeDub.disabled = !availableModes.includes('dub')
      modeDub.classList.toggle('text-white', mode !== 'dub')
      modeDub.classList.toggle('text-yellow-400', mode === 'dub')
      modeDub.classList.toggle('opacity-50', !availableModes.includes('dub'))
      modeDub.classList.toggle('cursor-not-allowed', !availableModes.includes('dub'))
    }
    if (modeSub) {
      modeSub.disabled = !availableModes.includes('sub')
      modeSub.classList.toggle('text-white', mode !== 'sub')
      modeSub.classList.toggle('text-yellow-400', mode === 'sub')
      modeSub.classList.toggle('opacity-50', !availableModes.includes('sub'))
      modeSub.classList.toggle('cursor-not-allowed', !availableModes.includes('sub'))
    }
  }

  const switchMode = (mode: string): void => {
    if (!availableModes.includes(mode) || mode === currentMode) return
    const nextURL = streamUrlForMode(mode)
    if (!nextURL) return
    const wasPlaying = !video.paused
    const previousTime = displayTimeFromAbsolute(video.currentTime)
    currentMode = mode
    hidePreviewPopover()
    video.src = nextURL
    video.load()
    pendingSeekTime = previousTime
    if (wasPlaying) video.play().catch(() => {})
    updateSubtitleOptions()
    updateModeButtons(currentMode)
  }

  const updatePlayPauseIcons = (isPlaying: boolean): void => {
    if (iconPlay && iconPause) {
      if (isPlaying) {
        iconPlay.classList.add('hidden')
        iconPause.classList.remove('hidden')
      } else {
        iconPlay.classList.remove('hidden')
        iconPause.classList.add('hidden')
      }
    }
  }

  const updateMuteIcons = (isMuted: boolean): void => {
    if (iconVolume && iconMuted) {
      if (isMuted) {
        iconVolume.classList.add('hidden')
        iconMuted.classList.remove('hidden')
      } else {
        iconVolume.classList.remove('hidden')
        iconMuted.classList.add('hidden')
      }
    }
  }

  const syncVolumeUI = (): void => {
    if (volumeRange) {
      const volumeValue = video.muted ? 0 : Math.round(video.volume * 100)
      volumeRange.value = String(volumeValue)
      volumeRange.style.setProperty('--volume-percent', `${volumeValue}%`)
    }
    if (!video.muted && video.volume > 0) {
      lastKnownVolume = video.volume
    }
    updateMuteIcons(video.muted || video.volume === 0)
  }

  const toggleDub = (): void => {
    if (availableModes.includes('dub')) {
      switchMode('dub')
    }
    showControls()
  }

  const toggleSub = (): void => {
    if (availableModes.includes('sub')) {
      switchMode('sub')
    }
    showControls()
  }

  const showControls = (): void => {
    container.classList.add('show-controls')
    window.clearTimeout(playerControlsTimeout)
    playerControlsTimeout = window.setTimeout(() => {
      if (!isScrubbing && !video.paused) {
        container.classList.remove('show-controls')
      }
    }, 2000)
  }

  const togglePlayPause = (): void => {
    if (video.paused) {
      video.play()
      return
    }

    video.pause()
  }

  const toggleFullscreen = (): void => {
    if (document.fullscreenElement) {
      if (document.exitFullscreen) document.exitFullscreen()
      return
    }

    if ('requestFullscreen' in container && typeof container.requestFullscreen === 'function') {
      container.requestFullscreen()
    }
  }

  // Initialize
  updateSubtitleOptions()
  updateModeButtons(currentMode)

  const startingURL = streamUrlForMode(currentMode)
  if (startingURL) {
    video.src = startingURL
  } else if (initialStreamToken) {
    video.src = `${streamURL}?mode=${encodeURIComponent(currentMode)}&token=${encodeURIComponent(initialStreamToken)}`
  }

  if (video) {
    video.addEventListener('loadedmetadata', () => {
      if (loading) loading.style.display = 'none'
      resolveActiveSegments()
      renderSegments()

      const startTimeSeconds = Number.parseFloat(container.getAttribute('data-start-time-seconds') || '0')
      const currentDisplayTime = displayTimeFromAbsolute(video.currentTime)
      if (Number.isFinite(startTimeSeconds) && startTimeSeconds > 0 && currentDisplayTime <= 0.5) {
        const nextStart = absoluteTimeFromDisplay(startTimeSeconds)
        if (nextStart > 0) {
          try {
            video.currentTime = nextStart
          } catch {}
        }
      }
      if (pendingSeekTime !== null && Number.isFinite(pendingSeekTime)) {
        try {
          video.currentTime = absoluteTimeFromDisplay(pendingSeekTime)
        } catch {}
        pendingSeekTime = null
      }
      if (shouldAutoPlay) {
        video.play().catch(() => {})
      }
      updateTimeline(video.currentTime)
      updateSkipButton(video.currentTime)
    })

    video.addEventListener('waiting', () => {
      if (loading) loading.style.display = 'flex'
    })

    video.addEventListener('playing', () => {
      if (loading) loading.style.display = 'none'
    })

    video.addEventListener('timeupdate', () => {
      updateTimeline(video.currentTime)
      updateSubtitleRender(displayTimeFromAbsolute(video.currentTime))
      updateSkipButton(video.currentTime)
      scheduleProgressSave()
    })

    video.addEventListener('play', () => {
      updatePlayPauseIcons(true)
      showControls()
    })

    video.addEventListener('pause', () => {
      updatePlayPauseIcons(false)
      showControls()
      window.clearTimeout(progressSaveTimer)
      progressSaveTimer = undefined
      saveProgress()
    })

    video.addEventListener('volumechange', () => {
      syncVolumeUI()
    })

    video.addEventListener('ended', () => {
      goToNextEpisode()
    })
  }

  

  const goToNextEpisode = async (): Promise<void> => {
    const currentEpNum = Number.parseInt(currentEpisode, 10)
    if (Number.isNaN(currentEpNum)) return

    if (Number.isInteger(totalEpisodes) && totalEpisodes > 0 && currentEpNum >= totalEpisodes) {
      completeAnime(currentEpNum)
      return
    }

    if (!isAutoplayEnabled()) return

    const nextEpisode = currentEpNum + 1
    markEpisodeTransition(nextEpisode)

    sessionStorage.setItem('mal:autoplay-next', 'true')
    const newUrl = new URL(window.location.href)
    newUrl.searchParams.set('ep', String(nextEpisode))
    window.location.href = newUrl.toString()
  }

  

  const completeAnime = async (episodeNumber: number): Promise<void> => {
    if (completionSent) return
    if (!Number.isInteger(malID) || malID <= 0) return
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    completionSent = true

    try {
      const response = await fetch('/api/watch-complete', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        keepalive: true,
        body: JSON.stringify({
          mal_id: malID,
          episode: episodeNumber,
        }),
      })

      if (!response.ok) {
        completionSent = false
        if (completionAttempts < 2) {
          completionAttempts += 1
          window.setTimeout(() => {
            completeAnime(episodeNumber)
          }, 1000)
        }
        return
      }

      const dropdownTrigger = document.querySelector('[data-dropdown-trigger]') as HTMLButtonElement | null
      if (dropdownTrigger) {
        dropdownTrigger.innerHTML = 'Completed <span class="text-xs">▾</span>'
      }

      const watchStatusDropdown = document.getElementById('watch-status-dropdown')
      if (watchStatusDropdown) {
        const payload = {
          anime_id: String(malID),
          anime_title: animeTitle,
          anime_title_english: animeTitleEnglish,
          anime_title_japanese: animeTitleJapanese,
          anime_image: animeImage,
          status: 'completed',
          airing: animeAiring,
        }

        fetch('/api/watchlist', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'HX-Request': 'true',
          },
          body: `anime_id=${encodeURIComponent(payload.anime_id)}&anime_title=${encodeURIComponent(payload.anime_title)}&anime_title_english=${encodeURIComponent(payload.anime_title_english)}&anime_title_japanese=${encodeURIComponent(payload.anime_title_japanese)}&anime_image=${encodeURIComponent(payload.anime_image)}&status=${encodeURIComponent(payload.status)}&airing=${encodeURIComponent(String(payload.airing))}`,
          credentials: 'same-origin',
        }).then(async (res) => {
          if (!res.ok) return
          if (!watchStatusDropdown || !watchStatusDropdown.isConnected) return
          const html = await res.text()
          const wrapper = document.createElement('span')
          wrapper.id = 'watch-status-dropdown'
          wrapper.innerHTML = DOMPurify.sanitize(html)
          watchStatusDropdown.replaceWith(wrapper)
        }).catch(() => {})
      }
    } catch {
      completionSent = false
      if (completionAttempts < 2) {
        completionAttempts += 1
        window.setTimeout(() => {
          completeAnime(episodeNumber)
        }, 1000)
      }
      return
    }
  }

  playPause?.addEventListener('click', () => {
    togglePlayPause()
    showControls()
  })

  video.addEventListener('click', () => {
    togglePlayPause()
    showControls()
  })

  muteBtn?.addEventListener('click', () => {
    if (video.muted || video.volume === 0) {
      const restoredVolume = lastKnownVolume > 0 ? lastKnownVolume : 1
      video.muted = false
      video.volume = restoredVolume
    } else {
      lastKnownVolume = video.volume > 0 ? video.volume : lastKnownVolume
      video.muted = true
    }
    showControls()
  })

  volumeRange?.addEventListener('input', () => {
    const sliderValue = Number(volumeRange.value)
    if (!Number.isFinite(sliderValue)) return
    const nextVolume = Math.max(0, Math.min(1, sliderValue / 100))
    video.volume = nextVolume
    video.muted = nextVolume === 0
    if (nextVolume > 0) {
      lastKnownVolume = nextVolume
    }
    showControls()
  })

  volumeRange?.addEventListener('pointerdown', () => {
    volumePanel?.classList.add('is-dragging')
  })

  window.addEventListener('pointerup', () => {
    volumePanel?.classList.remove('is-dragging')
  })

  backwardBtn?.addEventListener('click', () => seekBy(-10))
  forwardBtn?.addEventListener('click', () => seekBy(10))

  fullscreenBtn?.addEventListener('click', () => {
    toggleFullscreen()
    showControls()
  })

  skipSegmentBtn?.addEventListener('click', () => {
    if (!activeSkipSegment) return

    const target = absoluteTimeFromDisplay(activeSkipSegment.end + 0.01)
    video.currentTime = target

    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  })

  modeDub?.addEventListener('click', toggleDub)
  modeSub?.addEventListener('click', toggleSub)

  autoplayBtn?.addEventListener('change', (e) => {
    const isChecked = (e.target as HTMLInputElement).checked
    localStorage.setItem('mal:autoplay-enabled', isChecked ? 'true' : 'false')
    showControls()
  })

  subtitleSelect?.addEventListener('change', async () => {
    const selected = subtitleSelect.value
    if (selected === 'none') {
      activeSubtitles = []
      hideSubtitleText()
      return
    }
    const idx = Number(selected)
    const track = currentSubtitleTracks[idx]
    if (!track) {
      activeSubtitles = []
      return
    }
    activeSubtitles = await loadSubtitle(track.url)
  })

  progressWrap?.addEventListener('mousedown', (event) => {
    isScrubbing = true
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, ((event as MouseEvent).clientX - rect.left) / rect.width))

    video.currentTime = absoluteTimeFromRatio(ratio)

    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  })

  progressWrap?.addEventListener('mousemove', (event) => {
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
    updatePreviewUI(ratio)
  })

  progressWrap?.addEventListener('mouseleave', () => {
    hidePreviewPopover()
  })

  container.addEventListener('click', (event: Event) => {
    const target = event.target
    if (!(target instanceof Node)) return

    const targetElement = target instanceof Element ? target : target.parentElement
    if (!targetElement) return

    const anchor = targetElement.closest('a[href]')
    if (!(anchor instanceof HTMLAnchorElement)) return

    const nextEpisode = parseEpisodeFromWatchHref(anchor.href)
    if (nextEpisode === null) return
    markEpisodeTransition(nextEpisode)
  })

  window.addEventListener('mouseup', () => {
    isScrubbing = false
    saveProgress()
  })

  window.addEventListener('mousemove', (event) => {
    if (!isScrubbing || !progressWrap) return
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))

    video.currentTime = absoluteTimeFromRatio(ratio)

    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
  })

  container.addEventListener('mousemove', showControls)

  document.addEventListener('keydown', (event) => {
    const target = event.target as HTMLElement
    if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return
    if (event.code === 'Space') {
      event.preventDefault()
      togglePlayPause()
    }
    if (event.code === 'ArrowLeft') seekBy(-10)
    if (event.code === 'ArrowRight') seekBy(10)
    if (event.code === 'KeyM') video.muted = !video.muted
    if (event.code === 'KeyF') {
      toggleFullscreen()
    }
    showControls()
  })

  window.addEventListener('beforeunload', () => {
    if (transitionEpisode !== null) return
    if (completionSent) return
    if (!Number.isInteger(malID) || malID <= 0) return

    const bounds = timelineBounds()
    if (bounds.duration <= 0) return

    const episodeNumber = Number.parseInt(currentEpisode, 10)
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    const safeTime = displayTimeFromAbsolute(video.currentTime)
    const payload = buildWatchProgressPayload(episodeNumber, safeTime)
    sendWatchProgressBeacon(payload)
  })

  updatePlayPauseIcons(false)
  syncVolumeUI()
  updateSkipButton(0)
  updateAutoplayButton()
  showControls()

  playerInitialized = true
}

document.addEventListener('DOMContentLoaded', initPlayer)
document.body.addEventListener('htmx:afterSwap', (e: Event) => {
  const target = (e as CustomEvent).detail?.target as HTMLElement | null
  if (!target) return
  if (target.querySelector('[data-video-player]')) {
    initPlayer()
  }
})
