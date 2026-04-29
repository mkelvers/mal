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

const initSynopsisToggle = (): void => {
  document.addEventListener('click', (event: MouseEvent) => {
    const target = event.target
    if (!(target instanceof HTMLElement)) {
      return
    }

    const button = target.closest('[data-synopsis-toggle]')
    if (!(button instanceof HTMLElement)) {
      return
    }

    const container = button.parentElement
    if (!container) {
      return
    }

    const synopsis = container.querySelector('[data-synopsis]')
    if (!(synopsis instanceof HTMLElement)) {
      return
    }

    const expanded = synopsis.classList.toggle('line-clamp-none')
    synopsis.classList.toggle('max-md:line-clamp-3', !expanded)

    const moreLabel = button.getAttribute('data-label-more') ?? 'Read more'
    const lessLabel = button.getAttribute('data-label-less') ?? 'Show less'
    button.textContent = expanded ? lessLabel : moreLabel
  })
}

const initEpisodeListToggle = (): void => {
  document.addEventListener('click', (event: MouseEvent) => {
    const target = event.target
    if (!(target instanceof HTMLElement)) {
      return
    }

    const button = target.closest('[data-episodes-toggle]')
    if (!(button instanceof HTMLElement)) {
      return
    }

    const drawer = document.getElementById('episode-list-drawer')
    if (!drawer) {
      return
    }

    const isHidden = drawer.classList.toggle('hidden')

    const moreLabel = button.getAttribute('data-label-more') ?? 'SEE MORE EPISODES'
    const lessLabel = button.getAttribute('data-label-less') ?? 'SEE LESS'
    const labelSpan = button.querySelector('[data-toggle-label]')
    if (labelSpan) {
      labelSpan.textContent = isHidden ? moreLabel : lessLabel
    } else {
      button.textContent = isHidden ? moreLabel : lessLabel
    }
  })
}

initWatchlistDropdown()
initSynopsisToggle()
initEpisodeListToggle()
