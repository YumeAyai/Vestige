<script setup>
import * as echarts from 'echarts'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../services/api'

const route = useRoute()
const campaign = ref(null)
const stats = ref({ summary: {}, trend: [] })
const recipients = ref([])
const chartEl = ref(null)
const filter = ref('all')

const visibleRecipients = computed(() => {
  if (filter.value === 'loaded') return recipients.value.filter(r => r.qr_load_count > 0)
  if (filter.value === 'unloaded') return recipients.value.filter(r => r.qr_load_count === 0)
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
  const hours = stats.value.qr_trend?.map(t => t.hour) || []
  const data = stats.value.qr_trend?.map(t => t.count) || []

  chart.setOption({
    color: ['#3656a6'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: hours,
      axisLine: { lineStyle: { color: '#d8e3dd' } },
      axisLabel: { color: '#687b72', rotate: 45 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#edf3ef' } },
      axisLabel: { color: '#687b72' },
    },
    series: [{
      name: '二维码加载',
      type: 'line',
      smooth: true,
      symbolSize: 7,
      lineStyle: { width: 3 },
      areaStyle: { color: 'rgba(54, 86, 166, 0.1)' },
      data,
    }],
  })
}

function parseUserAgent(ua) {
  if (!ua) return '未知'
  if (ua.includes('iPhone')) return 'iPhone'
  if (ua.includes('iPad')) return 'iPad'
  if (ua.includes('Android')) return 'Android'
  if (ua.includes('Mac')) return 'Mac'
  if (ua.includes('Windows')) return 'Windows'
  if (ua.includes('Linux')) return 'Linux'
  return '其他'
}

function parseIP(ip) {
  if (!ip) return '-'
  return ip
}

function exportCSV() {
  const rows = [['公司', '邮箱', '加载次数', '首次加载', '最近加载', 'IP', '设备']]
  for (const r of visibleRecipients.value) {
    rows.push([
      r.name,
      r.email,
      r.qr_load_count,
      r.first_qr_load_at || '',
      r.last_qr_load_at || '',
      parseIP(r.last_qr_ip),
      parseUserAgent(r.last_qr_user_agent)
    ])
  }
  const csv = rows.map(r => r.map(c => `"${c}"`).join(',')).join('\n')
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `qrcode-${route.params.id}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(load)
</script>

<template>
  <section v-if="campaign">
    <div class="page-head">
      <div>
        <h1>二维码追踪 - {{ campaign.name }}</h1>
        <p class="muted">{{ campaign.subject }}</p>
      </div>
      <button class="button secondary" @click="exportCSV">导出 CSV</button>
    </div>

    <div class="grid four">
      <div class="card metric">
        <strong>{{ stats.summary.total || 0 }}</strong>
        <span>收件人</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_loaded || 0 }}</strong>
        <span>已扫描</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_load_events || 0 }}</strong>
        <span>扫描次数</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_not_loaded || 0 }}</strong>
        <span>未扫描</span>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <h2>扫描趋势</h2>
      <div ref="chartEl" class="chart"></div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="page-head">
        <div>
          <h2>扫描记录</h2>
          <p class="muted">二维码扫描详情，包含时间、IP 和设备信息。</p>
        </div>
        <select v-model="filter" style="max-width: 160px">
          <option value="all">全部</option>
          <option value="loaded">已扫描</option>
          <option value="unloaded">未扫描</option>
        </select>
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-company">公司</th>
            <th>邮箱</th>
            <th>扫描次数</th>
            <th>首次扫描</th>
            <th>最近扫描</th>
            <th>IP</th>
            <th>设备</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in visibleRecipients" :key="item.id">
            <td class="company-cell">{{ item.name }}</td>
            <td><span class="data-chip tone-0">{{ item.email }}</span></td>
            <td>{{ item.qr_load_count }}</td>
            <td>{{ item.first_qr_load_at || '-' }}</td>
            <td>{{ item.last_qr_load_at || '-' }}</td>
            <td>{{ parseIP(item.last_qr_ip) }}</td>
            <td>{{ parseUserAgent(item.last_qr_user_agent) }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="visibleRecipients.length === 0" class="empty">暂无符合条件的记录</p>
    </div>
  </section>
</template>