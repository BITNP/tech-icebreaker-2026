// src/router.js
// import { createRouter, createWebHistory } from 'vue-router'
import { createRouter, createWebHashHistory } from 'vue-router'
import IndexPage from './pages/IndexPage.vue'
import Directors from './pages/Directors.vue'
import LotteryOrdered from './pages/LotteryOrdered.vue'
import LotteryRandom from './pages/LotteryRandom.vue'
import Lottery from './pages/Lottery.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/directors',
      name: 'directors',
      component: Directors
    },
    {
      path: '/lottery',
      name: 'lottery',
      component: Lottery
    },
    {
      path: '/lottery-random',
      name: 'lottery-random',
      component: LotteryRandom
    },
    {
      path: '/lottery-ordered',
      name: 'lottery-ordered',
      component: LotteryOrdered
    },
    {
      path: '/',
      name: 'index-page',
      component: IndexPage
    }
  ]
})

export default router