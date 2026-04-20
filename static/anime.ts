import { parseClassList } from './utils'

const setDropdownMenuState = (menu: HTMLElement, isOpen: boolean): void => {
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

const setWatchlistDropdownState = (isOpen: boolean): void => {
  const dropdown = document.getElementById('watchlist-dropdown')
  if (!dropdown) {
    return
  }

  dropdown.classList.toggle('open', isOpen)
  const menu = dropdown.querySelector('[data-dropdown-menu]')
  if (menu instanceof HTMLElement) {
    setDropdownMenuState(menu, isOpen)
  }
}

const toggleWatchlistDropdown = (): void => {
  const dropdown = document.getElementById('watchlist-dropdown')
  if (!dropdown) {
    return
  }

  setWatchlistDropdownState(!dropdown.classList.contains('open'))
}

const closeDropdownOnOutsideClick = (event: MouseEvent): void => {
  const dropdown = document.getElementById('watchlist-dropdown')
  if (!dropdown) {
    return
  }

  const target = event.target
  if (!(target instanceof Node)) {
    return
  }

  if (!dropdown.contains(target)) {
    setWatchlistDropdownState(false)
  }
}

const initWatchlistDropdown = (): void => {
  ;(window as Window & { toggleDropdown?: () => void }).toggleDropdown = toggleWatchlistDropdown
  document.addEventListener('click', closeDropdownOnOutsideClick)
}

initWatchlistDropdown()
