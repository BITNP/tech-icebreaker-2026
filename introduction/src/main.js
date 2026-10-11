import { createApp } from 'vue'
import App from './App.vue'
import router from './router' // 引入路由配置
import { captureToken } from './api'

// 从 /?token=... 里收回登录结果（回跳地址里已经带着该去的 hash 路由）。
captureToken()

const app = createApp(App)
app.use(router)
app.mount('#app')
