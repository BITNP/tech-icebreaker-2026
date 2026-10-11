// src/router.js
import { createRouter, createWebHashHistory } from 'vue-router'
import IndexPage from './pages/IndexPage.vue'
import Directors from './pages/Directors.vue'
import LotteryOrdered from './pages/LotteryOrdered.vue'
import LotteryRandom from './pages/LotteryRandom.vue'
import Lottery from './pages/Lottery.vue'
import CheckinPage from './pages/CheckinPage.vue'
import AdminPage from './pages/AdminPage.vue'
import { autoLoginOnce, hasToken } from './api'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/directors',
      name: 'directors',
      component: Directors,
      meta: { title: '部长介绍' }
    },
    {
      // 大屏：公开直连，谁打开都能看
      path: '/lottery',
      name: 'lottery',
      component: Lottery,
      meta: { title: '抽奖' }
    },
    {
      // 到场签到：公开，输名字即可
      path: '/checkin',
      name: 'checkin',
      component: CheckinPage,
      meta: { title: '签到' }
    },
    {
      // 管理台：只有 Keycloak 登录 + 白名单能进
      path: '/admin',
      name: 'admin',
      component: AdminPage,
      meta: { title: '管理' }
    },
    {
      path: '/lottery-random',
      name: 'lottery-random',
      component: LotteryRandom,
      meta: { title: '抽奖（随机）' }
    },
    {
      path: '/lottery-ordered',
      name: 'lottery-ordered',
      component: LotteryOrdered,
      meta: { title: '伪抽奖（固定顺序）' }
    },
    {
      path: '/',
      name: 'index-page',
      component: IndexPage,
      meta: { title: '首页' }
    }
  ]
})

const SITE_TITLE = '技术部破冰会'

// 全站强制登录：没有令牌就先送去 Keycloak，登录后回到原来那个页面。
// 只自动跳一次，避免"登录被拒 → 又跳登录"来回弹。
router.beforeEach((to) => {
  if (hasToken()) return true
  autoLoginOnce('/#' + to.fullPath)
  return true
})

// 标签页标题按页面区分，例如"技术部破冰会 - 抽奖"。
router.afterEach((to) => {
  document.title = to.meta.title ? SITE_TITLE + ' - ' + to.meta.title : SITE_TITLE
})

export default router
