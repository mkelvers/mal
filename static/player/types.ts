export interface ModeSource {
  token: string
  subtitles: SubtitleItem[]
}

export interface SubtitleItem {
  lang: string
  token: string
}

export interface SkipSegment {
  type: string
  start: number
  end: number
}

export interface EpisodeData {
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

export interface SubtitleTrack {
  lang: string
  label: string
  url: string
}

export interface ParsedSegment {
  type: string
  start: number
  end: number
}

export interface TimelineBounds {
  start: number
  end: number
  duration: number
}

export interface WatchProgressPayload {
  mal_id: number
  episode: number
  time_seconds: number
}
