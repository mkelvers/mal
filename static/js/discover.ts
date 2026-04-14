export {}

const parseClassList = (value: string | null): string[] => {
  if (!value) {
    return []
  }

  return value
    .split(' ')
    .map((entry: string): string => entry.trim())
    .filter((entry: string): boolean => entry.length > 0)
}

const setActiveDiscoverTab = (clickedTab: Element): void => {
  const group = clickedTab.closest('[data-tab-group="discover"]')
  if (!group) {
    return
  }

  const triggers = group.querySelectorAll('[data-tab-trigger]')
  triggers.forEach((tab: Element): void => {
    const activeClasses = parseClassList(tab.getAttribute('data-tab-active-classes'))
    const inactiveClasses = parseClassList(tab.getAttribute('data-tab-inactive-classes'))
    tab.classList.remove(...activeClasses)
    tab.classList.add(...inactiveClasses)
  })

  const activeClasses = parseClassList(clickedTab.getAttribute('data-tab-active-classes'))
  const inactiveClasses = parseClassList(clickedTab.getAttribute('data-tab-inactive-classes'))
  clickedTab.classList.remove(...inactiveClasses)
  clickedTab.classList.add(...activeClasses)
}

const onDiscoverTabClick = (event: MouseEvent): void => {
  const target = event.target
  if (!(target instanceof Element)) {
    return
  }

  const trigger = target.closest('[data-tab-trigger]')
  if (!trigger) {
    return
  }

  setActiveDiscoverTab(trigger)
}

const initDiscoverTabs = (): void => {
  document.addEventListener('click', onDiscoverTabClick)
}

initDiscoverTabs()
