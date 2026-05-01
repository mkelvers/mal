class UIDropdown extends HTMLElement {
  isOpen: boolean = false
  contentEl: HTMLElement | null = null

  constructor() {
    super()
    this.toggle = this.toggle.bind(this)
    this.handleClickOutside = this.handleClickOutside.bind(this)
  }

  connectedCallback() {
    const trigger = this.querySelector('[data-trigger]')
    this.contentEl = this.querySelector('[data-content]')

    if (trigger) {
      trigger.addEventListener('click', this.toggle)
    }

    document.addEventListener('click', this.handleClickOutside)
  }

  disconnectedCallback() {
    const trigger = this.querySelector('[data-trigger]')
    if (trigger) {
      trigger.removeEventListener('click', this.toggle)
    }
    document.removeEventListener('click', this.handleClickOutside)
  }

  toggle() {
    this.isOpen = !this.isOpen
    if (this.contentEl) {
      if (this.isOpen) {
        this.contentEl.classList.remove('hidden')
      } else {
        this.contentEl.classList.add('hidden')
      }
    }
  }

  close() {
    this.isOpen = false
    if (this.contentEl) {
      this.contentEl.classList.add('hidden')
    }
  }

  handleClickOutside(event: MouseEvent) {
    if (!this.contains(event.target as Node)) {
      this.close()
    }
  }
}

customElements.define('ui-dropdown', UIDropdown)
