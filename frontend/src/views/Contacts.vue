<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../services/api'

const items = ref([])
const importing = ref(false)
const form = reactive({ name: '', email: '', company: '', department: '', phone: '', tags: '', notes: '' })
const columns = reactive([
  { key: 'company', label: '公司', width: 380, min: 240 },
  { key: 'email', label: '邮箱', width: 320, min: 220 },
  { key: 'phone', label: '联系电话', width: 240, min: 180 },
  { key: 'tags', label: '标签', width: 220, min: 160 },
  { key: 'notes', label: '备注', width: 360, min: 220 },
])

let resizeState = null

async function load() {
  items.value = await api.contacts()
}

async function save() {
  await api.createContact({
    ...form,
    name: form.company || form.name || form.email,
  })
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

function splitValues(value) {
  return String(value || '')
    .split(/[;；]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function startResize(event, column) {
  const pointer = event.touches?.[0] || event
  resizeState = {
    column,
    startX: pointer.clientX,
    startWidth: column.width,
  }
  window.addEventListener('mousemove', resizeColumn)
  window.addEventListener('mouseup', stopResize)
  window.addEventListener('touchmove', resizeColumn, { passive: false })
  window.addEventListener('touchend', stopResize)
}

function resizeColumn(event) {
  if (!resizeState) return
  event.preventDefault?.()
  const pointer = event.touches?.[0] || event
  const nextWidth = resizeState.startWidth + pointer.clientX - resizeState.startX
  resizeState.column.width = Math.max(resizeState.column.min, nextWidth)
}

function stopResize() {
  resizeState = null
  window.removeEventListener('mousemove', resizeColumn)
  window.removeEventListener('mouseup', stopResize)
  window.removeEventListener('touchmove', resizeColumn)
  window.removeEventListener('touchend', stopResize)
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
      <label>公司名称<input v-model="form.company" required /></label>
      <label>邮箱<input v-model="form.email" type="email" required /></label>
      <label>手机号<input v-model="form.phone" /></label>
      <label>标签<input v-model="form.tags" /></label>
      <label>备注<input v-model="form.notes" /></label>
      <button>新增联系人</button>
    </form>
    <div class="panel">
      <div class="excel-table-wrap">
        <table class="contact-table">
          <colgroup>
            <col v-for="column in columns" :key="column.key" :style="{ width: `${column.width}px` }" />
          </colgroup>
          <thead>
            <tr>
              <th v-for="column in columns" :key="column.key" class="resizable-th">
                <span>{{ column.label }}</span>
                <button
                  class="resize-handle"
                  type="button"
                  :aria-label="`调整${column.label}列宽`"
                  @mousedown="startResize($event, column)"
                  @touchstart="startResize($event, column)"
                ></button>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td class="company-cell">{{ item.company || item.name }}</td>
              <td>
                <div class="chip-list">
                  <span
                    v-for="(email, index) in splitValues(item.email)"
                    :key="email"
                    class="data-chip"
                    :class="`tone-${index % 5}`"
                  >
                    {{ email }}
                  </span>
                </div>
              </td>
              <td>
                <div class="chip-list">
                  <span
                    v-for="(phone, index) in splitValues(item.phone)"
                    :key="phone"
                    class="data-chip"
                    :class="`tone-${(index + 2) % 5}`"
                  >
                    {{ phone }}
                  </span>
                </div>
              </td>
              <td>{{ item.tags }}</td>
              <td class="notes-cell">{{ item.notes }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
