<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../services/api'

const campaigns = ref([])
const contacts = ref([])

onMounted(async () => {
  campaigns.value = await api.campaigns()
  contacts.value = await api.contacts()
})

const completed = computed(() => campaigns.value.filter((item) => item.status === 'completed').length)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>工作台</h1>
        <p class="muted">面向市场调查、通知、邀约的批量邮件发送与阅读追踪。</p>
      </div>
      <RouterLink class="button" to="/campaigns">新建邮件任务</RouterLink>
    </div>

    <div class="grid four">
      <div class="card metric"><strong>{{ campaigns.length }}</strong><span>邮件任务</span></div>
      <div class="card metric"><strong>{{ completed }}</strong><span>已完成任务</span></div>
      <div class="card metric"><strong>{{ contacts.length }}</strong><span>联系人</span></div>
      <div class="card metric"><strong>CSV</strong><span>任务结果导出</span></div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="page-head">
        <div>
          <h2>最近任务</h2>
          <p class="muted">快速查看近期邮件任务的发送状态和后续跟进入口。</p>
        </div>
      </div>
      <table>
        <thead>
          <tr><th>任务</th><th>主题</th><th>状态</th><th>创建时间</th></tr>
        </thead>
        <tbody>
          <tr v-for="item in campaigns.slice(0, 8)" :key="item.id">
            <td><RouterLink :to="`/campaigns/${item.id}`">{{ item.name }}</RouterLink></td>
            <td>{{ item.subject }}</td>
            <td><span class="status" :class="item.status">{{ item.status }}</span></td>
            <td>{{ item.created_at }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="campaigns.length === 0" class="empty">还没有邮件任务，先从创建任务开始。</p>
    </div>
  </section>
</template>
