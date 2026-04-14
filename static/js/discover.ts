((): void => {
  const setActiveTab = (clickedTab: Element): void => {
    const group = clickedTab.closest('[data-tab-group="discover"]')
    if (!group) {
      return
    }

    const triggers = group.querySelectorAll('[data-tab-trigger]')
    triggers.forEach((tab: Element): void => {
      tab.classList.add('tab-trigger')
      tab.classList.remove('bg-[var(--surface-tab-active)]', 'text-[var(--accent)]')
      tab.classList.add('bg-[var(--panel-soft)]', 'text-[var(--text-muted)]')
    })
    clickedTab.classList.add('tab-trigger')
    clickedTab.classList.remove('bg-[var(--panel-soft)]', 'text-[var(--text-muted)]')
    clickedTab.classList.add('bg-[var(--surface-tab-active)]', 'text-[var(--accent)]')
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
