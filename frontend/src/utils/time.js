function pad(value) {
  return String(value).padStart(2, '0')
}

function parseDateTime(value) {
  if (!value) return null
  const text = String(value).trim()
  if (!text) return null
  const normalized = text.includes('T') ? text : `${text.replace(' ', 'T')}Z`
  const withZone = /(?:Z|[+-]\d{2}:?\d{2})$/.test(normalized) ? normalized : `${normalized}Z`
  const date = new Date(withZone)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatDateTime(value) {
  const date = parseDateTime(value)
  if (!date) return value ? String(value) : '-'
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export function formatDate(value) {
  const date = parseDateTime(value)
  if (!date) return value ? String(value) : '-'
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

export function formatLocalHour(value) {
  const date = parseDateTime(value)
  if (!date) return value ? String(value) : '-'
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:00`
}
