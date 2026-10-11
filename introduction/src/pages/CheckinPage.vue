<!--
  现场签到：必须用组织账号登录。

  姓名不再由用户输入，而是取账号里的真名；一个账号只会有一条签到记录
  （服务端按 sub 唯一）。所以这里只有三步：跳登录 → 确认姓名 → 完成。
-->
<template>
  <div class="background"></div>
  <div class="wrap">
    <div class="card">
      <template v-if="phase === 'login'">
        <h1>破冰会签到</h1>
        <p class="hint">签到需要用组织账号登录，名字直接取账号里的真名。</p>
        <button class="submit" @click="goLogin">用组织账号登录</button>
        <p class="tips">正在跳转登录…没跳的话点上面的按钮。</p>
      </template>

      <template v-else-if="phase === 'confirm'">
        <h1>确认签到</h1>
        <p class="who">{{ me.name }}</p>
        <p class="hint">一个账号只能签一次，签完自己取消不了（要改找现场管理员）。</p>
        <button class="submit" :disabled="busy" @click="signup">
          {{ busy ? '提交中…' : '确认签到' }}
        </button>
      </template>

      <template v-else-if="phase === 'done'">
        <h1>已加入名单 🎉</h1>
        <p class="who">{{ result.name }}</p>
        <p class="done-line">你是第 {{ result.position }} 位，当前共 {{ result.count }} 人</p>
      </template>

      <template v-else-if="phase === 'already'">
        <h1>你已经签过了 ✅</h1>
        <p class="who">{{ result.name }}</p>
        <p class="done-line">你是第 {{ result.position }} 位，当前共 {{ result.count }} 人</p>
        <p class="tips">一个账号只算一次，重复签到不会在名单里多出一列。</p>
      </template>

      <p v-if="error" class="error">{{ error }}</p>
    </div>

    <router-link class="screen-link" to="/lottery">看大屏 →</router-link>
  </div>
</template>

<script>
import { api, autoLoginOnce, hasToken, login } from '@/api'

export default {
  name: 'CheckinPage',
  data() {
    return {
      phase: 'login', // login → confirm → done/already
      me: { sub: '', name: '', checkedIn: false },
      result: { name: '', position: 0, count: 0 },
      busy: false,
      error: ''
    }
  },
  methods: {
    goLogin() {
      login('/#/checkin')
    },

    // load 决定停在哪个状态：没令牌先去登录，签过就直接显示结果，否则等人点确认。
    async load() {
      if (!hasToken()) {
        this.phase = 'login'
        autoLoginOnce('/#/checkin')
        return
      }
      try {
        this.me = await api.me()
      } catch (e) {
        this.error = e.message
        this.phase = 'login'
        return
      }
      if (this.me.checkedIn) {
        await this.signup()
        return
      }
      this.phase = 'confirm'
    },

    async signup() {
      if (this.busy) return
      this.busy = true
      this.error = ''
      try {
        const res = await api.checkin()
        this.result = res
        this.phase = res.already ? 'already' : 'done'
      } catch (e) {
        this.error = e.message
        if (this.phase === 'login') this.phase = 'confirm'
      } finally {
        this.busy = false
      }
    }
  },
  mounted() {
    this.load()
  }
}
</script>

<style scoped>
.background {
  position: fixed;
  inset: 0;
  background-size: cover;
  background-position: center;
  background-image: url("@/assets/bg0.jpeg");
}

.wrap {
  position: relative;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1.25rem;
  padding: 1.5rem;
}

.card {
  width: 100%;
  max-width: 26rem;
  background: rgba(255, 255, 255, 0.95);
  border-radius: 1rem;
  padding: 2rem 1.5rem;
  text-align: center;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.25);
}

h1 {
  font-size: 1.5rem;
  margin: 0 0 0.75rem;
  color: #1c1c1e;
}

.hint {
  color: #555;
  margin: 0 0 1.25rem;
}

.who {
  font-size: 2rem;
  font-weight: bold;
  margin: 0.5rem 0 1rem;
  color: #1c1c1e;
}

.submit {
  width: 100%;
  font-size: 1.25rem;
  padding: 0.9rem;
  border: none;
  border-radius: 0.6rem;
  background: #2d5eff;
  color: #fff;
  cursor: pointer;
}

.submit:disabled {
  opacity: 0.5;
  cursor: default;
}

.done-line {
  color: #444;
  margin: 0 0 1rem;
}

.tips {
  color: #888;
  font-size: 0.85rem;
  margin: 0.9rem 0 0;
}

.error {
  margin: 1rem 0 0;
  color: #c0392b;
  font-size: 0.95rem;
}

.screen-link {
  color: #fff;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.6);
  text-decoration: none;
  font-size: 1rem;
}
</style>
