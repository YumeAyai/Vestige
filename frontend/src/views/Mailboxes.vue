<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../services/api'

const smtpPresets = [
  { id: '', label: '手动配置', host: '', port: 465, use_tls: true },
  { id: 'tencent-exmail', label: '腾讯企业邮箱', host: 'smtp.exmail.qq.com', port: 465, use_tls: true },
  { id: 'qq', label: 'QQ 邮箱', host: 'smtp.qq.com', port: 465, use_tls: true },
  { id: 'netease-163', label: '网易 163 邮箱', host: 'smtp.163.com', port: 465, use_tls: true },
  { id: 'netease-126', label: '网易 126 邮箱', host: 'smtp.126.com', port: 465, use_tls: true },
  { id: 'netease-exmail', label: '网易企业邮箱', host: 'smtp.qiye.163.com', port: 465, use_tls: true },
  { id: 'aliyun-exmail', label: '阿里企业邮箱', host: 'smtp.qiye.aliyun.com', port: 465, use_tls: true },
  { id: 'microsoft-365', label: 'Outlook / Microsoft 365', host: 'smtp.office365.com', port: 587, use_tls: true },
  { id: 'feishu', label: '飞书邮箱', host: 'smtp.feishu.cn', port: 465, use_tls: true },
]

const items = ref([])
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const success = ref('')
const selectedPreset = ref('')
const form = reactive({
  name: '企业邮箱',
  host: '',
  port: 465,
  username: '',
  password: '',
  from_email: '',
  from_name: '',
  use_tls: true
})
const testToEmail = ref('')

function applyPreset() {
  const preset = smtpPresets.find((item) => item.id === selectedPreset.value)
  if (!preset || !preset.id) return
  form.name = preset.label
  form.host = preset.host
  form.port = preset.port
  form.use_tls = preset.use_tls
  syncEmailFields()
}

function syncEmailFields() {
  const username = form.username.trim()
  const fromEmail = form.from_email.trim()
  if (!fromEmail && username.includes('@')) {
    form.from_email = username
  } else if (!username && fromEmail.includes('@')) {
    form.username = fromEmail
  }
}

async function load() {
  items.value = await api.mailboxes()
}

async function save() {
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    await api.createMailbox(form)
    Object.assign(form, {
      name: '',
      host: '',
      port: 465,
      username: '',
      password: '',
      from_email: '',
      from_name: '',
      use_tls: true,
    })
    selectedPreset.value = ''
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function testMailbox() {
  testing.value = true
  error.value = ''
  success.value = ''
  try {
    await api.testMailbox({ ...form, to_email: testToEmail.value })
    success.value = '测试邮件已发送，请检查收件箱和垃圾邮件。'
  } catch (err) {
    error.value = err.message
  } finally {
    testing.value = false
  }
}

async function removeMailbox(item) {
  if (!confirm(`确认删除发件邮箱「${item.name}」？`)) return
  error.value = ''
  try {
    await api.deleteMailbox(item.id)
    await load()
  } catch (err) {
    error.value = err.message
  }
}

onMounted(load)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>发件邮箱</h1>
        <p class="muted">配置企业邮箱 SMTP 后即可批量发送。</p>
      </div>
    </div>
    <p v-if="error" class="notice error">{{ error }}</p>
    <p v-if="success" class="notice success">{{ success }}</p>
    <div class="grid two">
      <form class="panel grid" @submit.prevent="save">
        <label>服务商模板
          <select v-model="selectedPreset" @change="applyPreset">
            <option v-for="preset in smtpPresets" :key="preset.id" :value="preset.id">
              {{ preset.label }}
            </option>
          </select>
        </label>
        <label>配置名称<input v-model="form.name" required /></label>
        <label>SMTP Host<input v-model="form.host" placeholder="smtp.exmail.qq.com" required /></label>
        <label>端口<input v-model.number="form.port" type="number" required /></label>
        <label>账号<input v-model="form.username" required @blur="syncEmailFields" /></label>
        <label>授权码/密码<input v-model="form.password" type="password" required /></label>
        <label>发件邮箱<input v-model="form.from_email" type="email" required @blur="syncEmailFields" /></label>
        <label>发件人名称<input v-model="form.from_name" required /></label>
        <label><span><input v-model="form.use_tls" type="checkbox" style="width:auto" /> 使用 TLS / STARTTLS</span></label>
        <label>测试收件邮箱<input v-model="testToEmail" type="email" placeholder="默认发送到发件邮箱" /></label>
        <div class="toolbar">
          <button :disabled="saving">保存邮箱</button>
          <button class="secondary" type="button" :disabled="testing" @click="testMailbox">
            {{ testing ? '测试中' : '发送测试' }}
          </button>
        </div>
      </form>
      <div class="panel">
        <h2>已配置邮箱</h2>
        <table>
          <thead><tr><th>名称</th><th>服务器</th><th>发件人</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>{{ item.name }}</td>
              <td>{{ item.host }}:{{ item.port }}</td>
              <td>{{ item.from_name }} &lt;{{ item.from_email }}&gt;</td>
              <td><button class="secondary danger" @click="removeMailbox(item)">删除</button></td>
            </tr>
          </tbody>
        </table>
        <p v-if="items.length === 0" class="empty">还没有配置发件邮箱。</p>
      </div>
    </div>
  </section>
</template>
