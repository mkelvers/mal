"use strict";
function copyRecoveryKey(keyElementId, feedbackElementId) {
    const keyElement = document.getElementById(keyElementId);
    const feedbackElement = document.getElementById(feedbackElementId);
    if (!keyElement || !feedbackElement) {
        return;
    }
    const key = keyElement.textContent || '';
    navigator.clipboard
        .writeText(key)
        .then(() => {
        feedbackElement.textContent = 'Recovery key copied.';
    })
        .catch(() => {
        feedbackElement.textContent = 'Copy failed. Select and copy manually.';
    });
}
function confirmDangerAction(message) {
    return window.confirm(message);
}
;
window.copyRecoveryKey = copyRecoveryKey;
window.confirmDangerAction = confirmDangerAction;
