;(function () {
  const contentSelector = '#schedule-content'

  const loadDay = (tab) => {
    const day = tab.getAttribute('data-day')
    if (!day || typeof htmx === 'undefined') {
      return
    }

    const tabs = document.querySelectorAll('[data-schedule-tab]')
    tabs.forEach((item) => item.classList.remove('active'))
    tab.classList.add('active')
    htmx.ajax('GET', `/api/schedule?day=${day}`, contentSelector)
  }

  document.addEventListener('click', (event) => {
    const target = event.target
    if (!(target instanceof Element)) {
      return
    }

    const tab = target.closest('[data-schedule-tab]')
    if (!tab) {
      return
    }

    loadDay(tab)
  })
})()
