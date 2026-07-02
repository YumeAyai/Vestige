let dialogHandler = null

export function installDialog(handler) {
  dialogHandler = handler
}

export function askConfirm(message, options = {}) {
  if (dialogHandler) {
    return dialogHandler({ ...options, type: 'confirm', message })
  }
  return Promise.resolve(window.confirm(message))
}

export function askPrompt(message, defaultValue = '', options = {}) {
  if (dialogHandler) {
    return dialogHandler({ ...options, type: 'prompt', message, defaultValue })
  }
  return Promise.resolve(window.prompt(message, defaultValue))
}
