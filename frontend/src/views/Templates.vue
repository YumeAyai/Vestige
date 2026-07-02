<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { api } from '../services/api'
import { formatDateTime } from '../utils/time'
import { askConfirm, askPrompt } from '../utils/dialog'

const items = ref([])
const preview = ref({ subject: '', body_html: '' })
const previewError = ref('')
const saveMessage = ref('')
const imageAsset = ref(null)
const imageError = ref('')
const linkError = ref('')
const imageLoading = ref(false)
const imageFile = ref(null)
const bodyEditor = ref(null)
const activeTemplateId = ref(null)
const form = reactive({
  name: '企业微信推广联系',
  subject: '{{.Company}} 您好，添加企业微信获取合作资料',
  body_html:
    '<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f5f8f6;padding:24px 0"><tr><td align="center"><table role="presentation" width="640" cellpadding="0" cellspacing="0" style="max-width:640px;background:#ffffff;border:1px solid #e2ebe6"><tr><td style="padding:28px;font-family:Arial,Helvetica,sans-serif;color:#17233c"><h1 style="font-size:22px;margin:0 0 14px">企业微信联系</h1><p>{{.Name}} 您好：</p><p>我们整理了一份适合 {{.Company}} 的合作资料，欢迎添加企业微信进一步沟通。</p><p style="margin:20px 0;color:#607269">上传图片后，这里会插入图片埋点。</p><p style="color:#607269;font-size:13px">图片埋点由服务端返回；邮件客户端加载图片时会记录设备、IP、User-Agent 和 Referer。</p><p>期待交流。</p></td></tr></table></td></tr></table>',
})
const sample = reactive({
  name: '上海示例企业有限公司',
  email: 'contact@example.com',
  company: '上海示例企业有限公司',
  department: '行政部',
  phone: '021-00000000',
  tags: '教育,10000人以上',
  notes: ''
})
const imageForm = reactive({
  label: '企业微信图片',
  width: 176
})
const linkForm = reactive({
  label: '官网链接',
  url: 'https://example.com/survey',
  text: '查看详情'
})

const canPreview = computed(() => form.subject || form.body_html)
const collectFields = [
  'IP 地址',
  'User-Agent',
  '设备/系统',
  '浏览器',
  '语言',
  'Referer',
  'X-Forwarded-For',
  '请求时间',
  '预加载判断'
]

async function load() {
  items.value = await api.templates()
}

async function save() {
  saveMessage.value = ''
  if (activeTemplateId.value) {
    await api.updateTemplate(activeTemplateId.value, form)
    saveMessage.value = '模板已更新。'
  } else {
    const created = await api.createTemplate(form)
    activeTemplateId.value = created.id
    saveMessage.value = '模板已保存，可以在邮件任务中选择使用。'
  }
  await load()
}

function openTemplate(item) {
  activeTemplateId.value = item.id
  Object.assign(form, {
    name: item.name,
    subject: item.subject,
    body_html: item.body_html
  })
  saveMessage.value = `已打开模板：${item.name}`
}

function newTemplate() {
  activeTemplateId.value = null
  Object.assign(form, {
    name: '企业微信推广联系',
    subject: '{{.Company}} 您好，添加企业微信获取合作资料',
    body_html:
      '<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f5f8f6;padding:24px 0"><tr><td align="center"><table role="presentation" width="640" cellpadding="0" cellspacing="0" style="max-width:640px;background:#ffffff;border:1px solid #e2ebe6"><tr><td style="padding:28px;font-family:Arial,Helvetica,sans-serif;color:#17233c"><h1 style="font-size:22px;margin:0 0 14px">企业微信联系</h1><p>{{.Name}} 您好：</p><p>我们整理了一份适合 {{.Company}} 的合作资料，欢迎添加企业微信进一步沟通。</p><p style="margin:20px 0;color:#607269">上传图片后，这里会插入图片埋点。</p><p style="color:#607269;font-size:13px">图片埋点由服务端返回；邮件客户端加载图片时会记录设备、IP、User-Agent 和 Referer。</p><p>期待交流。</p></td></tr></table></td></tr></table>'
  })
  saveMessage.value = ''
}

async function deleteTemplate(item) {
  if (!(await askConfirm(`删除模板「${item.name}」？`))) return
  await api.deleteTemplate(item.id)
  if (activeTemplateId.value === item.id) {
    newTemplate()
  }
  await load()
}

async function copyTemplate(item) {
  await api.copyTemplate(item.id)
  saveMessage.value = `已复制模板：${item.name}`
  await load()
}

async function renameTemplate(item) {
  const name = await askPrompt('模板新名称', item.name)
  if (!name || name.trim() === item.name) return
  await api.updateTemplate(item.id, { ...item, name: name.trim() })
  if (activeTemplateId.value === item.id) {
    form.name = name.trim()
  }
  await load()
}

async function renderPreview() {
  if (!canPreview.value) return
  previewError.value = ''
  try {
    preview.value = await api.previewTemplate({
      subject: form.subject,
      body_html: form.body_html,
      contact: sample
    })
  } catch (err) {
    previewError.value = err.message
  }
}

function insertAtCursor(text) {
  const el = bodyEditor.value
  if (!el) {
    form.body_html += text
    return
  }
  const start = el.selectionStart ?? form.body_html.length
  const end = el.selectionEnd ?? form.body_html.length
  form.body_html = form.body_html.slice(0, start) + text + form.body_html.slice(end)
  requestAnimationFrame(() => {
    el.focus()
    const next = start + text.length
    el.setSelectionRange(next, next)
  })
}

function insertVariable(value) {
  insertAtCursor(value)
}

function wrapSelection(before, after) {
  const el = bodyEditor.value
  if (!el) {
    insertAtCursor(before + after)
    return
  }
  const start = el.selectionStart ?? 0
  const end = el.selectionEnd ?? 0
  const selected = form.body_html.slice(start, end) || '正文'
  form.body_html = form.body_html.slice(0, start) + before + selected + after + form.body_html.slice(end)
  requestAnimationFrame(() => {
    el.focus()
    el.setSelectionRange(start + before.length, start + before.length + selected.length)
  })
}

function selectTrackingImage(event) {
  imageFile.value = event.target.files?.[0] || null
  imageError.value = ''
}

async function uploadTrackingImageAsset() {
  if (!imageFile.value) {
    imageError.value = '请先选择埋点图片。'
    return
  }
  imageLoading.value = true
  imageError.value = ''
  try {
    imageAsset.value = await api.uploadTrackingImageAsset(imageFile.value, imageForm)
    insertAtCursor(imageAsset.value.placeholder)
  } catch (err) {
    imageError.value = err.message
  } finally {
    imageLoading.value = false
  }
}

function insertTrackingImagePlaceholder() {
  if (!imageAsset.value?.placeholder) {
    imageError.value = '请先上传埋点图片，再插入埋点位置。'
    return
  }
  insertAtCursor(imageAsset.value.placeholder)
}

function quoteTemplateString(value) {
  return JSON.stringify(String(value || '').trim())
}

function insertTrackingLink() {
  const label = linkForm.label.trim() || '链接'
  const url = linkForm.url.trim()
  const text = linkForm.text.trim() || label
  if (!url) {
    linkError.value = '请填写链接埋点目标 URL。'
    return
  }
  linkError.value = ''
  const href = `{{TrackingLink ${quoteTemplateString(label)} ${quoteTemplateString(url)}}}`
  insertAtCursor(`<a href="${href}">${text}</a>`)
}

watch(
  () => [form.subject, form.body_html, sample.name, sample.company, sample.email, sample.phone],
  renderPreview
)

onMounted(load)
onMounted(renderPreview)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>邮件模板编辑</h1>
        <p class="muted">左侧编辑 HTML 邮件模板，右侧实时预览收件人看到的效果。</p>
      </div>
      <div class="toolbar">
        <button type="button" class="secondary" @click="newTemplate">新建</button>
        <button type="button" @click="save">{{ activeTemplateId ? '更新模板' : '保存模板' }}</button>
      </div>
    </div>

    <div class="template-editor">
      <form class="panel editor-panel" @submit.prevent="save">
        <div class="editor-meta">
          <label>模板名称<input v-model="form.name" required /></label>
          <label>邮件主题<input v-model="form.subject" required /></label>
        </div>

        <div class="editor-tools" aria-label="邮件模板工具栏">
          <button type="button" class="secondary icon-button" title="加粗" @click="wrapSelection('<strong>', '</strong>')">B</button>
          <button type="button" class="secondary icon-button" title="段落" @click="wrapSelection('<p>', '</p>')">P</button>
          <button type="button" class="secondary icon-button" title="按钮链接" @click="wrapSelection('<a href=&quot;https://example.com/survey&quot; style=&quot;display:inline-block;background:#008f68;color:#ffffff;text-decoration:none;padding:10px 16px;border-radius:6px&quot;>', '</a>')">↗</button>
          <span class="tool-divider"></span>
          <button type="button" class="secondary" @click="insertVariable('{{.Name}}')">姓名</button>
          <button type="button" class="secondary" @click="insertVariable('{{.Company}}')">公司</button>
          <button type="button" class="secondary" @click="insertVariable('{{.Department}}')">部门</button>
          <button type="button" class="secondary" @click="insertVariable('{{.Email}}')">邮箱</button>
          <button type="button" class="secondary" @click="insertTrackingImagePlaceholder">联系图片埋点</button>
          <button type="button" class="secondary" @click="insertTrackingLink">链接埋点</button>
        </div>

        <label class="html-editor">
          HTML 正文
          <textarea ref="bodyEditor" v-model="form.body_html" required spellcheck="false"></textarea>
        </label>

        <div class="qr-tool">
          <div>
            <h2>联系图片埋点资源</h2>
            <p class="muted">上传需要追踪加载的图片，服务端返回可插入到邮件里的图片埋点；正式发送时每个收件人会获得独立 token。</p>
          </div>
          <div class="qr-tool-grid">
            <label>图片埋点名称<input v-model="imageForm.label" placeholder="例如：企业微信图片 A" /></label>
            <label>埋点图片<input type="file" accept="image/png,image/jpeg,image/gif,image/webp" @change="selectTrackingImage" /></label>
            <label>显示宽度
              <select v-model.number="imageForm.width">
                <option :value="132">132 px</option>
                <option :value="176">176 px</option>
                <option :value="220">220 px</option>
              </select>
            </label>
            <button type="button" :disabled="imageLoading" @click="uploadTrackingImageAsset">
              {{ imageLoading ? '上传中' : '上传并插入' }}
            </button>
          </div>
          <p v-if="imageError" class="notice error">{{ imageError }}</p>
          <div v-if="imageAsset" class="qr-asset-result">
            <img :src="imageAsset.image_url" alt="图片埋点预览" />
            <div>
              <strong>{{ imageAsset.label }}</strong>
              <code>{{ imageAsset.placeholder }}</code>
              <span>{{ imageAsset.image_url }}</span>
            </div>
          </div>
        </div>

        <div class="qr-tool">
          <div>
            <h2>链接埋点资源</h2>
            <p class="muted">生成可插入正文的追踪链接；正式发送时每个收件人都会获得独立 click token，点击会计入链接追踪。</p>
          </div>
          <div class="qr-tool-grid link-tool-grid">
            <label>链接名称<input v-model="linkForm.label" placeholder="例如：官网按钮" /></label>
            <label>目标 URL<input v-model="linkForm.url" type="url" placeholder="https://example.com/survey" /></label>
            <label>链接文字<input v-model="linkForm.text" placeholder="查看详情" /></label>
            <button type="button" @click="insertTrackingLink">插入链接埋点</button>
          </div>
          <p v-if="linkError" class="notice error">{{ linkError }}</p>
        </div>

        <p v-if="saveMessage" class="notice success">{{ saveMessage }}</p>
        <p v-if="previewError" class="notice error">{{ previewError }}</p>
      </form>

      <aside class="panel preview-panel">
        <div class="preview-head">
          <div>
            <h2>实时预览</h2>
            <p class="muted">使用示例联系人渲染变量和图片埋点。</p>
          </div>
        </div>
        <div class="preview-sample">
          <label>预览公司<input v-model="sample.company" /></label>
          <label>预览邮箱<input v-model="sample.email" /></label>
        </div>
        <div class="mail-preview">
          <div class="mail-reader-toolbar">
            <span></span>
            <span></span>
            <span></span>
          </div>
          <div class="mail-reader-header">
            <p class="mail-reader-label">主题</p>
            <h2>{{ preview.subject || '邮件主题预览' }}</h2>
            <div class="mail-reader-meta">
              <div>
                <span>发件人</span>
                <strong>见迹邮件助手 &lt;noreply@example.com&gt;</strong>
              </div>
              <div>
                <span>收件人</span>
                <strong>{{ sample.company || sample.email }} &lt;{{ sample.email }}&gt;</strong>
              </div>
              <div>
                <span>时间</span>
                <strong>刚刚</strong>
              </div>
            </div>
          </div>
          <div class="mail-reader-body">
          <iframe title="邮件 HTML 预览" :srcdoc="preview.body_html"></iframe>
          </div>
        </div>
        <div class="tracking-summary">
          <h2>请求可采集信息</h2>
          <div class="chip-list">
            <span v-for="field in collectFields" :key="field" class="data-chip tone-1">{{ field }}</span>
          </div>
        </div>
      </aside>
    </div>

    <div class="panel" style="margin-top: 18px">
      <h2>模板列表</h2>
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>主题</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id" :class="{ 'active-row': item.id === activeTemplateId }">
              <td>{{ item.name }}</td>
              <td>{{ item.subject }}</td>
              <td>{{ formatDateTime(item.created_at) }}</td>
              <td>{{ formatDateTime(item.updated_at) }}</td>
              <td>
                <div class="table-actions">
                  <button type="button" class="secondary" @click="openTemplate(item)">打开</button>
                  <button type="button" class="secondary" @click="renameTemplate(item)">重命名</button>
                  <button type="button" class="secondary" @click="copyTemplate(item)">复制</button>
                  <button type="button" class="secondary danger" @click="deleteTemplate(item)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
    </div>
  </section>
</template>
