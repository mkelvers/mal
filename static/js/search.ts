((): void => {
  const globalWindow = window as Window & { searchInitialized?: boolean }
  if (globalWindow.searchInitialized) {
    return
  }
  globalWindow.searchInitialized = true

  let searchTimeout: number | undefined
  const searchInput = document.getElementById('search-input') as HTMLInputElement | null
  const searchDropdown = document.querySelector('[data-search-results-container]') as HTMLElement | null

  if (!searchInput || !searchDropdown) {
    return
  }

  searchInput.addEventListener('input', (event: Event): void => {
    if (searchTimeout) {
      window.clearTimeout(searchTimeout)
    }

    const target = event.target
    if (!(target instanceof HTMLInputElement)) {
      return
    }

    const query = target.value.trim()
    if (query.length < 2) {
      searchDropdown.replaceChildren()
      return
    }

    searchTimeout = window.setTimeout((): void => {
      fetch('/api/search-quick?q=' + encodeURIComponent(query))
        .then((res: Response) => res.json())
        .then((results: Array<{ id?: number; image?: string; title?: string; type?: string }>): void => {
          if (!results || results.length === 0) {
            searchDropdown.replaceChildren()
            return
          }

          const searchResults = document.createElement('div')
          searchResults.className = 'grid'

          const title = document.createElement('div')
          title.className = 'px-3 py-2 text-[0.68rem] text-[var(--text-faint)]'
          title.textContent = 'Anime'
          searchResults.appendChild(title)

          results.forEach((result): void => {
            const item = document.createElement('a')
            item.className = 'flex items-start gap-3 px-3 py-2 text-inherit no-underline hover:bg-[var(--panel-soft)] hover:no-underline'
            item.setAttribute('href', '/anime/' + encodeURIComponent(String(result.id || '')))

            if (isSafeImageUrl(result.image)) {
              const img = document.createElement('img')
              img.className = 'aspect-[2/3] w-[42px] shrink-0 object-cover bg-[var(--surface-thumb)]'
              img.setAttribute('src', result.image || '')
              img.setAttribute('alt', String(result.title || ''))
              item.appendChild(img)
            } else {
              const noImage = document.createElement('div')
              noImage.className = 'aspect-[2/3] w-[42px] shrink-0 bg-[var(--surface-thumb)] text-[0] text-transparent'
              noImage.textContent = 'no image'
              item.appendChild(noImage)
            }

            const info = document.createElement('div')
            info.className = 'grid min-w-0 gap-px'

            const itemTitle = document.createElement('div')
            itemTitle.className = 'line-clamp-1 text-[0.86rem] leading-[1.3] text-[var(--text)]'
            itemTitle.textContent = String(result.title || '')
            info.appendChild(itemTitle)

            const itemType = document.createElement('div')
            itemType.className = 'text-[0.67rem] text-[var(--text-faint)]'
            itemType.textContent = String(result.type || '')
            info.appendChild(itemType)

            item.appendChild(info)
            searchResults.appendChild(item)
          })

          const viewAll = document.createElement('a')
          viewAll.className = 'bg-[var(--surface-search-view-all)] px-3 py-2 text-center text-[0.8rem] text-[var(--text-muted)] no-underline hover:bg-[var(--panel-soft)] hover:text-[var(--text)] hover:no-underline'
          viewAll.setAttribute('href', '/search?q=' + encodeURIComponent(query))
          viewAll.textContent = 'View all results for ' + query
          searchResults.appendChild(viewAll)

          searchDropdown.replaceChildren(searchResults)
        })
        .catch((err: unknown): void => {
          console.error('Search error:', err)
        })
    }, 300)
  })

  searchInput.addEventListener('blur', (): void => {
    window.setTimeout((): void => {
      searchDropdown.replaceChildren()
    }, 200)
  })

  document.addEventListener('click', (event: MouseEvent): void => {
    const target = event.target
    if (!(target instanceof Element)) {
      return
    }

    if (!target.closest('[data-search-root]')) {
      searchDropdown.replaceChildren()
    }
  })

  function isSafeImageUrl(rawUrl?: string): boolean {
    if (!rawUrl || typeof rawUrl !== 'string') {
      return false
    }

    try {
      const parsed = new URL(rawUrl, window.location.origin)
      return parsed.protocol === 'https:' || parsed.protocol === 'http:'
    } catch {
      return false
    }
  }
})()
