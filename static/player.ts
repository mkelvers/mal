export {}

interface ModeSource {
  url: string
  referer: string
  subtitles: SubtitleItem[]
}

interface SubtitleItem {
  lang: string
  url: string
  referer: string
}

interface SkipSegment {
  type: string
  start: number
  end: number
}

interface PreviewCue {
  start: number
  end: number
  sprite: string
  x: number
  y: number
  width: number
  height: number
}

interface PreviewMap {
  width: number
  height: number
  columns: number
  rows: number
  interval: number
  duration: number
  cues: PreviewCue[]
}

interface PreviewPayload {
  spriteURL: string
  map: PreviewMap
}

interface PreviewMapResponse {
  sprite_url: string
  map: PreviewMap
}

const isObjectRecord = (value: unknown): value is Record<string, unknown> => {
  return typeof value === 'object' && value !== null
}

const isPreviewCue = (value: unknown): value is PreviewCue => {
  if (!isObjectRecord(value)) return false
  return Number.isFinite(value.start)
    && Number.isFinite(value.end)
    && typeof value.sprite === 'string'
    && Number.isFinite(value.x)
    && Number.isFinite(value.y)
    && Number.isFinite(value.width)
    && Number.isFinite(value.height)
}

const isPreviewMap = (value: unknown): value is PreviewMap => {
  if (!isObjectRecord(value)) return false
  if (!Array.isArray(value.cues)) return false
  if (!value.cues.every((cue: unknown) => isPreviewCue(cue))) return false
  return Number.isFinite(value.width)
    && Number.isFinite(value.height)
    && Number.isFinite(value.columns)
    && Number.isFinite(value.rows)
    && Number.isFinite(value.interval)
    && Number.isFinite(value.duration)
}

const parsePreviewMapResponse = (value: unknown): PreviewMapResponse | null => {
  if (!isObjectRecord(value)) return null
  if (typeof value.sprite_url !== 'string') return null
  if (!isPreviewMap(value.map)) return null
  return {
    sprite_url: value.sprite_url,
    map: value.map,
  }
}

const initPlayer = (): void => {
  const container = document.querySelector('[data-video-player]')
  if (!container) return

  const video = container.querySelector('video') as HTMLVideoElement
  const loading = container.querySelector('[data-loading]') as HTMLElement
  const playPause = container.querySelector('[data-play-pause]') as HTMLButtonElement
  const iconPlay = container.querySelector('[data-icon-play]') as SVGElement
  const iconPause = container.querySelector('[data-icon-pause]') as SVGElement
  const muteBtn = container.querySelector('[data-mute]') as HTMLButtonElement
  const volumeWrap = container.querySelector('[data-volume-wrap]') as HTMLElement
  const volumePanel = container.querySelector('.volume-panel') as HTMLElement
  const volumeRange = container.querySelector('[data-volume-range]') as HTMLInputElement
  const iconVolume = container.querySelector('[data-icon-volume]') as SVGElement
  const iconMuted = container.querySelector('[data-icon-muted]') as SVGElement
  const timeDisplay = container.querySelector('[data-time]') as HTMLElement
  const progressWrap = container.querySelector('[data-progress-wrap]') as HTMLElement
  const progress = container.querySelector('[data-progress]') as HTMLElement
  const scrubber = container.querySelector('[data-scrubber]') as HTMLElement
  const segmentsTrack = container.querySelector('[data-segments]') as HTMLElement
  const subtitleSelect = container.querySelector('[data-subtitle-select]') as HTMLSelectElement
  const backwardBtn = container.querySelector('[data-backward]') as HTMLButtonElement
  const forwardBtn = container.querySelector('[data-forward]') as HTMLButtonElement
  const fullscreenBtn = container.querySelector('[data-fullscreen]') as HTMLButtonElement
  const skipSegmentBtn = container.querySelector('[data-skip]') as HTMLButtonElement
  const subtitleText = container.querySelector('[data-subtitle-text]') as HTMLElement

  const streamURL = container.getAttribute('data-stream-url') || '/watch/proxy/stream'
  const previewMapURL = container.getAttribute('data-preview-map-url') || '/watch/proxy/preview-map'
  const currentEpisode = container.getAttribute('data-current-episode') || '1'
  const malID = Number.parseInt(container.getAttribute('data-mal-id') || '', 10)
  const startTimeSeconds = Number.parseFloat(container.getAttribute('data-start-time-seconds') || '0')
  const modeSources = JSON.parse(container.getAttribute('data-mode-sources') || '{}')
  const availableModes = JSON.parse(container.getAttribute('data-available-modes') || '[]')
  const initialMode = container.getAttribute('data-initial-mode') || 'dub'
  const segments = JSON.parse(container.getAttribute('data-segments') || '[]')
  const malIDFromPath = (() => {
    const pathParts = window.location.pathname.split('/').filter(Boolean)
    if (pathParts.length < 2) return ''
    if (pathParts[0] !== 'watch') return ''
    return pathParts[1] || ''
  })()

  const maxIntroStartSeconds = 180
  const minOutroStartRatio = 0.5
  const minSegmentDurationSeconds = 20
  const maxSegmentDurationSeconds = 240

  const parsedSegments = segments
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
    .filter((s: unknown): s is { type: string, start: number, end: number } => s !== null)
    .sort((a: { start: number }, b: { start: number }) => a.start - b.start)

  let currentMode = availableModes.includes(initialMode) ? initialMode : (availableModes[0] || 'dub')
  let controlsTimeout: number | undefined
  let isScrubbing = false
  let isHoveringVolume = false
  let lastKnownVolume = 1
  let activeSubtitles: Array<{ start: number, end: number, text: string }> = []
  let currentSubtitleTracks: Array<{ lang: string, label: string, url: string }> = []
  let pendingSeekTime: number | null = null
  let activeSkipSegment: { type: string, start: number, end: number } | null = null
  let activeSegments: Array<{ type: string, start: number, end: number }> = []
  let previewState: { [key: string]: PreviewPayload } = {}
  let previewRequestToken = 0
  let lastSavedProgress = { episode: currentEpisode, seconds: -1 }
  let progressSaveTimer: number | undefined

  const previewPopover = container.querySelector('[data-preview-popover]') as HTMLElement
  const previewFrame = container.querySelector('[data-preview-frame]') as HTMLElement
  const previewTime = container.querySelector('[data-preview-time]') as HTMLElement

  const streamUrlForMode = (mode: string): string => {
    const modeParam = encodeURIComponent(mode)
    const stateParam = encodeURIComponent(JSON.stringify(modeSources))
    return `${streamURL}?mode=${modeParam}&state=${stateParam}`
  }

  const subtitleProxyURL = (track: SubtitleItem): string => {
    if (!track || !track.url) return ''
    let proxied = `/watch/proxy/subtitle?u=${encodeURIComponent(track.url)}`
    if (track.referer) {
      proxied += `&r=${encodeURIComponent(track.referer)}`
    }
    return proxied
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

  const resolveActiveSegments = (): void => {
    if (!Number.isFinite(video.duration) || video.duration <= 0) {
      activeSegments = []
      return
    }
    activeSegments = parsedSegments.filter((segment: { start: number, end: number, type: string }) => {
      const start = segment.start
      const end = segment.end
      const segmentDuration = end - start
      if (segmentDuration < minSegmentDurationSeconds || segmentDuration > maxSegmentDurationSeconds) return false
      if (start < 0 || end <= start || end > video.duration + 1) return false
      if (segment.type === 'op') {
        if (start > maxIntroStartSeconds) return false
        if (start > video.duration * 0.5) return false
        return true
      }
      if (segment.type === 'ed') {
        return start >= video.duration * minOutroStartRatio
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
    const segment = activeSegments.find((item: { start: number, end: number }) => {
      const activationTime = skipActivationTime(item)
      return currentTime >= activationTime && currentTime < item.end
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
    if (!video.duration || !Number.isFinite(video.duration)) return
    activeSegments.forEach((segment: { start: number, end: number }) => {
      const left = (segment.start / video.duration) * 100
      const width = ((segment.end - segment.start) / video.duration) * 100
      const bar = document.createElement('div')
      bar.className = 'absolute top-0 h-full bg-yellow-400'
      bar.style.left = `${left}%`
      bar.style.width = `${width}%`
      segmentsTrack.appendChild(bar)
    })
  }

  const formatTime = (seconds: number): string => {
    if (!Number.isFinite(seconds) || seconds < 0) return '00:00'
    const mins = Math.floor(seconds / 60)
    const secs = Math.floor(seconds % 60)
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  }

  const updateTimeline = (currentTime: number): void => {
    if (!timeDisplay || !progress) return
    if (!video.duration || !Number.isFinite(video.duration)) {
      progress.style.width = '0%'
      if (scrubber) scrubber.style.left = '0%'
      timeDisplay.textContent = `00:00 / 00:00`
      return
    }
    const pct = Math.max(0, Math.min(100, (currentTime / video.duration) * 100))
    progress.style.width = `${pct}%`
    if (scrubber) scrubber.style.left = `${pct}%`
    timeDisplay.textContent = `${formatTime(currentTime)} / ${formatTime(video.duration)}`
  }

  const seekBy = (delta: number): void => {
    if (!Number.isFinite(video.duration)) return
    const next = Math.max(0, Math.min(video.duration, video.currentTime + delta))
    video.currentTime = next
    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  }

  const streamSourceForMode = (mode: string): { url: string, referer: string } | null => {
    const modeSource = modeSources[mode]
    if (!modeSource) return null
    const sourceURL = String(modeSource.url || '').trim()
    if (sourceURL === '') return null
    return {
      url: sourceURL,
      referer: String(modeSource.referer || ''),
    }
  }

  const previewCacheKey = (mode: string, sourceURL: string, sourceReferer: string): string => {
    const normalizedReferer = sourceReferer.trim()
    return `${mode}|${sourceURL}|${normalizedReferer}`
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

  const cueForTime = (map: PreviewMap, time: number): PreviewCue | null => {
    if (!map.cues.length) return null
    const match = map.cues.find((cue: PreviewCue) => time >= cue.start && time < cue.end)
    if (match) return match
    const first = map.cues[0]
    const last = map.cues[map.cues.length - 1]
    if (time <= first.start) return first
    if (time >= last.end) return last
    return null
  }

  const updatePreviewUI = (ratio: number): void => {
    if (!progressWrap || !previewPopover || !previewFrame || !previewTime) return
    if (!video.duration || !Number.isFinite(video.duration)) {
      hidePreviewPopover()
      return
    }

    const targetTime = Math.max(0, Math.min(video.duration, ratio * video.duration))
    previewTime.textContent = formatTime(targetTime)

    const source = streamSourceForMode(currentMode)
    if (!source || malIDFromPath === '') {
      hidePreviewPopover()
      return
    }

    const cacheKey = previewCacheKey(currentMode, source.url, source.referer)
    const cached = previewState[cacheKey]
    if (!cached || !cached.map || !cached.spriteURL) {
      hidePreviewPopover()
      return
    }

    const cue = cueForTime(cached.map, targetTime)
    if (!cue) {
      hidePreviewPopover()
      return
    }

    previewFrame.style.width = `${cue.width}px`
    previewFrame.style.height = `${cue.height}px`
    previewFrame.style.backgroundImage = `url('${cached.spriteURL}')`
    previewFrame.style.backgroundRepeat = 'no-repeat'
    previewFrame.style.backgroundPosition = `-${cue.x}px -${cue.y}px`
    previewFrame.style.backgroundSize = `${cached.map.columns * cue.width}px ${cached.map.rows * cue.height}px`

    const barWidth = progressWrap.clientWidth
    const popoverOffset = ratio * barWidth
    const halfWidth = cue.width / 2
    const clampedOffset = Math.max(halfWidth, Math.min(barWidth - halfWidth, popoverOffset))
    previewPopover.style.left = `${clampedOffset}px`

    showPreviewPopover()
  }

  const loadPreviewMap = async (): Promise<void> => {
    if (!video.duration || !Number.isFinite(video.duration)) return
    const source = streamSourceForMode(currentMode)
    if (!source || malIDFromPath === '') return

    const cacheKey = previewCacheKey(currentMode, source.url, source.referer)
    if (previewState[cacheKey]) return

    const token = previewRequestToken + 1
    previewRequestToken = token

    const query = new URLSearchParams({
      mal_id: malIDFromPath,
      ep: currentEpisode,
      mode: currentMode,
      u: source.url,
      r: source.referer,
      d: String(video.duration),
    })

    try {
      const response = await fetch(`${previewMapURL}?${query.toString()}`)
      if (!response.ok) return
      const payloadRaw: unknown = await response.json()
      if (token !== previewRequestToken) return
      const payload = parsePreviewMapResponse(payloadRaw)
      if (!payload) return

      previewState = {
        ...previewState,
        [cacheKey]: {
          spriteURL: payload.sprite_url,
          map: payload.map,
        },
      }
    } catch {
      return
    }
  }

  const saveProgress = async (): Promise<void> => {
    if (!Number.isInteger(malID) || malID <= 0) return
    if (!video.duration || !Number.isFinite(video.duration)) return
    const episodeNumber = Number.parseInt(currentEpisode, 10)
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    const safeTime = Math.max(0, Math.min(video.currentTime, video.duration))
    if (lastSavedProgress.episode === currentEpisode && Math.abs(lastSavedProgress.seconds - safeTime) < 2) {
      return
    }

    try {
      const response = await fetch('/api/watch-progress', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          mal_id: malID,
          episode: episodeNumber,
          time_seconds: safeTime,
        }),
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
    }, 1500)
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

  const updateSubtitleRender = (currentTime: number): void => {
    if (!subtitleText) return
    if (!activeSubtitles.length) {
      subtitleText.textContent = ''
      subtitleText.classList.remove('block')
      subtitleText.classList.add('hidden')
      return
    }
    const cue = activeSubtitles.find(item => currentTime >= item.start && currentTime <= item.end)
    if (!cue) {
      subtitleText.textContent = ''
      subtitleText.classList.remove('block')
      subtitleText.classList.add('hidden')
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
    if (subtitleText) {
      subtitleText.textContent = ''
      subtitleText.classList.remove('block')
      subtitleText.classList.add('hidden')
    }
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
    const wasPlaying = !video.paused
    const previousTime = video.currentTime
    currentMode = mode
    previewRequestToken += 1
    hidePreviewPopover()
    video.src = streamUrlForMode(currentMode)
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
    window.clearTimeout(controlsTimeout)
    controlsTimeout = window.setTimeout(() => {
      if (!isScrubbing && !isHoveringVolume && !video.paused) {
        container.classList.remove('show-controls')
      }
    }, 2000)
  }

  // Initialize
  updateSubtitleOptions()
  updateModeButtons(currentMode)

  if (video) {
    video.src = streamUrlForMode(currentMode)

    video.addEventListener('loadedmetadata', () => {
      if (loading) loading.style.display = 'none'
      resolveActiveSegments()
      renderSegments()
      if (Number.isFinite(startTimeSeconds) && startTimeSeconds > 0 && video.currentTime === 0) {
        const nextStart = Math.min(startTimeSeconds, Math.max(0, video.duration - 0.5))
        if (nextStart > 0) {
          try {
            video.currentTime = nextStart
          } catch {}
        }
      }
      if (pendingSeekTime !== null && Number.isFinite(pendingSeekTime)) {
        try {
          video.currentTime = pendingSeekTime
        } catch {}
        pendingSeekTime = null
      }
      updateTimeline(video.currentTime)
      updateSkipButton(video.currentTime)
      loadPreviewMap()
    })

    video.addEventListener('waiting', () => {
      if (loading) loading.style.display = 'flex'
    })

    video.addEventListener('playing', () => {
      if (loading) loading.style.display = 'none'
    })

    video.addEventListener('timeupdate', () => {
      updateTimeline(video.currentTime)
      updateSubtitleRender(video.currentTime)
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

  const goToNextEpisode = (): void => {
    const pathParts = window.location.pathname.split('/')
    if (pathParts.length < 4) return

    const animeID = pathParts[2]
    const currentEpisode = Number.parseInt(pathParts[3], 10)
    if (Number.isNaN(currentEpisode)) return

    const nextEpisode = currentEpisode + 1
    const nextUrl = `/watch/${animeID}/${nextEpisode}`

    window.location.href = nextUrl
  }

  playPause?.addEventListener('click', () => {
    if (video.paused) {
      video.play()
    } else {
      video.pause()
    }
    showControls()
  })

  video.addEventListener('click', () => {
    if (video.paused) {
      video.play()
    } else {
      video.pause()
    }
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

  const setVolumePanelOpen = (isOpen: boolean): void => {
    if (volumePanel) {
      volumePanel.classList.toggle('is-visible', isOpen)
    }
    volumeWrap?.classList.toggle('is-volume-open', isOpen)
    isHoveringVolume = isOpen
    if (isOpen) showControls()
  }

  const openVolumePanel = (): void => {
    setVolumePanelOpen(true)
  }

  const closeVolumePanel = (): void => {
    setVolumePanelOpen(false)
  }

  closeVolumePanel()

  muteBtn?.addEventListener('mouseenter', openVolumePanel)

  volumeWrap?.addEventListener('mouseleave', closeVolumePanel)

  volumeWrap?.addEventListener('focusin', openVolumePanel)

  volumeWrap?.addEventListener('focusout', (event: FocusEvent) => {
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && volumeWrap.contains(nextTarget)) return
    closeVolumePanel()
  })

  backwardBtn?.addEventListener('click', () => seekBy(-10))
  forwardBtn?.addEventListener('click', () => seekBy(10))

  fullscreenBtn?.addEventListener('click', () => {
    if (document.fullscreenElement) {
      document.exitFullscreen()
    } else {
      container.requestFullscreen()
    }
    showControls()
  })

  skipSegmentBtn?.addEventListener('click', () => {
    if (!activeSkipSegment) return
    const target = activeSkipSegment.end + 0.01
    if (Number.isFinite(video.duration)) {
      video.currentTime = Math.min(video.duration, target)
    } else {
      video.currentTime = target
    }
    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  })

  modeDub?.addEventListener('click', toggleDub)
  modeSub?.addEventListener('click', toggleSub)

  subtitleSelect?.addEventListener('change', async () => {
    const selected = subtitleSelect.value
    if (selected === 'none') {
      activeSubtitles = []
      if (subtitleText) {
        subtitleText.textContent = ''
        subtitleText.classList.remove('block')
        subtitleText.classList.add('hidden')
      }
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
    if (Number.isFinite(video.duration)) {
      video.currentTime = ratio * video.duration
    }
    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
    showControls()
  })

  progressWrap?.addEventListener('mouseenter', () => {
    loadPreviewMap()
  })

  progressWrap?.addEventListener('mousemove', (event) => {
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
    updatePreviewUI(ratio)
  })

  progressWrap?.addEventListener('mouseleave', () => {
    hidePreviewPopover()
  })

  window.addEventListener('mouseup', () => {
    isScrubbing = false
    saveProgress()
  })

  window.addEventListener('mousemove', (event) => {
    if (!isScrubbing || !progressWrap) return
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
    if (Number.isFinite(video.duration)) {
      video.currentTime = ratio * video.duration
    }
    updateTimeline(video.currentTime)
    updateSkipButton(video.currentTime)
  })

  container.addEventListener('mousemove', showControls)

  document.addEventListener('keydown', (event) => {
    if (event.code === 'Space') {
      event.preventDefault()
      if (video.paused) {
        video.play()
      } else {
        video.pause()
      }
    }
    if (event.code === 'ArrowLeft') seekBy(-10)
    if (event.code === 'ArrowRight') seekBy(10)
    if (event.code === 'KeyM') video.muted = !video.muted
    if (event.code === 'KeyF') {
      if (document.fullscreenElement) {
        document.exitFullscreen()
      } else {
        container.requestFullscreen()
      }
    }
    showControls()
  })

  window.addEventListener('beforeunload', () => {
    if (!Number.isInteger(malID) || malID <= 0) return
    if (!video.duration || !Number.isFinite(video.duration)) return
    const episodeNumber = Number.parseInt(currentEpisode, 10)
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return
    const safeTime = Math.max(0, Math.min(video.currentTime, video.duration))
    const payload = JSON.stringify({
      mal_id: malID,
      episode: episodeNumber,
      time_seconds: safeTime,
    })
    if (navigator.sendBeacon) {
      const blob = new Blob([payload], { type: 'application/json' })
      navigator.sendBeacon('/api/watch-progress', blob)
    }
  })

  updatePlayPauseIcons(false)
  syncVolumeUI()
  updateSkipButton(0)
  showControls()
}

document.addEventListener('DOMContentLoaded', initPlayer)
