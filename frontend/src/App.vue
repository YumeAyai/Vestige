<script setup>
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const drawerOpen = ref(false)

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  },
)
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
        <strong>Nous Mail</strong>
        <span>调研邮件监控</span>
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
        <span class="mark">N</span>
        <div>
          <strong>Nous Mail</strong>
          <small>调研邮件监控</small>
        </div>
      </div>
      <nav>
        <RouterLink to="/">总览</RouterLink>
        <RouterLink to="/campaigns">邮件任务</RouterLink>
        <RouterLink to="/contacts">联系人</RouterLink>
        <RouterLink to="/templates">模板</RouterLink>
        <RouterLink to="/mailboxes">发件邮箱</RouterLink>
      </nav>
    </aside>
    <main class="content">
      <RouterView />
    </main>
  </div>
</template>
