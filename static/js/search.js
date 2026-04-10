(function() {
  if (window.searchInitialized) return
  window.searchInitialized = true

  let searchTimeout
  const searchInput = document.getElementById('search-input')
  const searchDropdown = document.getElementById('search-dropdown')

  if (searchInput) {
    searchInput.addEventListener('input', function(e) {
      clearTimeout(searchTimeout)
      const query = e.target.value.trim()

      if (query.length < 2) {
        searchDropdown.replaceChildren()
        return
      }

      searchTimeout = setTimeout(() => {
        fetch('/api/search-quick?q=' + encodeURIComponent(query))
          .then(res => res.json())
          .then(results => {
            if (!results || results.length === 0) {
              searchDropdown.replaceChildren()
              return
            }

            const searchResults = document.createElement('div')
            searchResults.className = 'search-results'

            const title = document.createElement('div')
            title.className = 'search-results-title'
            title.textContent = 'Anime'
            searchResults.appendChild(title)

            results.forEach(r => {
              const item = document.createElement('a')
              item.className = 'search-result-item'
              item.setAttribute('href', '/anime/' + encodeURIComponent(String(r.id || '')))

              if (isSafeImageUrl(r.image)) {
                const img = document.createElement('img')
                img.className = 'search-result-thumb'
                img.setAttribute('src', r.image)
                img.setAttribute('alt', String(r.title || ''))
                item.appendChild(img)
              } else {
                const noImage = document.createElement('div')
                noImage.className = 'search-result-no-image'
                noImage.textContent = 'no image'
                item.appendChild(noImage)
              }

              const info = document.createElement('div')
              info.className = 'search-result-info'

              const itemTitle = document.createElement('div')
              itemTitle.className = 'search-result-title'
              itemTitle.textContent = String(r.title || '')
              info.appendChild(itemTitle)

              const itemType = document.createElement('div')
              itemType.className = 'search-result-type'
              itemType.textContent = String(r.type || '')
              info.appendChild(itemType)

              item.appendChild(info)
              searchResults.appendChild(item)
            })

            const viewAll = document.createElement('a')
            viewAll.className = 'search-result-view-all'
            viewAll.setAttribute('href', '/search?q=' + encodeURIComponent(query))
            viewAll.textContent = 'View all results for ' + query
            searchResults.appendChild(viewAll)

            searchDropdown.replaceChildren(searchResults)
          })
          .catch(err => console.error('Search error:', err))
      }, 300)
    })

    searchInput.addEventListener('blur', () => {
      setTimeout(() => {
        searchDropdown.replaceChildren()
      }, 200)
    })

    document.addEventListener('click', (e) => {
      if (!e.target.closest('.header-search-wrapper')) {
        searchDropdown.replaceChildren()
      }
    })
  }

  function isSafeImageUrl(rawUrl) {
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
