const { defineConfig } = require('@vue/cli-service')
module.exports = defineConfig({
  transpileDependencies: true,
  // 前端与后端同源（生产由 Go 二进制托管 dist）；开发期用代理凑同一个源，
  // 免得 /auth/callback 与 Cookie/跨域上多出一堆特例。
  devServer: {
    port: 3000,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080' },
      '/auth': { target: 'http://127.0.0.1:8080' }
    }
  }
})
