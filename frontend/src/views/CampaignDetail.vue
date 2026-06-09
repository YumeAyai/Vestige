<script setup>
import * as echarts from 'echarts'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../services/api'

const route = useRoute()
const campaign = ref(null)
const stats = ref({ summary: {}, trend: [] })
const recipients = ref([])
const sending = ref(false)
const chartEl = ref(null)
const filter = ref('all')

const visibleRecipients = computed(() => {
  if (filter.value === 'opened') return recipients.value.filter((item) => item.open_count > 0)
  if (filter.value === 'unopened') return recipients.value.filter((item) => item.open_count === 0)
  if (filter.value === 'failed')
    return recipients.value.filter((item) => item.send_status === 'failed')
  return recipients.value
})

async function load() {
  campaign.value = await api.campaign(route.params.id)
  stats.value = await api.campaignStats(route.params.id)
  recipients.value = await api.recipients(route.params.id)
  await nextTick()
  renderChart()
}

function renderChart() {
  if (!chartEl.value) return
  const chart = echarts.init(chartEl.value)
  chart.setOption({
    color: ['#00a376'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: stats.value.trend.map((item) => item.hour),
      axisLine: { lineStyle: { color: '#d8e3dd' } },
      axisLabel: { color: '#687b72' },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#edf3ef' } },
      axisLabel: { color: '#687b72' },
    },
    series: [
      {
        name: '阅读次数',
        type: 'line',
        smooth: true,
        symbolSize: 7,
        lineStyle: { width: 3 },
        areaStyle: { color: 'rgba(0, 163, 118, 0.12)' },
        data: stats.value.trend.map((item) => item.count),
      },
    ],
  })
}

async function send() {
  sending.value = true
  try {
    await api.sendCampaign(route.params.id)
    await load()
  } finally {
    sending.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="campaign">
    <div class="page-head">
      <div>
        <h1>{{ campaign.name }}</h1>
        <p class="muted">{{ campaign.subject }}</p>
      </div>
      <div class="toolbar">
        <a class="button secondary" :href="`/api/campaigns/${campaign.id}/export.csv`">导出 CSV</a>
        <button :disabled="sending" @click="send">
          {{ sending ? '发送中' : '开始/重试发送' }}
        </button>
      </div>
    </div>

    <div class="grid four">
      <div class="card metric">
        <strong>{{ stats.summary.total || 0 }}</strong
        ><span>收件人</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.sent || 0 }}</strong
        ><span>发送成功</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.opened || 0 }}</strong
        ><span>已阅读</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.failed || 0 }}</strong
        ><span>发送失败</span>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <h2>阅读趋势</h2>
      <div ref="chartEl" class="chart"></div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="page-head">
        <div>
          <h2>收件人明细</h2>
          <p class="muted">阅读状态基于图片加载判断，数据用于跟进参考。</p>
        </div>
        <select v-model="filter" style="max-width: 180px">
          <option value="all">全部</option>
          <option value="opened">已阅读</option>
          <option value="unopened">未阅读</option>
          <option value="failed">发送失败</option>
        </select>
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-company">公司</th>
            <th>邮箱</th>
            <th>发送</th>
            <th>阅读次数</th>
            <th>首次阅读</th>
            <th>失败原因</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in visibleRecipients" :key="item.id">
            <td class="company-cell">{{ item.name }}</td>
            <td><span class="email-chip">{{ item.email }}</span></td>
            <td>
              <span class="status" :class="item.send_status">{{ item.send_status }}</span>
            </td>
            <td>{{ item.open_count }}</td>
            <td>{{ item.first_opened_at || '-' }}</td>
            <td>{{ item.failure_reason }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="visibleRecipients.length === 0" class="empty">暂无符合条件的收件人</p>
    </div>
  </section>
</template>
