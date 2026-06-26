<script setup>
import * as echarts from 'echarts'
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../services/api'
import { formatDateTime, formatLocalHour } from '../utils/time'
import { askConfirm } from '../utils/dialog'

const route = useRoute()
const campaign = ref(null)
const stats = ref({ summary: {}, trend: [] })
const recipients = ref([])
const links = ref([])
const abStats = ref(null)
const sending = ref(false)
const sendError = ref('')
const chartEl = ref(null)
const rateChartEl = ref(null)
const funnelChartEl = ref(null)
const engagementTrendEl = ref(null)
const linkChartEl = ref(null)
const abChartEl = ref(null)
const imageChartEl = ref(null)
const linkEngagement = ref({ totalClicks: 0, uniqueClicks: 0, trend: [] })
const filter = ref('all')
const imageFilter = ref('all')
const activeTab = ref('overview')
const editingVariantId = ref(null)
const variantError = ref('')
const variantBodyMode = ref('preview')
const variantPreview = ref({ subject: '', body_html: '' })
const variantPreviewError = ref('')
const variantForm = reactive({
  name: '',
  subject: '',
  body_html: '',
  weight: 50,
})

const overviewMetrics = computed(() => {
  const summary = stats.value.summary || {}
  const total = summary.total || 0
  const sent = summary.sent || 0
  const failed = summary.failed || 0
  const pending = summary.pending ?? Math.max(total - sent - failed - (summary.waiting || 0), 0)
  const waiting = summary.waiting || 0
  const opened = summary.opened || 0
  const imageLoaded = summary.qr_loaded || 0
  const uniqueClicks = summary.clicked ?? linkEngagement.value.uniqueClicks ?? 0
  const totalClicks = summary.click_events ?? linkEngagement.value.totalClicks ?? 0
  return {
    total,
    sent,
    failed,
    pending,
    waiting,
    opened,
    imageLoaded,
    totalClicks,
    uniqueClicks,
    sendRate: percent(sent, total),
    pixelRate: percent(opened, sent),
    imageLoadRate: percent(imageLoaded, sent),
    clickReturnRate: percent(uniqueClicks, sent),
    clickAfterLoadRate: percent(uniqueClicks, imageLoaded),
  }
})

const visibleRecipients = computed(() => {
  if (filter.value === 'opened') return recipients.value.filter((item) => countValue(item.open_count) > 0)
  if (filter.value === 'qr_loaded') return recipients.value.filter((item) => countValue(item.qr_load_count) > 0)
  if (filter.value === 'qr_unloaded') return recipients.value.filter((item) => countValue(item.qr_load_count) === 0)
  if (filter.value === 'unopened') return recipients.value.filter((item) => countValue(item.open_count) === 0)
  if (filter.value === 'pending')
    return recipients.value.filter((item) => item.send_status === 'pending')
  if (filter.value === 'waiting')
    return recipients.value.filter((item) => item.send_status === 'waiting')
  if (filter.value === 'failed')
    return recipients.value.filter((item) => item.send_status === 'failed')
  return recipients.value
})

const imageRecipients = computed(() => {
  if (imageFilter.value === 'loaded') return recipients.value.filter((item) => countValue(item.qr_load_count) > 0)
  if (imageFilter.value === 'unloaded') return recipients.value.filter((item) => countValue(item.qr_load_count) === 0)
  if (imageFilter.value === 'prefetch') return recipients.value.filter((item) => item.last_qr_is_prefetch)
  return recipients.value
})

const variantPreviewContact = computed(() => {
  const selected = recipients.value[0]
  return selected
    ? {
        name: selected.name,
        email: selected.email,
        company: selected.name,
      }
    : {
        name: '上海示例企业有限公司',
        email: 'contact@example.com',
        company: '上海示例企业有限公司',
      }
})

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

function splitValues(value) {
  return String(value || '')
    .split(/[;；]/)
    .map((item) => item.trim())
    .filter(Boolean)
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

function countValue(value) {
  const count = Number(value)
  return Number.isFinite(count) && count >= 0 ? count : 0
}

function formatLoadCount(value) {
  return countValue(value)
}

function isIPPortraitLoading(item) {
  if (!item?.qr_load_count || !item.last_qr_ip) return false
  return !String(item.last_qr_ip_risk || '').trim()
}

function formatIPType(item) {
  if (!item?.qr_load_count) return '-'
  const summary = String(item.last_qr_ip_risk || '').trim()
  if (!summary) return item.last_qr_ip ? '查询中' : '-'
  const parts = summary.split('/').map((part) => part.trim()).filter(Boolean)
  if (parts.some((part) => part.includes('家庭宽带'))) return '家庭宽带'
  if (parts.some((part) => part.includes('基站'))) return '基站'
  if (parts.some((part) => part.includes('商业宽带') || part.includes('商用宽带') || part.includes('企业宽带') || part.includes('企业专线'))) return '商用宽带'
  if (parts.some((part) => /机房|IDC|数据中心|云主机|云服务|服务器|托管/i.test(part))) return '机房/IDC'
  if (parts.some((part) => /代理|VPN|CDN/i.test(part))) return '代理IP'
  return parts.find((part) => !part.startsWith('风险')) || summary
}

function sendStatusLabel(status) {
  return {
    pending: '就绪',
    waiting: '队列中',
    sent: '已发送',
    failed: '发送失败',
  }[status] || status
}

function percent(count, total) {
  if (!total) return 0
  return Number(((count / total) * 100).toFixed(1))
}

function formatPercent(value) {
  return `${Number(value || 0).toFixed(1)}%`
}

async function load() {
  campaign.value = await api.campaign(route.params.id)
  if (!variantForm.subject && !variantForm.body_html) {
    resetVariantForm()
  }
  stats.value = await api.campaignStats(route.params.id)
  recipients.value = await api.recipients(route.params.id)
  links.value = await api.links(route.params.id).catch(() => [])
  await loadLinkEngagement()
  abStats.value = await api.abStats(route.params.id).catch(() => null)
  await nextTick()
  renderChart()
  renderRateChart()
  renderFunnelChart()
  renderEngagementTrend()
  if (links.value.length > 0) renderLinkChart()
  if (abStats.value?.variants?.length > 0) renderABChart()
}

async function loadLinkEngagement() {
  const aggregateTrend = new Map()
  let totalClicks = 0
  let uniqueClicks = 0
  await Promise.all(links.value.map(async (link) => {
    try {
      const data = await api.linkStats(route.params.id, link.id)
      totalClicks += data.summary?.total_clicks || 0
      uniqueClicks += data.summary?.unique_clicks || 0
      ;(data.trend || []).forEach((item) => {
        aggregateTrend.set(item.hour, (aggregateTrend.get(item.hour) || 0) + (item.count || 0))
      })
    } catch {
      // Keep the overview usable even when a single link stat request fails.
    }
  }))
  linkEngagement.value = {
    totalClicks,
    uniqueClicks,
    trend: Array.from(aggregateTrend.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([hour, count]) => ({ hour, count })),
  }
}

function renderChart() {
  if (!chartEl.value) return
  const chart = echarts.init(chartEl.value)
  const { sent, failed, pending, waiting } = overviewMetrics.value
  chart.setOption({
    color: ['#067647', '#d92d20', '#667085', '#2563eb'],
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { color: '#667085' } },
    xAxis: {
      show: false,
    },
    yAxis: {
      show: false,
    },
    series: [
      {
        name: '发送状态',
        type: 'pie',
        radius: ['52%', '74%'],
        center: ['50%', '44%'],
        avoidLabelOverlap: true,
        label: { color: '#344054', formatter: '{b} {c}' },
        data: [
          { name: '已发送', value: sent },
          { name: '失败', value: failed },
          { name: '就绪', value: pending },
          { name: '队列中', value: waiting },
        ],
      },
    ],
  })
}

function renderRateChart() {
  if (!rateChartEl.value) return
  const chart = echarts.init(rateChartEl.value)
  const metrics = overviewMetrics.value
  chart.setOption({
    color: ['#087f8c'],
    grid: { left: 68, right: 24, top: 20, bottom: 28 },
    tooltip: { trigger: 'axis', valueFormatter: (value) => formatPercent(value) },
    xAxis: {
      type: 'value',
      max: 100,
      axisLabel: { color: '#667085', formatter: '{value}%' },
      splitLine: { lineStyle: { color: '#e8ecf2' } },
    },
    yAxis: {
      type: 'category',
      data: ['发送率', '内容加载率', '像素加载率', '点击回报率'],
      axisLabel: { color: '#344054' },
      axisLine: { show: false },
      axisTick: { show: false },
    },
    series: [{
      name: '转化率',
      type: 'bar',
      barWidth: 18,
      label: { show: true, position: 'right', color: '#344054', formatter: ({ value }) => formatPercent(value) },
      data: [metrics.sendRate, metrics.imageLoadRate, metrics.pixelRate, metrics.clickReturnRate],
    }],
  })
}

function renderFunnelChart() {
  if (!funnelChartEl.value) return
  const chart = echarts.init(funnelChartEl.value)
  const metrics = overviewMetrics.value
  chart.setOption({
    color: ['#3157a4', '#087f8c', '#a16207', '#d92d20'],
    tooltip: { trigger: 'item', formatter: '{b}: {c}' },
    series: [{
      name: '响应漏斗',
      type: 'funnel',
      left: '8%',
      top: 20,
      bottom: 20,
      width: '84%',
      minSize: '18%',
      maxSize: '100%',
      sort: 'descending',
      gap: 4,
      label: { color: '#344054', formatter: '{b} {c}' },
      data: [
        { name: '收件人', value: metrics.total },
        { name: '已发送', value: metrics.sent },
        { name: '内容加载', value: metrics.imageLoaded },
        { name: '唯一点击', value: metrics.uniqueClicks },
      ],
    }],
  })
}

function renderEngagementTrend() {
  if (!engagementTrendEl.value) return
  const chart = echarts.init(engagementTrendEl.value)
  const hours = Array.from(
    new Set([
      ...(stats.value.trend || []).map((item) => item.hour),
      ...(stats.value.qr_trend || []).map((item) => item.hour),
      ...(linkEngagement.value.trend || []).map((item) => item.hour),
    ]),
  ).sort()
  const pixelByHour = new Map((stats.value.trend || []).map((item) => [item.hour, item.count]))
  const imageByHour = new Map((stats.value.qr_trend || []).map((item) => [item.hour, item.count]))
  const clickByHour = new Map((linkEngagement.value.trend || []).map((item) => [item.hour, item.count]))
  const hourLabels = hours.map(formatLocalHour)
  chart.setOption({
    color: ['#087f8c', '#3157a4', '#a16207'],
    grid: { left: 36, right: 18, top: 32, bottom: 42 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 8, textStyle: { color: '#667085' } },
    xAxis: {
      type: 'category',
      data: hourLabels,
      axisLine: { lineStyle: { color: '#d9dee8' } },
      axisLabel: { color: '#667085', rotate: 35 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#e8ecf2' } },
      axisLabel: { color: '#667085' },
    },
    series: [
      { name: '像素加载', type: 'line', smooth: true, symbolSize: 6, data: hours.map((hour) => pixelByHour.get(hour) || 0) },
      { name: '图片加载', type: 'line', smooth: true, symbolSize: 6, data: hours.map((hour) => imageByHour.get(hour) || 0) },
      { name: '链接点击', type: 'line', smooth: true, symbolSize: 6, data: hours.map((hour) => clickByHour.get(hour) || 0) },
    ],
  })
}

function renderImageChart() {
  if (!imageChartEl.value) return
  const chart = echarts.init(imageChartEl.value)
  const hours = stats.value.qr_trend?.map((item) => item.hour) || []
  const hourLabels = hours.map(formatLocalHour)
  const data = stats.value.qr_trend?.map((item) => item.count) || []
  chart.setOption({
    color: ['#3157a4'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: hourLabels,
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

async function renderLinkChart() {
  if (!linkChartEl.value || links.value.length === 0) return
  const chart = echarts.init(linkChartEl.value)
  const linkNames = links.value.map(l => l.label || '链接')
  const clickData = await Promise.all(links.value.map(async l => {
    try {
      const data = await api.linkStats(route.params.id, l.id)
      return data.summary?.total_clicks || 0
    } catch {
      return 0
    }
  }))
  chart.setOption({
    color: ['#a16207'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: linkNames,
      axisLine: { lineStyle: { color: '#d9dee8' } },
      axisLabel: { color: '#667085' },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#e8ecf2' } },
      axisLabel: { color: '#667085' },
    },
    series: [{
      name: '点击次数',
      type: 'bar',
      barWidth: '50%',
      data: clickData,
    }],
  })
}

function renderABChart() {
  if (!abChartEl.value || !abStats.value?.variants?.length) return
  const chart = echarts.init(abChartEl.value)
  const variantNames = abStats.value.variants.map(v => v.variant?.name || '变体')
  const openRates = abStats.value.variants.map(v => v.open_rate || 0)
  const clickRates = abStats.value.variants.map(v => v.click_rate || 0)
  chart.setOption({
    color: ['#087f8c', '#a16207'],
    grid: { left: 36, right: 18, top: 28, bottom: 36 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 8, textStyle: { color: '#667085' } },
    xAxis: {
      type: 'category',
      data: variantNames,
      axisLine: { lineStyle: { color: '#d9dee8' } },
      axisLabel: { color: '#667085' },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      splitLine: { lineStyle: { color: '#e8ecf2' } },
      axisLabel: { color: '#667085', formatter: '{value}%' },
    },
    series: [
      { name: '打开率', type: 'bar', barWidth: '30%', data: openRates },
      { name: '点击率', type: 'bar', barWidth: '30%', data: clickRates },
    ],
  })
}

async function send() {
  sending.value = true
  sendError.value = ''
  try {
    await api.sendCampaign(route.params.id)
    await load()
  } catch (err) {
    sendError.value = err.message
  } finally {
    sending.value = false
  }
}

function resetVariantForm() {
  editingVariantId.value = null
  variantError.value = ''
  variantPreviewError.value = ''
  variantBodyMode.value = 'preview'
  variantForm.name = ''
  variantForm.subject = campaign.value?.subject || ''
  variantForm.body_html = campaign.value?.body_html || ''
  variantForm.weight = 50
  renderVariantPreview()
}

function editVariant(item) {
  const variant = item.variant || item
  editingVariantId.value = variant.id
  variantError.value = ''
  variantForm.name = variant.name || ''
  variantForm.subject = variant.subject || ''
  variantForm.body_html = variant.body_html || ''
  variantForm.weight = variant.weight || 50
  variantBodyMode.value = 'preview'
  renderVariantPreview()
}

async function renderVariantPreview() {
  if (!variantForm.subject && !variantForm.body_html) {
    variantPreview.value = { subject: '', body_html: '' }
    return
  }
  variantPreviewError.value = ''
  try {
    variantPreview.value = await api.previewTemplate({
      subject: variantForm.subject,
      body_html: variantForm.body_html,
      contact: variantPreviewContact.value,
    })
  } catch (err) {
    variantPreviewError.value = err.message
  }
}

async function saveVariant() {
  variantError.value = ''
  const payload = {
    name: variantForm.name,
    subject: variantForm.subject,
    body_html: variantForm.body_html,
    weight: Number(variantForm.weight) || 1,
  }
  try {
    if (editingVariantId.value) {
      await api.updateVariant(route.params.id, editingVariantId.value, payload)
    } else {
      await api.createVariant(route.params.id, payload)
    }
    resetVariantForm()
    await load()
    activeTab.value = 'ab'
  } catch (err) {
    variantError.value = err.message
  }
}

async function removeVariant(item) {
  const variant = item.variant || item
  if (!(await askConfirm(`删除变体「${variant.name}」？`))) return
  await api.deleteVariant(route.params.id, variant.id)
  if (editingVariantId.value === variant.id) resetVariantForm()
  await load()
  activeTab.value = 'ab'
}

onMounted(load)
watch(activeTab, async (tab) => {
  await nextTick()
  if (tab === 'overview') {
    renderChart()
    renderRateChart()
    renderFunnelChart()
    renderEngagementTrend()
  }
  if (tab === 'ab' && abStats.value?.variants?.length > 0) renderABChart()
  if (tab === 'links' && links.value.length > 0) renderLinkChart()
  if (tab === 'qr') renderImageChart()
})
watch(
  () => [variantForm.subject, variantForm.body_html, recipients.value.length],
  () => {
    if (activeTab.value === 'ab' && variantBodyMode.value === 'preview') {
      renderVariantPreview()
    }
  },
)
</script>

<template>
  <section v-if="campaign">
    <div class="page-head">
      <div>
        <h1>{{ campaign.name }}</h1>
        <p class="muted">{{ campaign.subject }}</p>
      </div>
      <div class="toolbar campaign-actions">
        <a class="button secondary" :href="`/api/campaigns/${campaign.id}/export.csv`">导出 CSV</a>
        <button class="button" :disabled="sending" @click="send">
          {{ sending ? '发送中' : '开始/重试发送' }}
        </button>
      </div>
    </div>

    <p v-if="sendError" class="notice error">{{ sendError }}</p>

    <div class="tabs">
      <button :class="{ active: activeTab === 'overview' }" @click="activeTab = 'overview'">概览</button>
      <button :class="{ active: activeTab === 'recipients' }" @click="activeTab = 'recipients'">收件人</button>
      <button :class="{ active: activeTab === 'links' }" @click="activeTab = 'links'">链接追踪</button>
      <button :class="{ active: activeTab === 'ab' }" @click="activeTab = 'ab'">AB 测试</button>
      <button :class="{ active: activeTab === 'qr' }" @click="activeTab = 'qr'">图片埋点</button>
    </div>

    <!-- Overview Tab -->
    <template v-if="activeTab === 'overview'">
      <div class="overview-metrics">
        <div class="card metric">
          <strong>{{ overviewMetrics.total }}</strong>
          <span>收件人</span>
        </div>
        <div class="card metric">
          <strong>{{ overviewMetrics.sent }}</strong>
          <span>已发送</span>
        </div>
        <div class="card metric">
          <strong>{{ overviewMetrics.pending }}</strong>
          <span>就绪</span>
        </div>
        <div class="card metric">
          <strong>{{ overviewMetrics.waiting }}</strong>
          <span>队列中</span>
        </div>
        <div class="card metric">
          <strong>{{ formatPercent(overviewMetrics.imageLoadRate) }}</strong>
          <span>内容加载率</span>
        </div>
        <div class="card metric">
          <strong>{{ formatPercent(overviewMetrics.clickReturnRate) }}</strong>
          <span>点击回报率</span>
        </div>
        <div class="card metric">
          <strong>{{ overviewMetrics.totalClicks }}</strong>
          <span>总点击</span>
        </div>
      </div>

      <div class="grid two overview-charts" style="margin-top: 16px">
        <div class="panel">
          <h2>发送状态</h2>
          <div ref="chartEl" class="chart"></div>
        </div>
        <div class="panel">
          <h2>回报率对比</h2>
          <p class="muted">点击回报率按唯一点击 / 已发送计算。</p>
          <div ref="rateChartEl" class="chart"></div>
        </div>
        <div class="panel">
          <h2>响应漏斗</h2>
          <div ref="funnelChartEl" class="chart"></div>
        </div>
        <div class="panel">
          <h2>互动趋势</h2>
          <div ref="engagementTrendEl" class="chart"></div>
        </div>
      </div>
    </template>

    <!-- Recipients Tab -->
    <template v-if="activeTab === 'recipients'">
      <div class="panel">
        <div class="page-head">
          <div>
            <h2>收件人明细</h2>
            <p class="muted">图片加载是正文图片资源请求记录，比普通像素更适合判断邮件内容是否被加载。</p>
          </div>
          <select v-model="filter" style="max-width: 180px">
            <option value="all">全部</option>
            <option value="qr_loaded">图片已加载</option>
            <option value="qr_unloaded">图片未加载</option>
            <option value="opened">像素已加载</option>
            <option value="unopened">像素未加载</option>
            <option value="pending">就绪</option>
            <option value="waiting">队列中</option>
            <option value="failed">发送失败</option>
          </select>
        </div>
        <table>
          <thead>
            <tr>
              <th class="col-company">公司</th>
              <th>邮箱</th>
              <th>发送</th>
              <th>图片加载</th>
              <th>首次图片加载</th>
              <th>最近图片加载</th>
              <th>来源判断</th>
              <th>最近 IP</th>
              <th>IP类型</th>
              <th>预加载</th>
              <th>浏览器</th>
              <th>设备</th>
              <th>像素加载</th>
              <th>失败原因</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in visibleRecipients" :key="item.id">
              <td class="company-cell">{{ item.name }}</td>
              <td>
                <div class="chip-list">
                  <span v-for="(email, index) in splitValues(item.email)" :key="email" class="data-chip"
                    :class="`tone-${index % 5}`">
                    {{ email }}
                  </span>
                </div>
              </td>
              <td>
                <span class="status" :class="item.send_status">{{ sendStatusLabel(item.send_status) }}</span>
              </td>
              <td>{{ item.qr_load_count }}</td>
              <td>{{ formatDateTime(item.first_qr_load_at) }}</td>
              <td>{{ formatDateTime(item.last_qr_load_at) }}</td>
              <td>{{ formatOrigin(item) }}</td>
              <td>{{ item.last_qr_ip || '-' }}</td>
              <td :title="item.last_qr_ip_risk || (isIPPortraitLoading(item) ? '正在查询百度 IP 画像' : '')">
                <span v-if="isIPPortraitLoading(item)" class="ip-type-loading">
                  <span class="inline-spinner" aria-hidden="true"></span>
                  查询中
                </span>
                <span v-else>{{ formatIPType(item) }}</span>
              </td>
              <td>{{ item.qr_load_count > 0 ? formatPrefetch(item.last_qr_is_prefetch) : '-' }}</td>
              <td>{{ parseBrowser(item.last_qr_user_agent) }}</td>
              <td>{{ parseDevice(item.last_qr_user_agent) }}</td>
              <td>{{ formatLoadCount(item.open_count) }}</td>
              <td>{{ item.failure_reason }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="visibleRecipients.length === 0" class="empty">暂无符合条件的收件人</p>
      </div>
    </template>

    <!-- Links Tab -->
    <template v-if="activeTab === 'links'">
      <div class="panel">
        <h2>链接点击追踪</h2>
        <p class="muted">追踪邮件中各链接的点击情况</p>
        <div v-if="links.length === 0" class="empty">暂无追踪的链接</div>
        <div v-else ref="linkChartEl" class="chart"></div>
      </div>
    </template>

    <!-- AB Test Tab -->
    <template v-if="activeTab === 'ab'">
      <div class="panel">
        <div class="page-head">
          <div>
            <h2>AB 测试</h2>
            <p class="muted">创建多个主题和正文变体，发送时会按权重分配给收件人。</p>
          </div>
          <button type="button" class="secondary" @click="resetVariantForm">新建变体</button>
        </div>

        <form class="grid" style="margin-bottom: 16px" @submit.prevent="saveVariant">
          <div class="grid four">
            <label>变体名称<input v-model="variantForm.name" placeholder="A 版 / B 版" required /></label>
            <label>权重<input v-model.number="variantForm.weight" type="number" min="1" required /></label>
            <label style="grid-column: span 2">邮件主题<input v-model="variantForm.subject" required /></label>
          </div>
          <div class="field-block">
            <div class="field-row">
              <div>
                <div class="field-label">变体正文</div>
                <p class="muted">默认展示最终渲染效果；需要修改源码时切到 HTML。</p>
              </div>
              <div class="segmented">
                <button
                  type="button"
                  :class="{ active: variantBodyMode === 'preview' }"
                  @click="variantBodyMode = 'preview'; renderVariantPreview()"
                >
                  预览
                </button>
                <button type="button" :class="{ active: variantBodyMode === 'html' }" @click="variantBodyMode = 'html'">
                  HTML
                </button>
              </div>
            </div>
            <div v-if="variantBodyMode === 'preview'" class="mail-preview campaign-preview">
              <div class="mail-preview-subject">{{ variantPreview.subject || variantForm.subject || '邮件主题预览' }}</div>
              <iframe title="AB 变体正文预览" :srcdoc="variantPreview.body_html || variantForm.body_html"></iframe>
            </div>
            <label v-else class="html-editor">HTML 正文<textarea v-model="variantForm.body_html" required /></label>
            <p v-if="variantPreviewError" class="notice error">{{ variantPreviewError }}</p>
          </div>
          <p v-if="variantError" class="notice error">{{ variantError }}</p>
          <div class="toolbar">
            <button type="submit">{{ editingVariantId ? '更新变体' : '创建变体' }}</button>
            <button type="button" class="secondary" @click="resetVariantForm">重置</button>
          </div>
        </form>

        <div v-if="!abStats?.variants?.length" class="empty">暂无 AB 测试变体</div>
        <div v-else>
          <div ref="abChartEl" class="chart"></div>
          <table style="margin-top: 16px">
            <thead>
              <tr><th>变体</th><th>权重</th><th>主题</th><th>发送</th><th>打开</th><th>点击</th><th>打开率</th><th>点击率</th><th>操作</th></tr>
            </thead>
            <tbody>
              <tr v-for="v in abStats.variants" :key="v.variant?.id">
                <td>{{ v.variant?.name }}</td>
                <td>{{ v.variant?.weight }}</td>
                <td>{{ v.variant?.subject }}</td>
                <td>{{ v.stats?.sent }}</td>
                <td>{{ v.stats?.opened }}</td>
                <td>{{ v.stats?.clicked }}</td>
                <td>{{ v.open_rate?.toFixed(1) }}%</td>
                <td>{{ v.click_rate?.toFixed(1) }}%</td>
                <td>
                  <div class="table-actions">
                    <button type="button" class="secondary" @click="editVariant(v)">编辑</button>
                    <button type="button" class="secondary danger" @click="removeVariant(v)">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <!-- Image Tracking Tab -->
    <template v-if="activeTab === 'qr'">
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
        <h2>图片加载趋势</h2>
        <div ref="imageChartEl" class="chart"></div>
      </div>

      <div class="panel" style="margin-top: 16px">
        <div class="page-head">
          <div>
            <h2>图片埋点报告</h2>
            <p class="muted">埋点图片加载详情，包含远端同步来源、预加载判断、浏览器和设备信息。</p>
          </div>
          <select v-model="imageFilter" style="max-width: 160px">
            <option value="all">全部</option>
            <option value="loaded">已加载</option>
            <option value="unloaded">未加载</option>
            <option value="prefetch">疑似预加载</option>
          </select>
        </div>
        <table>
          <thead>
            <tr>
              <th class="col-company">公司</th>
              <th>邮箱</th>
              <th>发送</th>
              <th>像素加载</th>
              <th>图片加载</th>
              <th>首次加载</th>
              <th>最近加载</th>
              <th>来源判断</th>
              <th>IP</th>
              <th>IP类型</th>
              <th>预加载</th>
              <th>浏览器</th>
              <th>设备</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in imageRecipients" :key="item.id">
              <td class="company-cell">{{ item.name }}</td>
              <td>
                <div class="chip-list">
                  <span v-for="(email, index) in splitValues(item.email)" :key="email" class="data-chip"
                    :class="`tone-${index % 5}`">
                    {{ email }}
                  </span>
                </div>
              </td>
              <td><span class="status" :class="item.send_status">{{ sendStatusLabel(item.send_status) }}</span></td>
              <td>{{ formatLoadCount(item.open_count) }}</td>
              <td>{{ formatLoadCount(item.qr_load_count) }}</td>
              <td>{{ formatDateTime(item.first_qr_load_at) }}</td>
              <td>{{ formatDateTime(item.last_qr_load_at) }}</td>
              <td>{{ formatOrigin(item) }}</td>
              <td>{{ item.last_qr_ip || '-' }}</td>
              <td :title="item.last_qr_ip_risk || (isIPPortraitLoading(item) ? '正在查询百度 IP 画像' : '')">
                <span v-if="isIPPortraitLoading(item)" class="ip-type-loading">
                  <span class="inline-spinner" aria-hidden="true"></span>
                  查询中
                </span>
                <span v-else>{{ formatIPType(item) }}</span>
              </td>
              <td>{{ item.qr_load_count > 0 ? formatPrefetch(item.last_qr_is_prefetch) : '-' }}</td>
              <td>{{ parseBrowser(item.last_qr_user_agent) }}</td>
              <td>{{ parseDevice(item.last_qr_user_agent) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="imageRecipients.length === 0" class="empty">暂无符合条件的记录</p>
      </div>
    </template>
  </section>
</template>
