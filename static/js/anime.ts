((): void => {
  const parseClassList = (value: string | null): string[] => {
    if (!value) {
      return []
    }

    return value
      .split(' ')
      .map((entry: string): string => entry.trim())
      .filter((entry: string): boolean => entry.length > 0)
  }

  const setMenuState = (menu: HTMLElement, isOpen: boolean): void => {
    const openClasses = parseClassList(menu.getAttribute('data-dropdown-open-classes'))
    const closedClasses = parseClassList(menu.getAttribute('data-dropdown-closed-classes'))

    if (isOpen) {
      menu.classList.remove(...closedClasses)
      menu.classList.add(...openClasses)
      return
    }

    menu.classList.remove(...openClasses)
    menu.classList.add(...closedClasses)
  }

  const toggleDropdown = (): void => {
    const dropdown = document.getElementById('watchlist-dropdown')
    if (!dropdown) {
      return
    }

    const isOpen = !dropdown.classList.contains('open')
    dropdown.classList.toggle('open', isOpen)
    const menu = dropdown.querySelector('[data-dropdown-menu]')
    if (menu instanceof HTMLElement) {
      setMenuState(menu, isOpen)
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
        setMenuState(menu, false)
      }
    }
  })
})()
