import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Dashboard from './views/Dashboard.vue'
import Contacts from './views/Contacts.vue'
import Mailboxes from './views/Mailboxes.vue'
import Templates from './views/Templates.vue'
import Campaigns from './views/Campaigns.vue'
import CampaignDetail from './views/CampaignDetail.vue'
import './styles.css'

const routes = [
  { path: '/', component: Dashboard },
  { path: '/contacts', component: Contacts },
  { path: '/mailboxes', component: Mailboxes },
  { path: '/templates', component: Templates },
  { path: '/campaigns', component: Campaigns },
  { path: '/campaigns/:id', component: CampaignDetail }
]

createApp(App).use(createRouter({ history: createWebHistory(), routes })).mount('#app')
