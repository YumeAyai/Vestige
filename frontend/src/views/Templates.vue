<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../services/api'

const items = ref([])
const form = reactive({
  name: '市场调查邀请',
  subject: '请协助完成 {{.Company}} 调研问卷',
  body_html: '<p>{{.Name}} 您好：</p><p>我们正在开展一项市场调查，想邀请您协助填写问卷。</p><p>谢谢支持。</p>'
})

async function load() {
  items.value = await api.templates()
}

async function save() {
  await api.createTemplate(form)
  Object.assign(form, { name: '', subject: '', body_html: '' })
  await load()
}

onMounted(load)
</script>

<template>
  <section>
    <div class="page-head">
      <div>
        <h1>邮件模板</h1>
        <p class="muted" v-pre>支持变量：{{.Name}}、{{.Company}}、{{.Department}}、{{.Email}}。</p>
      </div>
    </div>
    <div class="grid two">
      <form class="panel grid" @submit.prevent="save">
        <label>模板名称<input v-model="form.name" required /></label>
        <label>邮件主题<input v-model="form.subject" required /></label>
        <label>HTML 正文<textarea v-model="form.body_html" required /></label>
        <button>保存模板</button>
      </form>
      <div class="panel">
        <h2>模板列表</h2>
        <table>
          <thead><tr><th>名称</th><th>主题</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td>{{ item.name }}</td>
              <td>{{ item.subject }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
