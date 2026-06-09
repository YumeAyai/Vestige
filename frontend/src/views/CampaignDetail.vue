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
  if (filter.value === 'qr_loaded') return recipients.value.filter((item) => item.qr_load_count > 0)
  if (filter.value === 'qr_unloaded') return recipients.value.filter((item) => item.qr_load_count === 0)
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
  const hours = Array.from(
    new Set([
      ...stats.value.trend.map((item) => item.hour),
      ...(stats.value.qr_trend || []).map((item) => item.hour),
    ]),
  ).sort()
  const pixelByHour = new Map(stats.value.trend.map((item) => [item.hour, item.count]))
  const qrByHour = new Map((stats.value.qr_trend || []).map((item) => [item.hour, item.count]))
  chart.setOption({
    color: ['#00a376', '#3656a6'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 8, textStyle: { color: '#687b72' } },
    xAxis: {
      type: 'category',
      data: hours,
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
        name: '像素加载',
        type: 'line',
        smooth: true,
        symbolSize: 7,
        lineStyle: { width: 3 },
        areaStyle: { color: 'rgba(0, 163, 118, 0.12)' },
        data: hours.map((hour) => pixelByHour.get(hour) || 0),
      },
      {
        name: '二维码加载',
        type: 'line',
        smooth: true,
        symbolSize: 7,
        lineStyle: { width: 3 },
        areaStyle: { color: 'rgba(54, 86, 166, 0.1)' },
        data: hours.map((hour) => qrByHour.get(hour) || 0),
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

    <div class="grid five">
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
        ><span>像素加载人数</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_loaded || 0 }}</strong
        ><span>二维码加载人数</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.failed || 0 }}</strong
        ><span>发送失败</span>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <h2>加载趋势</h2>
      <div ref="chartEl" class="chart"></div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="page-head">
        <div>
          <h2>收件人明细</h2>
          <p class="muted">二维码加载是正文图片请求记录，比普通像素更适合判断邮件内容是否被加载。</p>
        </div>
        <select v-model="filter" style="max-width: 180px">
          <option value="all">全部</option>
          <option value="qr_loaded">二维码已加载</option>
          <option value="qr_unloaded">二维码未加载</option>
          <option value="opened">像素已加载</option>
          <option value="unopened">像素未加载</option>
          <option value="failed">发送失败</option>
        </select>
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-company">公司</th>
            <th>邮箱</th>
            <th>发送</th>
            <th>二维码加载</th>
            <th>首次二维码加载</th>
            <th>最近 IP</th>
            <th>像素加载</th>
            <th>失败原因</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in visibleRecipients" :key="item.id">
            <td class="company-cell">{{ item.name }}</td>
            <td><span class="data-chip tone-0">{{ item.email }}</span></td>
            <td>
              <span class="status" :class="item.send_status">{{ item.send_status }}</span>
            </td>
            <td>{{ item.qr_load_count }}</td>
            <td>{{ item.first_qr_load_at || '-' }}</td>
            <td>{{ item.last_qr_ip || '-' }}</td>
            <td>{{ item.open_count }}</td>
            <td>{{ item.failure_reason }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="visibleRecipients.length === 0" class="empty">暂无符合条件的收件人</p>
    </div>
  </section>
</template>
