((): void => {
  const setActiveTab = (clickedTab: Element): void => {
    const group = clickedTab.closest('[data-tab-group="discover"]')
    if (!group) {
      return
    }

    const triggers = group.querySelectorAll('[data-tab-trigger]')
    triggers.forEach((tab: Element): void => tab.classList.remove('active'))
    clickedTab.classList.add('active')
  }

  document.addEventListener('click', (event: MouseEvent): void => {
    const target = event.target
    if (!(target instanceof Element)) {
      return
    }

    const trigger = target.closest('[data-tab-trigger]')
    if (!trigger) {
      return
    }

    setActiveTab(trigger)
  })
})()
