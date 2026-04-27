declare const htmx: {
  ajax(verb: string, path: string, target: HTMLElement): Promise<void>
}

import DOMPurify from 'dompurify'
import type { ModeSource, SubtitleTrack, ParsedSegment, EpisodeData } from './types'
import { safeJsonParse, formatTime, parseEpisodeFromWatchHref } from './utils'
import { timelineBounds, displayTimeFromAbsolute, absoluteTimeFromDisplay, absoluteTimeFromRatio } from './timeline'
import { parseSegments, resolveActiveSegments, skipLabel, skipActivationTime } from './segments'
import { subtitlesForMode, loadSubtitle } from './subtitles'
import { buildWatchProgressPayload, sendWatchProgressBeacon, saveProgress, markEpisodeTransition } from './watch-progress'

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
  const initialStreamToken = container.getAttribute('data-stream-token') || ''
  let currentEpisode = container.getAttribute('data-current-episode') || '1'
  const malID = Number.parseInt(container.getAttribute('data-mal-id') || '', 10)
  let totalEpisodes = Number.parseInt(container.getAttribute('data-total-episodes') || '0', 10)
  const animeTitle = container.getAttribute('data-anime-title') || ''
  const animeTitleEnglish = container.getAttribute('data-anime-title-english') || ''
  const animeTitleJapanese = container.getAttribute('data-anime-title-japanese') || ''
  const animeImage = container.getAttribute('data-anime-image') || ''
  const animeAiring = (container.getAttribute('data-anime-airing') || '').toLowerCase() === 'true'

  const modeSources = safeJsonParse<Record<string, ModeSource>>(
    container.getAttribute('data-mode-sources'),
    {} as Record<string, ModeSource>
  )
  const availableModes = safeJsonParse<string[]>(
    container.getAttribute('data-available-modes'),
    [] as string[]
  )
  const initialMode = container.getAttribute('data-initial-mode') || 'dub'
  const segments = safeJsonParse<any[]>(
    container.getAttribute('data-segments'),
    [] as any[]
  )

  let parsedSegments = parseSegments(segments)
  let currentMode = availableModes.includes(initialMode) ? initialMode : (availableModes[0] || 'dub')
  const fallbackMode = Object.keys(modeSources).find(
    (mode: string) => typeof modeSources[mode]?.token === 'string' && modeSources[mode].token !== ''
  )
  if ((!modeSources[currentMode] || !modeSources[currentMode].token) && fallbackMode) {
    currentMode = fallbackMode
  }

  let controlsTimeout: number | undefined
  let isScrubbing = false
  let lastKnownVolume = 1
  let activeSubtitles: Array<{ start: number; end: number; text: string }> = []
  let currentSubtitleTracks: SubtitleTrack[] = []
  let pendingSeekTime: number | null = null
  let activeSkipSegment: { type: string; start: number; end: number } | null = null
  let activeSegments: ParsedSegment[] = []
  let lastSavedProgress = { episode: currentEpisode, seconds: -1 }
  let progressSaveTimer: number | undefined
  let transitionEpisode: number | null = null
  let completionSent = false
  let completionAttempts = 0

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

  const updateSkipButton = (currentTime: number): void => {
    const currentDisplayTime = displayTimeFromAbsolute(currentTime, video)
    const segment = activeSegments.find((item: ParsedSegment) => {
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

    const bounds = timelineBounds(video)
    if (bounds.duration <= 0) return

    activeSegments.forEach((segment: ParsedSegment) => {
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

  const updateTimeline = (currentTime: number): void => {
    if (!timeDisplay || !progress) return

    const bounds = timelineBounds(video)
    if (bounds.duration <= 0) {
      progress.style.width = '0%'
      if (scrubber) scrubber.style.left = '0%'
      timeDisplay.textContent = `00:00 / 00:00`
      return
    }

    const currentDisplayTime = displayTimeFromAbsolute(currentTime, video)
    const pct = Math.max(0, Math.min(100, (currentDisplayTime / bounds.duration) * 100))
    progress.style.width = `${pct}%`
    if (scrubber) scrubber.style.left = `${pct}%`
    timeDisplay.textContent = `${formatTime(currentDisplayTime)} / ${formatTime(bounds.duration)}`
  }

  const seekBy = (delta: number): void => {
    const bounds = timelineBounds(video)
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

    const bounds = timelineBounds(video)
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

  const scheduleProgressSave = (): void => {
    if (progressSaveTimer !== undefined) return
    progressSaveTimer = window.setTimeout(() => {
      progressSaveTimer = undefined
      saveProgress(video, malID, currentEpisode, lastSavedProgress).then((result) => {
        lastSavedProgress = result
      })
    }, 30000)
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
    currentSubtitleTracks = subtitlesForMode(currentMode, modeSources)
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
    const previousTime = displayTimeFromAbsolute(video.currentTime, video)
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
    window.clearTimeout(controlsTimeout)
    controlsTimeout = window.setTimeout(() => {
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
      resolveActiveSegments(parsedSegments, video)
      renderSegments()

      const startTimeSeconds = Number.parseFloat(container.getAttribute('data-start-time-seconds') || '0')
      const currentDisplayTime = displayTimeFromAbsolute(video.currentTime, video)
      if (Number.isFinite(startTimeSeconds) && startTimeSeconds > 0 && currentDisplayTime <= 0.5) {
        const nextStart = absoluteTimeFromDisplay(startTimeSeconds, video)
        if (nextStart > 0) {
          try {
            video.currentTime = nextStart
          } catch {}
        }
      }
      if (pendingSeekTime !== null && Number.isFinite(pendingSeekTime)) {
        try {
          video.currentTime = absoluteTimeFromDisplay(pendingSeekTime, video)
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
      updateSubtitleRender(displayTimeFromAbsolute(video.currentTime, video))
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
      saveProgress(video, malID, currentEpisode, lastSavedProgress).then((result) => {
        lastSavedProgress = result
      })
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
    const currentEpisodeNumber = Number.parseInt(pathParts[3], 10)
    if (Number.isNaN(currentEpisodeNumber)) return

    if (Number.isInteger(totalEpisodes) && totalEpisodes > 0 && currentEpisodeNumber >= totalEpisodes) {
      completeAnime(currentEpisodeNumber)
      return
    }

    const nextEpisode = currentEpisodeNumber + 1
    markEpisodeTransition(malID, nextEpisode)

    if (document.fullscreenElement) {
      loadNextEpisodeInPlace(Number(animeID), nextEpisode)
      return
    }

    const nextUrl = `/watch/${animeID}/${nextEpisode}`
    sessionStorage.setItem('mal:autoplay-next', 'true')
    window.location.href = nextUrl
  }

  const loadNextEpisodeInPlace = async (animeID: number, nextEpisode: number): Promise<void> => {
    if (!Number.isInteger(animeID) || animeID <= 0) return

    const url = `/api/watch/episode/${animeID}/${nextEpisode}`
    let data: EpisodeData | null = null

    try {
      const resp = await fetch(url)
      if (!resp.ok) return
      data = await resp.json() as EpisodeData
    } catch {
      return
    }

    if (!data) return

    const container = document.querySelector('[data-video-player]') as HTMLElement | null
    if (!container) return

    const video = container.querySelector('video') as HTMLVideoElement | null
    if (!video) return

    container.setAttribute('data-current-episode', String(nextEpisode))
    container.setAttribute('data-mal-id', String(animeID))
    container.setAttribute('data-total-episodes', String(data.total_episodes))
    container.setAttribute('data-start-time-seconds', '0')
    container.setAttribute('data-initial-mode', data.initial_mode)
    container.setAttribute('data-stream-token', data.token)
    container.setAttribute('data-available-modes', JSON.stringify(data.available_modes))
    container.setAttribute('data-mode-sources', JSON.stringify(data.mode_sources))
    container.setAttribute('data-segments', JSON.stringify(data.segments))

    currentEpisode = String(nextEpisode)
    totalEpisodes = data.total_episodes

    const newStreamURL = container.getAttribute('data-stream-url') || '/watch/proxy/stream'
    const streamMode = data.initial_mode
    const modeSource = data.mode_sources[streamMode]

    if (modeSource?.token) {
      video.src = `${newStreamURL}?mode=${encodeURIComponent(streamMode)}&token=${encodeURIComponent(modeSource.token)}`
    } else if (data.token) {
      video.src = `${newStreamURL}?mode=${encodeURIComponent(streamMode)}&token=${encodeURIComponent(data.token)}`
    }

    video.load()
    video.play().catch(() => {})

    parsedSegments = parseSegments(data.segments || [])
    activeSegments = []
    resolveActiveSegments(parsedSegments, video)
    renderSegments()
    updateSubtitleOptions()
    updateModeButtons(data.initial_mode)
    updateVideoOverlay(String(nextEpisode), data.episode_title)

    const nextUrl = `/watch/${animeID}/${nextEpisode}`
    window.history.replaceState(null, '', nextUrl)

    const episodesList = document.getElementById('episodes-list')
    if (episodesList) {
      htmx.ajax('GET', `/api/anime/${animeID}/episodes?current=${nextEpisode}`, episodesList)
    }
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

    const target = absoluteTimeFromDisplay(activeSkipSegment.end + 0.01, video)
    video.currentTime = target

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

    video.currentTime = absoluteTimeFromRatio(ratio, video)

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

    const nextEpisode = parseEpisodeFromWatchHref(anchor.href, malID)
    if (nextEpisode === null) return
    markEpisodeTransition(malID, nextEpisode)
  })

  window.addEventListener('mouseup', () => {
    isScrubbing = false
    saveProgress(video, malID, currentEpisode, lastSavedProgress).then((result) => {
      lastSavedProgress = result
    })
  })

  window.addEventListener('mousemove', (event) => {
    if (!isScrubbing || !progressWrap) return
    const rect = progressWrap.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))

    video.currentTime = absoluteTimeFromRatio(ratio, video)

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

    const bounds = timelineBounds(video)
    if (bounds.duration <= 0) return

    const episodeNumber = Number.parseInt(currentEpisode, 10)
    if (!Number.isInteger(episodeNumber) || episodeNumber <= 0) return

    const safeTime = displayTimeFromAbsolute(video.currentTime, video)
    const payload = buildWatchProgressPayload(malID, episodeNumber, safeTime)
    sendWatchProgressBeacon(payload)
  })

  updatePlayPauseIcons(false)
  syncVolumeUI()
  updateSkipButton(0)
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
