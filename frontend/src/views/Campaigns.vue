<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../services/api'

const router = useRouter()
const campaigns = ref([])
const contacts = ref([])
const mailboxes = ref([])
const templates = ref([])
const selectedTemplate = ref('')
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

async function load() {
  campaigns.value = await api.campaigns()
  contacts.value = await api.contacts()
  mailboxes.value = await api.mailboxes()
  templates.value = await api.templates()
}

function applyTemplate() {
  const tpl = templates.value.find((item) => item.id === Number(selectedTemplate.value))
  if (!tpl) return
  form.subject = tpl.subject
  form.body_html = tpl.body_html
  if (!form.name) form.name = tpl.name
}

async function create() {
  const result = await api.createCampaign({
    ...form,
    mailbox_id: Number(form.mailbox_id),
    contact_ids: form.contact_ids.map(Number),
  })
  router.push(`/campaigns/${result.id}`)
}

onMounted(load)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>邮件任务</h1>
        <p class="muted">创建市场调查、通知、邀约邮件，并自动生成阅读追踪。</p>
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
        <label>HTML 正文<textarea v-model="form.body_html" required /></label>
        <label>
          收件人
          <select v-model="form.contact_ids" multiple size="8" required>
            <option v-for="item in contacts" :key="item.id" :value="item.id">
              {{ item.name }} - {{ item.email }}
            </option>
          </select>
        </label>
        <label
          ><span
            ><input v-model="form.tracking_enabled" type="checkbox" style="width: auto" />
            开启阅读状态追踪</span
          ></label
        >
        <button :disabled="!canCreate">创建任务</button>
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
              <td>{{ item.created_at }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
