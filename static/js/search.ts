export {}

type QuickSearchResult = {
  id?: number
  image?: string
  title?: string
  type?: string
}

const globalWindow = window as Window & { searchInitialized?: boolean }

let searchTimeout: number | undefined
const searchInput = document.getElementById('search-input') as HTMLInputElement | null
const searchDropdown = document.querySelector('[data-search-results-container]') as HTMLElement | null

const isSafeImageUrl = (rawUrl?: string): boolean => {
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

const clearSearchResults = (): void => {
  if (!searchDropdown) {
    return
  }

  searchDropdown.replaceChildren()
}

const buildSearchResultItem = (result: QuickSearchResult): HTMLAnchorElement => {
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
  return item
}

const renderQuickSearchResults = (query: string, results: QuickSearchResult[]): void => {
  if (!searchDropdown) {
    return
  }

  if (!results || results.length === 0) {
    clearSearchResults()
    return
  }

  const searchResults = document.createElement('div')
  searchResults.className = 'grid'

  const title = document.createElement('div')
  title.className = 'px-3 py-2 text-[0.68rem] text-[var(--text-faint)]'
  title.textContent = 'Anime'
  searchResults.appendChild(title)

  results.forEach((result: QuickSearchResult): void => {
    searchResults.appendChild(buildSearchResultItem(result))
  })

  const viewAll = document.createElement('a')
  viewAll.className = 'bg-[var(--surface-search-view-all)] px-3 py-2 text-center text-[0.8rem] text-[var(--text-muted)] no-underline hover:bg-[var(--panel-soft)] hover:text-[var(--text)] hover:no-underline'
  viewAll.setAttribute('href', '/search?q=' + encodeURIComponent(query))
  viewAll.textContent = 'View all results for ' + query
  searchResults.appendChild(viewAll)

  searchDropdown.replaceChildren(searchResults)
}

const fetchAndRenderQuickSearch = (query: string): void => {
  fetch('/api/search-quick?q=' + encodeURIComponent(query))
    .then((res: Response) => res.json())
    .then((results: QuickSearchResult[]): void => {
      renderQuickSearchResults(query, results)
    })
    .catch((err: unknown): void => {
      console.error('Search error:', err)
    })
}

const onSearchInput = (event: Event): void => {
  if (searchTimeout) {
    window.clearTimeout(searchTimeout)
  }

  const target = event.target
  if (!(target instanceof HTMLInputElement)) {
    return
  }

  const query = target.value.trim()
  if (query.length < 2) {
    clearSearchResults()
    return
  }

  searchTimeout = window.setTimeout((): void => {
    fetchAndRenderQuickSearch(query)
  }, 300)
}

const onSearchBlur = (): void => {
  window.setTimeout((): void => {
    clearSearchResults()
  }, 200)
}

const onDocumentClick = (event: MouseEvent): void => {
  const target = event.target
  if (!(target instanceof Element)) {
    return
  }

  if (!target.closest('[data-search-root]')) {
    clearSearchResults()
  }
}

const initQuickSearch = (): void => {
  if (globalWindow.searchInitialized) {
    return
  }
  globalWindow.searchInitialized = true

  if (!searchInput || !searchDropdown) {
    return
  }

  searchInput.addEventListener('input', onSearchInput)
  searchInput.addEventListener('blur', onSearchBlur)
  document.addEventListener('click', onDocumentClick)
}

initQuickSearch()
