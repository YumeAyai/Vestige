<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { api } from '../services/api'

const items = ref([])
const preview = ref({ subject: '', body_html: '' })
const previewError = ref('')
const form = reactive({
  name: '市场调查邀请',
  subject: '请协助完成 {{.Company}} 调研问卷',
  body_html:
    '<p>{{.Name}} 您好：</p><p>我们正在开展一项市场调查，想邀请您协助填写问卷。</p><p>{{.QRCode}}</p><p>谢谢支持。</p>',
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

const canPreview = computed(() => form.subject || form.body_html)

async function load() {
  items.value = await api.templates()
}

async function save() {
  await api.createTemplate(form)
  Object.assign(form, { name: '', subject: '', body_html: '' })
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

watch(() => [form.subject, form.body_html, sample.name, sample.company, sample.email, sample.phone], renderPreview)

onMounted(load)
onMounted(renderPreview)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>邮件模板</h1>
        <p class="muted" v-pre>支持变量：{{.Name}}、{{.Company}}、{{.Department}}、{{.Email}}、{{.QRCode}}。</p>
      </div>
    </div>
    <div class="grid two">
      <form class="panel grid" @submit.prevent="save">
        <label>模板名称<input v-model="form.name" required /></label>
        <label>邮件主题<input v-model="form.subject" required /></label>
        <label>HTML 正文<textarea v-model="form.body_html" required /></label>
        <div class="preview-sample">
          <label>预览公司<input v-model="sample.company" /></label>
          <label>预览邮箱<input v-model="sample.email" /></label>
        </div>
        <p v-if="previewError" class="notice error">{{ previewError }}</p>
        <button>保存模板</button>
      </form>
      <div class="panel">
        <h2>实时预览</h2>
        <div class="mail-preview">
          <div class="mail-preview-subject">{{ preview.subject || '邮件主题预览' }}</div>
          <iframe title="邮件 HTML 预览" :srcdoc="preview.body_html"></iframe>
        </div>
      </div>
    </div>

    <div class="panel" style="margin-top: 18px">
      <h2>模板列表</h2>
        <table>
          <thead><tr><th>名称</th><th>主题</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>{{ item.name }}</td>
              <td>{{ item.subject }}</td>
            </tr>
          </tbody>
        </table>
    </div>
  </section>
</template>
