<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../services/api'

const items = ref([])
const importing = ref(false)
const form = reactive({ name: '', email: '', company: '', department: '', phone: '', tags: '', notes: '' })

async function load() {
  items.value = await api.contacts()
}

async function save() {
  await api.createContact(form)
  Object.assign(form, { name: '', email: '', company: '', department: '', phone: '', tags: '', notes: '' })
  await load()
}

async function upload(event) {
  const file = event.target.files?.[0]
  if (!file) return
  importing.value = true
  try {
    await api.importContacts(file)
    await load()
  } finally {
    importing.value = false
    event.target.value = ''
  }
}

onMounted(load)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>联系人</h1>
        <p class="muted">支持导入 xlsx / csv；会自动识别“公司名、邮箱、联系电话、官网、行业、规模”等字段。</p>
      </div>
      <label class="button secondary">
        导入名单
        <input type="file" accept=".xlsx,.csv" style="display:none" :disabled="importing" @change="upload" />
      </label>
    </div>
    <form class="panel grid four" style="margin-bottom:16px" @submit.prevent="save">
      <label>姓名<input v-model="form.name" /></label>
      <label>邮箱<input v-model="form.email" type="email" required /></label>
      <label>公司<input v-model="form.company" /></label>
      <label>部门<input v-model="form.department" /></label>
      <label>手机号<input v-model="form.phone" /></label>
      <label>标签<input v-model="form.tags" /></label>
      <label>备注<input v-model="form.notes" /></label>
      <button>新增联系人</button>
    </form>
    <div class="panel">
      <table>
        <thead><tr><th>姓名</th><th>邮箱</th><th>公司</th><th>部门</th><th>标签</th></tr></thead>
        <tbody>
          <tr v-for="item in items" :key="item.id">
            <td>{{ item.name }}</td>
            <td>{{ item.email }}</td>
            <td>{{ item.company }}</td>
            <td>{{ item.department }}</td>
            <td>{{ item.tags }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
