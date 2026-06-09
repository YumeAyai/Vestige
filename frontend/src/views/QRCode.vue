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
    color: ['#3157a4'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: hours,
      axisLine: { lineStyle: { color: '#d9dee8' } },
      axisLabel: { color: '#667085', rotate: 45 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#e8ecf2' } },
      axisLabel: { color: '#667085' },
    },
    series: [{
      name: '图片加载',
      type: 'line',
      smooth: true,
      symbolSize: 7,
      lineStyle: { width: 3 },
      areaStyle: { color: 'rgba(54, 86, 166, 0.1)' },
      data,
    }],
  })
}

function parseDevice(ua) {
  if (!ua) return '未知'
  if (ua.includes('iPhone')) return 'iPhone'
  if (ua.includes('iPad')) return 'iPad'
  if (ua.includes('Android')) return 'Android'
  if (ua.includes('Mac')) return 'Mac'
  if (ua.includes('Windows')) return 'Windows'
  if (ua.includes('Linux')) return 'Linux'
  return '其他'
}

function parseBrowser(ua) {
  if (!ua) return '未知'
  if (ua.includes('MicroMessenger')) return '微信'
  if (ua.includes('QQ/')) return 'QQ'
  if (ua.includes('Outlook')) return 'Outlook'
  if (ua.includes('Edg/')) return 'Edge'
  if (ua.includes('Firefox/')) return 'Firefox'
  if (ua.includes('Chrome/')) return 'Chrome'
  if (ua.includes('Safari/') && !ua.includes('Chrome/')) return 'Safari'
  if (ua.includes('AppleWebKit')) return 'WebKit'
  return '其他'
}

function parseIP(ip) {
  if (!ip) return '-'
  return ip
}

function formatSource(source) {
  if (source === 'cloud') return '远端'
  if (source === 'local') return '本地'
  return source || '-'
}

function refererHost(referer) {
  if (!referer) return ''
  try {
    return new URL(referer).host
  } catch {
    return referer
  }
}

function formatOrigin(item) {
  if (!item?.qr_load_count) return '-'
  const parts = [formatSource(item.last_qr_source)]
  if (item.last_qr_forwarded_for) parts.push('经代理')
  const host = refererHost(item.last_qr_referer)
  parts.push(host ? `来自 ${host}` : '直接请求')
  return parts.filter(Boolean).join(' / ')
}

function formatPrefetch(value) {
  return value ? '疑似预加载' : '正常加载'
}

function pad(value) {
  return String(value).padStart(2, '0')
}

function formatDateTime(value) {
  if (!value) return '-'
  const text = String(value).trim()
  const normalized = text.includes('T') ? text : text.replace(' ', 'T')
  const date = new Date(normalized)
  if (Number.isNaN(date.getTime())) return text
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function exportCSV() {
  const rows = [['公司', '邮箱', '发送状态', '像素打开', '图片加载次数', '首次图片加载', '最近图片加载', '来源判断', 'IP', '预加载', '浏览器', '设备']]
  for (const r of visibleRecipients.value) {
    rows.push([
      r.name,
      r.email,
      r.send_status,
      r.open_count,
      r.qr_load_count,
      formatDateTime(r.first_qr_load_at),
      formatDateTime(r.last_qr_load_at),
      formatOrigin(r),
      parseIP(r.last_qr_ip),
      formatPrefetch(r.last_qr_is_prefetch),
      parseBrowser(r.last_qr_user_agent),
      parseDevice(r.last_qr_user_agent)
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
        <h1>图片埋点 - {{ campaign.name }}</h1>
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
        <span>已加载</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_load_events || 0 }}</strong>
        <span>加载次数</span>
      </div>
      <div class="card metric">
        <strong>{{ stats.summary.qr_not_loaded || 0 }}</strong>
        <span>未加载</span>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <h2>加载趋势</h2>
      <div ref="chartEl" class="chart"></div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="page-head">
        <div>
          <h2>加载记录</h2>
          <p class="muted">埋点图片加载详情，包含远端同步来源、请求头、预加载判断和设备信息。</p>
        </div>
        <select v-model="filter" style="max-width: 160px">
          <option value="all">全部</option>
          <option value="loaded">已加载</option>
          <option value="unloaded">未加载</option>
        </select>
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-company">公司</th>
            <th>邮箱</th>
            <th>发送</th>
            <th>像素打开</th>
            <th>加载次数</th>
            <th>首次加载</th>
            <th>最近加载</th>
            <th>来源判断</th>
            <th>IP</th>
            <th>预加载</th>
            <th>浏览器</th>
            <th>设备</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in visibleRecipients" :key="item.id">
            <td class="company-cell">{{ item.name }}</td>
            <td><span class="data-chip tone-0">{{ item.email }}</span></td>
            <td><span :class="['status', item.send_status]">{{ item.send_status }}</span></td>
            <td>{{ item.open_count }}</td>
            <td>{{ item.qr_load_count }}</td>
            <td>{{ formatDateTime(item.first_qr_load_at) }}</td>
            <td>{{ formatDateTime(item.last_qr_load_at) }}</td>
            <td>{{ formatOrigin(item) }}</td>
            <td>{{ parseIP(item.last_qr_ip) }}</td>
            <td>{{ item.qr_load_count > 0 ? formatPrefetch(item.last_qr_is_prefetch) : '-' }}</td>
            <td>{{ parseBrowser(item.last_qr_user_agent) }}</td>
            <td>{{ parseDevice(item.last_qr_user_agent) }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="visibleRecipients.length === 0" class="empty">暂无符合条件的记录</p>
    </div>
  </section>
</template>
