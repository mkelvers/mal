((): void => {
  const toggleDropdown = (): void => {
    const dropdown = document.getElementById('watchlist-dropdown')
    if (!dropdown) {
      return
    }

    dropdown.classList.toggle('open')
  }

  ;(window as Window & { toggleDropdown?: () => void }).toggleDropdown = toggleDropdown

  document.addEventListener('click', (event: MouseEvent): void => {
    const dropdown = document.getElementById('watchlist-dropdown')
    if (!dropdown) {
      return
    }

    const target = event.target
    if (!(target instanceof Node)) {
      return
    }

    if (!dropdown.contains(target)) {
      dropdown.classList.remove('open')
    }
  })
})()
