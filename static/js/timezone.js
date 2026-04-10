;(function () {
  const jstOffsetMinutes = 9 * 60

  const parseBroadcast = (text) => {
    const match = text.match(/^(.*) at (\d{1,2}):(\d{2}) \(JST\)$/i)
    if (!match) {
      return null
    }

    const day = match[1].trim()
    const hour = Number.parseInt(match[2], 10)
    const minute = Number.parseInt(match[3], 10)

    if (Number.isNaN(hour) || Number.isNaN(minute)) {
      return null
    }

    return { day, hour, minute }
  }

  const normalizeDay = (day) => {
    const key = day.trim().toLowerCase().replace(/s$/, '')
    const days = {
      mon: 1,
      monday: 1,
      tue: 2,
      tues: 2,
      tuesday: 2,
      wed: 3,
      wednesday: 3,
      thu: 4,
      thur: 4,
      thurs: 4,
      thursday: 4,
      fri: 5,
      friday: 5,
      sat: 6,
      saturday: 6,
      sun: 0,
      sunday: 0,
    }

    if (typeof days[key] !== 'number') {
      return null
    }

    return days[key]
  }

  const convertToLocal = (parsed, localOffsetMinutes) => {
    const sourceMinutes = parsed.hour * 60 + parsed.minute
    const diff = jstOffsetMinutes - localOffsetMinutes
    const localTotal = sourceMinutes - diff

    const dayShift = Math.floor(localTotal / 1440)
    const normalizedMinutes = ((localTotal % 1440) + 1440) % 1440
    const localHour = Math.floor(normalizedMinutes / 60)
    const localMinute = normalizedMinutes % 60

    const sourceDayIndex = normalizeDay(parsed.day)
    if (sourceDayIndex === null) {
      return null
    }

    const localDayIndex = ((sourceDayIndex + dayShift) % 7 + 7) % 7
    const localDay = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'][localDayIndex]

    const time = `${localHour.toString().padStart(2, '0')}:${localMinute.toString().padStart(2, '0')}`
    return `${localDay} at ${time} (Local)`
  }

  const updateNode = (node, localOffsetMinutes) => {
    const source = node.getAttribute('data-jst-text')
    if (!source) {
      return
    }

    const parsed = parseBroadcast(source)
    if (!parsed) {
      return
    }

    const converted = convertToLocal(parsed, localOffsetMinutes)
    if (!converted) {
      return
    }

    node.textContent = converted
  }

  const hideJstSuffix = () => {
    document.querySelectorAll('.notification-broadcast, [data-jst-text]').forEach((node) => {
      if (!(node instanceof HTMLElement)) {
        return
      }

      const text = node.textContent || ''
      if (text.includes('(JST)')) {
        node.textContent = text.replace(/\s*\(JST\)/gi, ' (Local)')
      }
    })
  }

  const updateAll = () => {
    const localOffsetMinutes = -new Date().getTimezoneOffset()
    const nodes = document.querySelectorAll('[data-jst-text]')
    nodes.forEach((node) => updateNode(node, localOffsetMinutes))
    hideJstSuffix()
  }

  document.addEventListener('DOMContentLoaded', updateAll)
  document.body.addEventListener('htmx:afterSwap', updateAll)
})()
