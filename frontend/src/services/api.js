const jsonHeaders = { 'Content-Type': 'application/json' }

async function request(path, options = {}) {
  const res = await fetch(path, options)
  if (!res.ok) {
    const data = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(data.error || res.statusText)
  }
  if (res.status === 204) return null
  const text = await res.text()
  if (!text) return null
  return JSON.parse(text)
}

function fileAsBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error('读取附件失败'))
    reader.onload = () => resolve(String(reader.result || '').split(',', 2)[1] || '')
    reader.readAsDataURL(file)
  })
}

export const api = {
  mailboxes: () => request('/api/mailboxes'),
  createMailbox: (data) => request('/api/mailboxes', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  testMailbox: (data) => request('/api/mailboxes/test', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  deleteMailbox: (id) => request(`/api/mailboxes/${id}`, { method: 'DELETE' }),
  contacts: () => request('/api/contacts'),
  contactsPage: ({ limit = 20, offset = 0, q = '' } = {}) => request(`/api/contacts/page?limit=${encodeURIComponent(limit)}&offset=${offset}&q=${encodeURIComponent(q)}`),
  createContact: (data) => request('/api/contacts', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  updateContact: (id, data) => request(`/api/contacts/${id}`, { method: 'PATCH', headers: jsonHeaders, body: JSON.stringify(data) }),
  updateContactsBatch: (data) => request('/api/contacts/batch', { method: 'PATCH', headers: jsonHeaders, body: JSON.stringify(data) }),
  deleteContactsBatch: (ids) => request('/api/contacts/batch', { method: 'DELETE', headers: jsonHeaders, body: JSON.stringify({ ids }) }),
  importContacts: async (file) => {
    const form = new FormData()
    form.append('file', file)
    return request('/api/contacts/import', { method: 'POST', body: form })
  },
  importContactsPath: (path) => request('/api/contacts/import', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  }),
  templates: () => request('/api/templates'),
  createTemplate: (data) => request('/api/templates', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  updateTemplate: (id, data) => request(`/api/templates/${id}`, { method: 'PATCH', headers: jsonHeaders, body: JSON.stringify(data) }),
  deleteTemplate: (id) => request(`/api/templates/${id}`, { method: 'DELETE' }),
  copyTemplate: (id) => request(`/api/templates/${id}/copy`, { method: 'POST' }),
  previewTemplate: (data) => request('/api/templates/preview', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  uploadTrackingImageAsset: (file, data = {}) => {
    const form = new FormData()
    form.append('file', file)
    form.append('label', data.label || '')
    form.append('width', data.width || 176)
    return request('/api/templates/tracking-image-asset', { method: 'POST', body: form })
  },
  campaigns: () => request('/api/campaigns'),
  createCampaign: (data) => request('/api/campaigns', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  uploadCampaignAttachment: async (file, { filename = '', linkBackup = false } = {}) => {
    const originalName = filename || file.name || 'attachment'
    return request('/api/campaign-attachments', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        original_name: originalName,
        content_type: file.type || 'application/octet-stream',
        content_base64: await fileAsBase64(file),
        link_backup: linkBackup,
      }),
    })
  },
  campaign: (id) => request(`/api/campaigns/${id}`),
  campaignStats: (id) => request(`/api/campaigns/${id}/stats`),
  recipients: (id) => request(`/api/campaigns/${id}/recipients`),
  sendCampaign: (id) => request(`/api/campaigns/${id}/send`, { method: 'POST' }),
  // AB Testing
  createVariant: (campaignId, data) => request(`/api/campaigns/${campaignId}/variants`, { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  updateVariant: (campaignId, variantId, data) => request(`/api/campaigns/${campaignId}/variants/${variantId}`, { method: 'PATCH', headers: jsonHeaders, body: JSON.stringify(data) }),
  deleteVariant: (campaignId, variantId) => request(`/api/campaigns/${campaignId}/variants/${variantId}`, { method: 'DELETE' }),
  abStats: (id) => request(`/api/campaigns/${id}/ab-stats`),
  // Links
  links: (id) => request(`/api/campaigns/${id}/links`),
  linkStats: (campaignId, linkId) => request(`/api/campaigns/${campaignId}/links/${linkId}/stats`),
  // Global stats
  globalStats: ({ since = '', campaign = '' } = {}) => {
    const params = new URLSearchParams()
    if (since) params.set('since', since)
    if (campaign) params.set('campaign', campaign)
    const query = params.toString() ? `?${params.toString()}` : ''
    return request(`/api/stats${query}`)
  }
}
