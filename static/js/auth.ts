function copyRecoveryKey(keyElementId: string, feedbackElementId: string): void {
  const keyElement = document.getElementById(keyElementId)
  const feedbackElement = document.getElementById(feedbackElementId)

  if (!keyElement || !feedbackElement) {
    return
  }

  const key = keyElement.textContent || ''
  navigator.clipboard
    .writeText(key)
    .then((): void => {
      feedbackElement.textContent = 'Recovery key copied.'
    })
    .catch((): void => {
      feedbackElement.textContent = 'Copy failed. Select and copy manually.'
    })
}

function confirmDangerAction(message: string): boolean {
  return window.confirm(message)
}

;(window as Window & { copyRecoveryKey?: typeof copyRecoveryKey; confirmDangerAction?: typeof confirmDangerAction }).copyRecoveryKey = copyRecoveryKey
;(window as Window & { copyRecoveryKey?: typeof copyRecoveryKey; confirmDangerAction?: typeof confirmDangerAction }).confirmDangerAction = confirmDangerAction
