const dedupe = (): void => {
  const script = document.currentScript as HTMLScriptElement | null
  if (!script) return
  const containerId = script.getAttribute('data-container')
  const container = containerId ? document.getElementById(containerId) : document
  if (!container) return
  const seen = new Set<string>()
  container.querySelectorAll('[data-id]').forEach((item) => {
    const id = item.getAttribute('data-id')
    if (id && seen.has(id)) {
      item.remove()
    } else if (id) {
      seen.add(id)
    }
  })
}

dedupe()
