const jsonHeaders = { 'Content-Type': 'application/json' }

async function request(path, options = {}) {
  const res = await fetch(path, options)
  if (!res.ok) {
    const data = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(data.error || res.statusText)
  }
  return res.json()
}

export const api = {
  mailboxes: () => request('/api/mailboxes'),
  createMailbox: (data) => request('/api/mailboxes', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  deleteMailbox: (id) => request(`/api/mailboxes/${id}`, { method: 'DELETE' }),
  contacts: () => request('/api/contacts'),
  createContact: (data) => request('/api/contacts', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  importContacts: async (file) => {
    const form = new FormData()
    form.append('file', file)
    return request('/api/contacts/import', { method: 'POST', body: form })
  },
  templates: () => request('/api/templates'),
  createTemplate: (data) => request('/api/templates', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  previewTemplate: (data) => request('/api/templates/preview', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  campaigns: () => request('/api/campaigns'),
  createCampaign: (data) => request('/api/campaigns', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(data) }),
  campaign: (id) => request(`/api/campaigns/${id}`),
  campaignStats: (id) => request(`/api/campaigns/${id}/stats`),
  recipients: (id) => request(`/api/campaigns/${id}/recipients`),
  sendCampaign: (id) => request(`/api/campaigns/${id}/send`, { method: 'POST', headers: { 'X-Base-URL': location.origin } })
}
