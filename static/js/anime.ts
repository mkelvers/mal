((): void => {
  const toggleDropdown = (): void => {
    const dropdown = document.getElementById('watchlist-dropdown')
    if (!dropdown) {
      return
    }

    dropdown.classList.toggle('open')
    const menu = dropdown.querySelector('[data-dropdown-menu]')
    if (menu instanceof HTMLElement) {
      menu.classList.toggle('invisible')
      menu.classList.toggle('opacity-0')
    }
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
      const menu = dropdown.querySelector('[data-dropdown-menu]')
      if (menu instanceof HTMLElement) {
        menu.classList.add('invisible')
        menu.classList.add('opacity-0')
      }
    }
  })
})()
