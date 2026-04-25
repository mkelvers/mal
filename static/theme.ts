type Theme = 'light' | 'dark'

const STORAGE_KEY = 'theme'

const getSavedTheme = (): Theme => {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === 'light' || raw === 'dark') return raw
  return 'dark'
}

const applyTheme = (theme: Theme): void => {
   const html = document.documentElement
   html.setAttribute('data-theme', theme)
   localStorage.setItem(STORAGE_KEY, theme)
   updateToggleButtons(theme)
 }

const cycleTheme = (): void => {
  const current = getSavedTheme()
  const next: Theme = current === 'light' ? 'dark' : 'light'
  applyTheme(next)
}

const updateToggleButtons = (theme: Theme): void => {
   const headerBtn = document.getElementById('theme-toggle')
   const footerBtn = document.getElementById('footer-theme-toggle')
   
   const updateButton = (btn: HTMLButtonElement | null): void => {
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
     } else {
       svg.innerHTML = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>'
       svg.setAttribute('stroke', 'currentColor')
       svg.setAttribute('fill', 'none')
     }
   }

   updateButton(headerBtn)
   updateButton(footerBtn)
 }

 const initTheme = (): void => {
   const saved = getSavedTheme()
   applyTheme(saved)

   const headerBtn = document.getElementById('theme-toggle')
   const footerBtn = document.getElementById('footer-theme-toggle')
   
   if (headerBtn) {
     headerBtn.addEventListener('click', cycleTheme)
   }
   
   if (footerBtn) {
     footerBtn.addEventListener('click', cycleTheme)
   }
 }

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initTheme)
} else {
  initTheme()
}
