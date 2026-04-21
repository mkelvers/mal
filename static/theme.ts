type Theme = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'theme'

const getSavedTheme = (): Theme => {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === 'light' || raw === 'dark') return raw
  return 'system'
}

const getEffectiveTheme = (theme: Theme): 'light' | 'dark' => {
  if (theme !== 'system') return theme
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const applyTheme = (theme: Theme): void => {
  const html = document.documentElement
  if (theme === 'system') {
    html.removeAttribute('data-theme')
  } else {
    html.setAttribute('data-theme', theme)
  }
  localStorage.setItem(STORAGE_KEY, theme)
  updateToggleButton(theme)
}

const cycleTheme = (): void => {
  const current = getSavedTheme()
  const next: Theme = current === 'system' ? 'light' : current === 'light' ? 'dark' : 'system'
  applyTheme(next)
}

const updateToggleButton = (theme: Theme): void => {
  const btn = document.getElementById('theme-toggle')
  if (!btn) return

  const label = btn.querySelector('[data-theme-label]') as HTMLElement | null
  if (label) {
    label.textContent = theme
  }

  const svg = btn.querySelector('svg')
  if (!svg) return

  if (theme === 'light') {
    svg.innerHTML = '<circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>'
    svg.setAttribute('stroke', 'currentColor')
    svg.setAttribute('fill', 'none')
  } else if (theme === 'dark') {
    svg.innerHTML = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>'
    svg.setAttribute('stroke', 'currentColor')
    svg.setAttribute('fill', 'none')
  } else {
    svg.innerHTML = '<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/>'
    svg.setAttribute('stroke', 'currentColor')
    svg.setAttribute('fill', 'none')
  }
}

const initTheme = (): void => {
  const saved = getSavedTheme()
  applyTheme(saved)

  const btn = document.getElementById('theme-toggle')
  if (btn) {
    btn.addEventListener('click', cycleTheme)
  }

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (getSavedTheme() === 'system') {
      applyTheme('system')
    }
  })
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initTheme)
} else {
  initTheme()
}
