<script setup>
import * as echarts from 'echarts'
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../services/api'

const stats = ref({ summary: {}, trend: [] })
const campaigns = ref([])
const selectedCampaign = ref('')
const since = ref(todayDate())
const chartEl = ref(null)

function todayDate() {
  const now = new Date()
  const offset = now.getTimezoneOffset() * 60000
  return new Date(now.getTime() - offset).toISOString().slice(0, 10)
}

const openRate = computed(() => {
  const s = stats.value.summary
  if (!s.total_sent) return 0
  return ((s.total_opened / s.total_sent) * 100).toFixed(1)
})

const clickRate = computed(() => {
  const s = stats.value.summary
  if (!s.total_sent) return 0
  return ((s.total_clicked / s.total_sent) * 100).toFixed(1)
})

const qrRate = computed(() => {
  const s = stats.value.summary
  if (!s.total_sent) return 0
  return ((s.total_qr_loaded / s.total_sent) * 100).toFixed(1)
})

async function load() {
  stats.value = await api.globalStats({ since: since.value, campaign: selectedCampaign.value })
  campaigns.value = await api.campaigns()
  await nextTick()
  renderChart()
}

function renderChart() {
  if (!chartEl.value) return
  const chart = echarts.init(chartEl.value)
  const dates = stats.value.trend.map(t => t.date)
  const sentData = stats.value.trend.map(t => t.sent)
  const openedData = stats.value.trend.map(t => t.opened)
  const clickedData = stats.value.trend.map(t => t.clicked)

  chart.setOption({
    color: ['#98a2b3', '#087f8c', '#3157a4'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 8, textStyle: { color: '#667085' } },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: '#d9dee8' } },
      axisLabel: { color: '#667085' },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#e8ecf2' } },
      axisLabel: { color: '#667085' },
    },
    series: [
      {
        name: '发送',
        type: 'bar',
        barWidth: '40%',
        itemStyle: { color: '#e8e4df' },
        data: sentData,
      },
      {
        name: '打开',
        type: 'line',
        smooth: true,
        symbolSize: 7,
        lineStyle: { width: 2 },
        data: openedData,
      },
      {
        name: '点击',
        type: 'line',
        smooth: true,
        symbolSize: 7,
        lineStyle: { width: 2 },
        data: clickedData,
      },
    ],
  })
}

function filter() {
  load()
}

onMounted(load)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>追踪统计</h1>
        <p class="muted">全局邮件追踪数据概览，包含发送、打开、点击和图片加载。</p>
      </div>
    </div>

    <div class="toolbar" style="margin-bottom: 16px">
      <select v-model="selectedCampaign" @change="filter">
        <option value="">全部任务</option>
        <option v-for="c in campaigns" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <input v-model="since" type="date" @change="filter" />
    </div>

    <div class="grid four">
      <div class="card metric">
        <strong>{{ stats.summary.total_sent || 0 }}</strong>
        <span>发送总数</span>
      </div>
      <div class="card metric">
        <strong>{{ openRate }}%</strong>
        <span>打开率</span>
      </div>
      <div class="card metric">
        <strong>{{ clickRate }}%</strong>
        <span>点击率</span>
      </div>
      <div class="card metric">
        <strong>{{ qrRate }}%</strong>
        <span>图片加载率</span>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <h2>趋势图</h2>
      <p class="muted">最近 30 天发送、打开和点击趋势</p>
      <div ref="chartEl" class="chart"></div>
    </div>

    <div class="grid two" style="margin-top: 16px">
      <div class="panel">
        <h3>任务统计</h3>
        <table>
          <thead>
            <tr><th>任务</th><th>发送</th><th>打开</th><th>点击</th></tr>
          </thead>
          <tbody>
            <tr v-for="c in stats.campaigns || []" :key="c.id">
              <td><RouterLink :to="`/campaigns/${c.id}`">{{ c.name }}</RouterLink></td>
              <td>{{ c.sent ?? 0 }}</td>
              <td>{{ c.opened ?? 0 }}</td>
              <td>{{ c.clicked ?? 0 }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="panel">
        <h3>最近事件</h3>
        <p class="muted">最新追踪事件记录</p>
      </div>
    </div>
  </section>
</template>
