;(function () {
  const toggleDropdown = () => {
    const dropdown = document.getElementById('watchlist-dropdown')
    if (!dropdown) {
      return
    }

    dropdown.classList.toggle('open')
  }

  window.toggleDropdown = toggleDropdown

  const toggleRelationExtras = (showExtras) => {
    const extras = document.querySelectorAll('.relation-extra')
    extras.forEach((item) => {
      item.classList.toggle('is-hidden', !showExtras)
    })

    const mainTab = document.getElementById('relations-main-tab')
    const extraTab = document.getElementById('relations-extra-tab')
    if (mainTab) {
      mainTab.classList.toggle('active', !showExtras)
    }
    if (extraTab) {
      extraTab.classList.toggle('active', showExtras)
    }
  }

  window.toggleRelationExtras = toggleRelationExtras
  window.addEventListener('load', () => toggleRelationExtras(false))

  document.body.addEventListener('htmx:afterSwap', (event) => {
    const target = event.target
    if (!(target instanceof HTMLElement)) {
      return
    }

    if (target.querySelector('#relations-grid') || target.id === 'relations-grid') {
      toggleRelationExtras(false)
    }
  })

  document.addEventListener('click', (event) => {
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
