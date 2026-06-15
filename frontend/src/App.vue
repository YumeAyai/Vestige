<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { installDialog } from './utils/dialog'

const route = useRoute()
const drawerOpen = ref(false)
const dialogInput = ref(null)
const dialog = reactive({
  open: false,
  type: 'confirm',
  title: '',
  message: '',
  value: '',
  resolve: null,
})
const navItems = [
  { to: '/', label: '工作台', icon: 'M4 13h6V4H4v9Zm10 7h6V4h-6v16ZM4 20h6v-5H4v5Z' },
  { to: '/campaigns', label: '邮件任务', icon: 'M4 6h16M4 12h16M4 18h10' },
  { to: '/stats', label: '追踪统计', icon: 'M4 19V5m0 14h16M8 16v-5m5 5V8m5 8v-2' },
  { to: '/contacts', label: '联系人', icon: 'M8 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm8 1a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM3 20a5 5 0 0 1 10 0m1-1a5 5 0 0 1 7-4.5' },
  { to: '/templates', label: '模板', icon: 'M6 4h12v16H6zM9 8h6M9 12h6M9 16h4' },
  { to: '/mailboxes', label: '发件邮箱', icon: 'M4 7h16v10H4zM4 8l8 5 8-5' },
]

const activeTitle = computed(() => {
  const match = [...navItems].reverse().find((item) =>
    item.to === '/' ? route.path === '/' : route.path.startsWith(item.to),
  )
  return match?.label || '见迹'
})

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  },
)

onMounted(() => {
  installDialog((options) => new Promise((resolve) => {
    dialog.open = true
    dialog.type = options.type || 'confirm'
    dialog.title = options.title || (dialog.type === 'prompt' ? '输入' : '确认')
    dialog.message = options.message || ''
    dialog.value = options.defaultValue || ''
    dialog.resolve = resolve
    if (dialog.type === 'prompt') {
      nextTick(() => dialogInput.value?.focus())
    }
  }))
})

function closeDialog(confirmed) {
  const resolve = dialog.resolve
  const result = dialog.type === 'prompt'
    ? (confirmed ? dialog.value : null)
    : Boolean(confirmed)
  dialog.open = false
  dialog.resolve = null
  resolve?.(result)
}
</script>

<template>
  <div class="shell" :class="{ 'drawer-open': drawerOpen }">
    <header class="mobile-topbar">
      <button class="menu-button secondary" type="button" aria-label="打开导航菜单" @click="drawerOpen = true">
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="M4 7h16M4 12h16M4 17h16" />
        </svg>
      </button>
      <div class="mobile-title">
        <strong>{{ activeTitle }}</strong>
        <span>见迹</span>
      </div>
    </header>
    <button
      v-if="drawerOpen"
      class="drawer-backdrop"
      type="button"
      aria-label="关闭导航"
      @click="drawerOpen = false"
    ></button>
    <aside class="sidebar" :aria-hidden="!drawerOpen">
      <div class="brand">
        <span class="mark">见</span>
        <div>
          <strong>见迹</strong>
          <small>Mail Ops</small>
        </div>
      </div>
      <nav aria-label="主导航">
        <RouterLink v-for="item in navItems" :key="item.to" :to="item.to">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path :d="item.icon" />
          </svg>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
    </aside>
    <main class="content">
      <RouterView />
    </main>
    <div v-if="dialog.open" class="dialog-backdrop" @click.self="closeDialog(false)">
      <form class="dialog-panel" @submit.prevent="closeDialog(true)">
        <h2>{{ dialog.title }}</h2>
        <p>{{ dialog.message }}</p>
        <input
          v-if="dialog.type === 'prompt'"
          ref="dialogInput"
          v-model="dialog.value"
          autocomplete="off"
        />
        <div class="dialog-actions">
          <button class="secondary" type="button" @click="closeDialog(false)">取消</button>
          <button type="submit">确定</button>
        </div>
      </form>
    </div>
  </div>
</template>
