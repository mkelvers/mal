function copyRecoveryKey(keyElementId, feedbackElementId) {
  var keyElement = document.getElementById(keyElementId)
  var feedbackElement = document.getElementById(feedbackElementId)

  if (!keyElement || !feedbackElement) {
    return
  }

  var key = keyElement.textContent || ''
  navigator.clipboard.writeText(key).then(function () {
    feedbackElement.textContent = 'Recovery key copied.'
  }).catch(function () {
    feedbackElement.textContent = 'Copy failed. Select and copy manually.'
  })
}

function confirmDangerAction(message) {
  return window.confirm(message)
}
