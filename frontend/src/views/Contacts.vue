<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../services/api'

const router = useRouter()
const items = ref([])
const importing = ref(false)
const total = ref(0)
const offset = ref(0)
const pageSize = ref(20)
const query = ref('')
const selectedIds = ref([])
const batch = reactive({ tags: '', notes: '' })
const notice = ref('')
const form = reactive({ name: '', email: '', company: '', department: '', phone: '', tags: '', notes: '' })
const columns = reactive([
  { key: 'select', label: '', width: 54, min: 54 },
  { key: 'company', label: '公司', width: 380, min: 240 },
  { key: 'email', label: '邮箱', width: 320, min: 220 },
  { key: 'phone', label: '联系电话', width: 240, min: 180 },
  { key: 'tags', label: '标签', width: 220, min: 160 },
  { key: 'notes', label: '备注', width: 360, min: 220 },
])

let resizeState = null

const pageStart = computed(() => (total.value === 0 ? 0 : offset.value + 1))
const pageEnd = computed(() => Math.min(offset.value + pageSize.value, total.value))
const allPageSelected = computed(() => items.value.length > 0 && items.value.every((item) => selectedIds.value.includes(item.id)))
const hasPrevious = computed(() => offset.value > 0)
const hasNext = computed(() => offset.value + pageSize.value < total.value)

async function load() {
  const data = await loadContactPage()
  items.value = data.items || []
  total.value = data.total || 0
  selectedIds.value = selectedIds.value.filter((id) => items.value.some((item) => item.id === id))
}

async function loadContactPage() {
  try {
    return await api.contactsPage({ limit: pageSize.value, offset: offset.value, q: query.value })
  } catch (error) {
    const all = await api.contacts()
    const filtered = filterContacts(all)
    return {
      items: filtered.slice(offset.value, offset.value + pageSize.value),
      total: filtered.length,
      limit: pageSize.value,
      offset: offset.value,
    }
  }
}

function filterContacts(list) {
  const keyword = query.value.trim().toLowerCase()
  if (!keyword) return list
  return list.filter((item) => [item.company, item.name, item.email, item.phone, item.tags, item.notes]
    .some((value) => String(value || '').toLowerCase().includes(keyword)))
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
    offset.value = 0
    await load()
  } finally {
    importing.value = false
    event.target.value = ''
  }
}

function search() {
  offset.value = 0
  load()
}

function changePageSize(size) {
  pageSize.value = size
  offset.value = 0
  load()
}

function previousPage() {
  if (!hasPrevious.value) return
  offset.value = Math.max(0, offset.value - pageSize.value)
  load()
}

function nextPage() {
  if (!hasNext.value) return
  offset.value += pageSize.value
  load()
}

function isSelected(id) {
  return selectedIds.value.includes(id)
}

function toggleOne(id) {
  const index = selectedIds.value.indexOf(id)
  if (index >= 0) {
    selectedIds.value.splice(index, 1)
  } else {
    selectedIds.value.push(id)
  }
}

function togglePage() {
  if (allPageSelected.value) {
    selectedIds.value = selectedIds.value.filter((id) => !items.value.some((item) => item.id === id))
  } else {
    selectedIds.value = Array.from(new Set([...selectedIds.value, ...items.value.map((item) => item.id)]))
  }
}

async function applyBatchUpdate() {
  if (selectedIds.value.length === 0) return
  const result = await api.updateContactsBatch({ ids: selectedIds.value, tags: batch.tags, notes: batch.notes })
  notice.value = `已更新 ${result.updated} 个联系人`
  batch.tags = ''
  batch.notes = ''
  await load()
}

async function removeSelected() {
  if (selectedIds.value.length === 0) return
  if (!confirm(`确认删除 ${selectedIds.value.length} 个未被任务使用的联系人？`)) return
  const result = await api.deleteContactsBatch(selectedIds.value)
  notice.value = `已删除 ${result.deleted} 个联系人；已被邮件任务使用的联系人会保留`
  selectedIds.value = []
  await load()
}

function jumpToCampaign() {
  if (selectedIds.value.length === 0) return
  router.push({ path: '/campaigns', query: { contacts: selectedIds.value.join(',') } })
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
    <div class="contact-actions">
      <input v-model="query" placeholder="搜索公司、邮箱、电话、标签或备注" @keydown.enter.prevent="search" />
      <button class="secondary" @click="search">搜索</button>
      <button class="secondary" :disabled="selectedIds.length === 0" @click="jumpToCampaign">创建邮件任务</button>
    </div>
    <form class="panel grid five contact-form" @submit.prevent="save">
      <label>公司名称<input v-model="form.company" required /></label>
      <label>邮箱<input v-model="form.email" type="email" required /></label>
      <label>手机号<input v-model="form.phone" /></label>
      <label>标签<input v-model="form.tags" /></label>
      <label>备注<input v-model="form.notes" /></label>
      <button class="contact-submit">新增联系人</button>
    </form>
    <div class="panel contact-grid-card">
      <div class="bulk-bar">
        <strong>已选 {{ selectedIds.length }} / 本页 {{ items.length }}</strong>
        <input v-model="batch.tags" placeholder="批量设置标签" />
        <input v-model="batch.notes" placeholder="批量设置备注" />
        <button class="secondary" :disabled="selectedIds.length === 0 || (!batch.tags && !batch.notes)" @click="applyBatchUpdate">批量更新</button>
        <button class="secondary danger" :disabled="selectedIds.length === 0" @click="removeSelected">批量删除</button>
      </div>
      <p v-if="notice" class="notice success">{{ notice }}</p>
      <div class="excel-table-wrap">
        <table class="contact-table">
          <colgroup>
            <col v-for="column in columns" :key="column.key" :style="{ width: `${column.width}px` }" />
          </colgroup>
          <thead>
            <tr>
              <th v-for="column in columns" :key="column.key" class="resizable-th">
                <span v-if="column.key !== 'select'">{{ column.label }}</span>
                <input v-else class="contact-check" type="checkbox" :checked="allPageSelected" @change="togglePage" />
                <button
                  v-if="column.key !== 'select'"
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
              <td class="select-cell"><input class="contact-check" type="checkbox" :checked="isSelected(item.id)" @change="toggleOne(item.id)" /></td>
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
      <div class="pager">
        <span>{{ pageStart }}-{{ pageEnd }} / {{ total }}</span>
        <div class="page-size">
          <button
            v-for="size in [20, 50, 100, 200]"
            :key="size"
            class="secondary"
            :class="{ active: pageSize === size }"
            @click="changePageSize(size)"
          >
            {{ size }}
          </button>
        </div>
        <button class="secondary" :disabled="!hasPrevious" @click="previousPage">上一页</button>
        <button class="secondary" :disabled="!hasNext" @click="nextPage">下一页</button>
      </div>
    </div>
  </section>
</template>
