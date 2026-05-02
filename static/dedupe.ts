const dedupe = (): void => {
  console.log('Dedupe running...')
  const seen = new Set<string>()
  const elements = document.querySelectorAll('[data-id]')
  console.log('Found elements:', elements.length)
  elements.forEach((item) => {
    const id = item.getAttribute('data-id')
    if (id && seen.has(id)) {
      console.log('Removing duplicate:', id)
      item.remove()
    } else if (id) {
      seen.add(id)
    }
  })
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', dedupe)
} else {
  dedupe()
}
// Also run on window load to be sure
window.addEventListener('load', dedupe)
