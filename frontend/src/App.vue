<template>
  <div class="shell">
    <aside class="sidebar">
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
