<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../services/api'
import { formatDateTime } from '../utils/time'

const router = useRouter()
const route = useRoute()
const campaigns = ref([])
const contacts = ref([])
const mailboxes = ref([])
const templates = ref([])
const selectedTemplate = ref('')
const contactQuery = ref('')
const bodyMode = ref('preview')
const preview = ref({ subject: '', body_html: '' })
const previewError = ref('')
const attachmentInput = ref(null)
const attachmentFiles = ref([])
const attachmentLinkBackup = ref(false)
const attachmentError = ref('')
const creating = ref(false)
const form = reactive({
  name: '',
  subject: '',
  body_html: '',
  mailbox_id: '',
  tracking_enabled: true,
  contact_ids: [],
})

const canCreate = computed(
  () =>
    form.name && form.subject && form.body_html && form.mailbox_id && form.contact_ids.length > 0,
)

const filteredContacts = computed(() => {
  const query = contactQuery.value.trim().toLowerCase()
  if (!query) return contacts.value
  return contacts.value.filter((item) =>
    [item.name, item.email, item.company, item.phone, item.tags]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query)),
  )
})

const previewContact = computed(() => {
  const selected = contacts.value.find((item) => form.contact_ids.includes(item.id))
  return (
    selected || {
      name: '上海示例企业有限公司',
      email: 'contact@example.com',
      company: '上海示例企业有限公司',
      department: '行政部',
      phone: '021-00000000',
      tags: '',
      notes: '',
    }
  )
})

async function load() {
  campaigns.value = await api.campaigns()
  contacts.value = await api.contacts()
  mailboxes.value = await api.mailboxes()
  templates.value = await api.templates()
  applyQueryContacts()
}

function applyQueryContacts() {
  const raw = String(route.query.contacts || '')
  if (!raw) return
  const ids = raw
    .split(',')
    .map((item) => Number(item))
    .filter(Boolean)
  const available = new Set(contacts.value.map((item) => item.id))
  form.contact_ids = ids.filter((id) => available.has(id))
}

function applyTemplate() {
  const tpl = templates.value.find((item) => item.id === Number(selectedTemplate.value))
  if (!tpl) return
  form.subject = tpl.subject
  form.body_html = tpl.body_html
  if (!form.name) form.name = tpl.name
  bodyMode.value = 'preview'
}

function isContactSelected(id) {
  return form.contact_ids.includes(Number(id))
}

function toggleContact(id) {
  const value = Number(id)
  const index = form.contact_ids.indexOf(value)
  if (index >= 0) {
    form.contact_ids.splice(index, 1)
  } else {
    form.contact_ids.push(value)
  }
}

async function renderPreview() {
  if (!form.subject && !form.body_html) {
    preview.value = { subject: '', body_html: '' }
    return
  }
  previewError.value = ''
  try {
    preview.value = await api.previewTemplate({
      subject: form.subject,
      body_html: form.body_html,
      contact: previewContact.value,
    })
  } catch (err) {
    previewError.value = err.message
  }
}

async function create() {
  creating.value = true
  attachmentError.value = ''
  try {
    const attachmentIds = []
    for (const file of attachmentFiles.value) {
      const uploaded = await api.uploadCampaignAttachment(file, { linkBackup: attachmentLinkBackup.value })
      attachmentIds.push(uploaded.id)
    }
    const result = await api.createCampaign({
      ...form,
      mailbox_id: Number(form.mailbox_id),
      contact_ids: form.contact_ids.map(Number),
      attachment_ids: attachmentIds,
    })
    router.push(`/campaigns/${result.id}`)
  } catch (err) {
    attachmentError.value = err.message
  } finally {
    creating.value = false
  }
}

function selectAttachments(event) {
  attachmentFiles.value = Array.from(event.target.files || [])
  attachmentError.value = ''
}

function openAttachmentPicker() {
  attachmentInput.value?.click()
}

function removeAttachment(index) {
  attachmentFiles.value = attachmentFiles.value.filter((_, itemIndex) => itemIndex !== index)
  if (attachmentInput.value) attachmentInput.value.value = ''
}

onMounted(load)
watch(
  () => [form.subject, form.body_html, form.contact_ids.join(','), contacts.value.length],
  renderPreview,
)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>邮件任务</h1>
        <p class="muted">创建市场调查、通知、邀约邮件，并自动生成匿名埋点。</p>
      </div>
    </div>

    <div class="grid two">
      <form class="panel grid" @submit.prevent="create">
        <label
          >任务名称<input v-model="form.name" placeholder="6 月供应商满意度调研" required
        /></label>
        <label>
          套用模板
          <select v-model="selectedTemplate" @change="applyTemplate">
            <option value="">不套用模板</option>
            <option v-for="item in templates" :key="item.id" :value="item.id">
              {{ item.name }}
            </option>
          </select>
        </label>
        <label
          >发件邮箱
          <select v-model="form.mailbox_id" required>
            <option value="">请选择</option>
            <option v-for="item in mailboxes" :key="item.id" :value="item.id">
              {{ item.name }} - {{ item.from_email }}
            </option>
          </select>
        </label>
        <label>邮件主题<input v-model="form.subject" required /></label>
        <div class="field-block">
          <div class="field-row">
            <div>
              <div class="field-label">邮件正文</div>
              <p class="muted">默认展示最终渲染效果；需要修改源码时切到 HTML。</p>
            </div>
            <div class="segmented">
              <button
                type="button"
                :class="{ active: bodyMode === 'preview' }"
                @click="bodyMode = 'preview'; renderPreview()"
              >
                预览
              </button>
              <button type="button" :class="{ active: bodyMode === 'html' }" @click="bodyMode = 'html'">
                HTML
              </button>
            </div>
          </div>
          <div v-if="bodyMode === 'preview'" class="mail-preview campaign-preview">
            <div class="mail-preview-subject">{{ preview.subject || form.subject || '邮件主题预览' }}</div>
            <iframe title="邮件正文预览" :srcdoc="preview.body_html || form.body_html"></iframe>
          </div>
          <label v-else class="html-editor">HTML 正文<textarea v-model="form.body_html" required /></label>
          <p v-if="previewError" class="notice error">{{ previewError }}</p>
        </div>
        <div class="field-block">
          <div class="field-row">
            <div>
              <div class="field-label">附件</div>
              <p class="muted">可直接夹带发送；勾选备用下载链接后，会额外生成可追踪下载入口。</p>
            </div>
            <label class="checkline">
              <input v-model="attachmentLinkBackup" class="contact-check" type="checkbox" />
              带备用下载链接
            </label>
          </div>
          <input ref="attachmentInput" class="file-picker-input" type="file" multiple @change="selectAttachments" />
          <button class="secondary attachment-upload-button" type="button" @click="openAttachmentPicker">
            选择附件
          </button>
          <div v-if="attachmentFiles.length" class="attachment-file-list">
            <div v-for="(file, index) in attachmentFiles" :key="file.name + file.size + file.lastModified" class="attachment-file">
              <span class="attachment-file-info">
                <strong>{{ file.name }}</strong>
                <small>{{ Math.ceil(file.size / 1024) }} KB</small>
              </span>
              <button type="button" class="attachment-remove" :aria-label="`移除 ${file.name}`" @click="removeAttachment(index)">
                ×
              </button>
            </div>
          </div>
          <p v-if="attachmentError" class="notice error">{{ attachmentError }}</p>
        </div>
        <div class="field-block">
          <div class="field-label">收件人</div>
          <input v-model="contactQuery" placeholder="搜索公司、邮箱、电话或标签" />
          <div class="picker-list" role="listbox" aria-label="收件人列表" aria-multiselectable="true">
            <button
              v-for="item in filteredContacts"
              :key="item.id"
              class="picker-option"
              :class="{ selected: isContactSelected(item.id) }"
              type="button"
              role="option"
              :aria-selected="isContactSelected(item.id)"
              @click="toggleContact(item.id)"
            >
              <span class="picker-check" aria-hidden="true">
                <svg v-if="isContactSelected(item.id)" viewBox="0 0 24 24">
                  <path d="M5 12.5l4.2 4.2L19 7" />
                </svg>
              </span>
              <span class="picker-main">
                <strong>{{ item.name || item.company || item.email }}</strong>
                <small>{{ item.email }}</small>
              </span>
            </button>
            <p v-if="filteredContacts.length === 0" class="empty">没有匹配的联系人</p>
          </div>
          <p class="muted">已选择 {{ form.contact_ids.length }} 个收件人，系统会逐个单独发送。</p>
        </div>
        <button :disabled="!canCreate || creating">{{ creating ? '创建中...' : '创建任务' }}</button>
      </form>

      <div class="panel">
        <h2>任务列表</h2>
        <table>
          <thead>
            <tr>
              <th>任务</th>
              <th>状态</th>
              <th>创建时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in campaigns" :key="item.id">
              <td>
                <RouterLink :to="`/campaigns/${item.id}`">{{ item.name }}</RouterLink>
              </td>
              <td>
                <span class="status" :class="item.status">{{ item.status }}</span>
              </td>
              <td>{{ formatDateTime(item.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
