<!--
  管理台：唯一需要登录的地方。

  Keycloak 登录 + admin_subs 白名单都由服务端判定，这里的按钮只是入口；
  没有白名单的账号即使登录成功，任何管理接口都会吃 403。
-->
<template>
  <div class="admin">
    <header>
      <h1>icebreaker 管理台</h1>
      <div class="who">
        <span>{{ me.name || '未登录' }}</span>
        <span class="sub">{{ me.sub }}</span>
      </div>
    </header>

    <p v-if="error" class="msg error">{{ error }}</p>
    <p v-else-if="notice" class="msg notice">{{ notice }}</p>

    <p v-if="denied" class="msg notice">
      这个组织账号没有控制台权限（需要 operator 或 admin）。
      <router-link to="/checkin">去签到页 →</router-link>
    </p>

    <section class="card">
      <h2>现场状态</h2>
      <div class="stats">
        <div class="stat"><b>{{ state.participantCount }}</b><span>已签到</span></div>
        <div class="stat"><b>{{ state.eligibleCount }}</b><span>可抽奖</span></div>
        <div class="stat"><b>{{ state.current ? state.current.no : '—' }}</b><span>当前轮次</span></div>
        <div class="stat"><b>{{ state.signupOpen ? '开' : '关' }}</b><span>签到开关</span></div>
      </div>
      <div class="row">
        <button :disabled="busy || state.signupOpen" class="primary" @click="openSignup">开放签到</button>
        <button :disabled="busy || !state.signupOpen" class="danger" @click="closeSignup">关闭签到</button>
        <span class="hint">签到默认开着；说"开始签到"之前可以先关掉。</span>
      </div>
    </section>

    <section class="card">
      <h2>大屏控制</h2>
      <template v-if="state.current">
        <p class="line">
          第 {{ state.current.no }} 轮 · {{ state.current.label || '未命名' }} ·
          共 {{ state.current.winners.length }} 人 · 已揭晓 {{ Math.max(state.current.revealedIndex + 1, 0) }} 位
        </p>
        <div class="row">
          <button class="big" :disabled="busy || state.current.revealedIndex < 0" @click="step(-1)">⬆ 上一位</button>
          <button
            class="big primary"
            :disabled="busy || state.current.revealedIndex >= state.current.winners.length - 1"
            @click="step(1)"
          >⬇ 下一位</button>
          <button class="big" :disabled="busy" @click="skip">跳过动画</button>
          <button :disabled="busy" @click="endRound">结束本轮（回待机）</button>
        </div>
        <ol class="winners">
          <li v-for="(w, i) in state.current.winners" :key="i" :class="{ revealed: i <= state.current.revealedIndex }">
            {{ w.name }}
          </li>
        </ol>
        <p class="meta">
          种子 <code>{{ state.current.seed }}</code> · 快照
          <code>{{ state.current.snapshotHash.slice(0, 16) }}…</code>
          <span class="hint">（复核凭证，只在管理台显示）</span>
        </p>
      </template>
      <p v-else class="hint">当前没有轮次，大屏正显示待机画面（人数 + 签到二维码）。</p>
    </section>

    <section class="card">
      <h2>发起抽奖</h2>
      <div class="row">
        <label>抽取人数<input v-model.number="draw.count" type="number" min="1" /></label>
        <label>轮次标签<input v-model="draw.label" type="text" placeholder="三等奖（可空）" /></label>
        <label>随机种子<input v-model="draw.seed" type="text" placeholder="留空由服务端生成" /></label>
        <label class="check">
          <input v-model="draw.removeWinners" type="checkbox" /> 中奖者退出后续轮次
        </label>
        <button class="primary" :disabled="busy" @click="drawRound">发起抽奖</button>
      </div>
      <p class="hint">从"可抽奖"的人里抽；同一种子 + 同一名单必然得到同一份结果，可事后复核。</p>
    </section>

    <section class="card">
      <h2>名单（{{ state.participants.length }} 人）</h2>
      <div class="row">
        <input v-model="newName" type="text" maxlength="20" placeholder="手动加一个人" @keyup.enter="addOne" />
        <button :disabled="busy || !newName.trim()" @click="addOne">加入名单</button>
        <input ref="file" type="file" accept=".txt,.json" style="display: none" @change="importFile" />
        <button :disabled="busy" @click="$refs.file.click()">导入文件…</button>
        <label class="check"><input v-model="replaceOnImport" type="checkbox" /> 覆盖（清空现有名单与轮次）</label>
        <button :disabled="busy" @click="exportFile('participants')">导出名单</button>
        <button :disabled="busy" @click="exportFile('results')">导出结果</button>
      </div>
      <div class="people">
        <span v-if="!state.participants.length" class="hint">还没人签到</span>
        <span v-for="p in state.participants" :key="p.id" class="person">
          {{ p.name }}
          <button class="x" title="移出名单" :disabled="busy" @click="remove(p)">×</button>
        </span>
      </div>
    </section>

    <section class="card">
      <h2>重置</h2>
      <div class="row">
        <button class="danger" :disabled="busy" @click="resetWinners">重置中奖（保留名单）</button>
        <button class="danger" :disabled="busy" @click="resetAll">清空整场（连名单一起）</button>
      </div>
      <p class="hint">两个都是硬删、不可撤销，点之前会再问一次。</p>
    </section>

    <section v-if="state.rounds.length" class="card">
      <h2>历史轮次</h2>
      <div v-for="r in historyRows" :key="r.no" class="history">
        <b>第 {{ r.no }} 轮</b>
        <span class="hint">{{ r.label || '未命名' }} · {{ r.removeWinners ? '中奖者已出局' : '中奖者仍在池中' }} ·</span>
        {{ r.winners.map((w) => w.name).join('、') }}
      </div>
    </section>

    <section v-if="can.roles" class="card">
      <h2>角色管理（只有 admin 看得到这一块）</h2>
      <p class="hint">
        登录后默认就是 user。配置文件里的根管理员在这里改不动：
        <code v-if="configAdmins.length">{{ configAdmins.join('、') }}</code>
        <span v-else>（config.yaml 的 auth.admin_subs 是空的，只靠下面的分配）</span>
      </p>
      <div class="role-list">
        <div v-for="p in withSub" :key="p.id" class="role-row">
          <span class="role-name">{{ p.name }}</span>
          <span v-if="isRootAdmin(p.sub)" class="hint">根管理员（配置里写的）</span>
          <span v-else-if="p.source !== 'checkin'" class="hint">手动添加</span>
          <select
            :value="p.role || 'user'"
            :disabled="busy || isRootAdmin(p.sub)"
            @change="assign(p, $event.target.value)"
          >
            <option value="user">user · 只能签到</option>
            <option value="minister">minister · 写部长介绍</option>
            <option value="operator">operator · 抽奖与大屏</option>
            <option value="admin">admin · 全部权限</option>
          </select>
        </div>
        <p v-if="!withSub.length" class="hint">还没有人签到。等大家签完，这里就能直接点角色。</p>
      </div>
    </section>

    <footer>
      大屏 <a :href="screenURL" target="_blank" rel="noopener">{{ screenURL }}</a> ·
      签到 <a :href="checkinURL" target="_blank" rel="noopener">{{ checkinURL }}</a>
    </footer>
  </div>
</template>

<script>
import { api, downloadExport, hasToken } from '@/api'

const POLL_MS = 3000

export default {
  name: 'AdminPage',
  data() {
    return {
      state: {
        signupOpen: true,
        participantCount: 0,
        eligibleCount: 0,
        participants: [],
        current: null,
        rounds: []
      },
      me: { sub: '', name: '' },
      myRole: '',
      can: { checkin: false, directors: false, lottery: false, roles: false },
      configAdmins: [],
      busy: false,
      error: '',
      notice: '',
      denied: false,
      newName: '',
      replaceOnImport: false,
      draw: { count: 1, label: '', seed: '', removeWinners: true },
      timer: null,
      checkinURL: window.location.origin + '/#/checkin',
      screenURL: window.location.origin + '/#/lottery'
    }
  },
  computed: {
    historyRows() {
      return this.state.rounds.slice().reverse()
    },
    withSub() {
      return this.state.participants.filter((p) => p.sub)
    }
  },
  methods: {
    isRootAdmin(sub) {
      return this.configAdmins.includes(sub)
    },

    // refreshAccess 先问清楚"我是谁、能不能用控制台"，再决定渲染与请求。
    async refreshAccess() {
      if (!hasToken()) return
      try {
        const me = await api.me()
        this.me = { sub: me.sub, name: me.name }
        this.myRole = me.role
        this.can = me.can || this.can
        this.denied = false
        if (this.can.roles) await this.loadRoles()
      } catch (e) {
        this.error = e.message
      }
    },

    async loadRoles() {
      try {
        const res = await api.roles()
        this.configAdmins = res.configAdmins || []
      } catch (e) {
        this.error = e.message
      }
    },

    assign(person, role) {
      this.run(async () => {
        await api.setRole(person.sub, role, person.name)
        return '已把 ' + person.name + ' 设为 ' + role
      })
    },

    async refresh() {
      if (!hasToken()) return
      try {
        const state = await api.adminState()
        this.state = state
        this.me = { sub: state.sub, name: state.name }
        this.denied = false
      } catch (e) {
        this.error = e.message
        this.denied = e.code === 'no_permission'
      }
    },

    // run 统一处理"跑一个动作 → 刷新状态 → 显示结果/错误"
    async run(action, successMessage) {
      if (this.busy) return
      this.busy = true
      this.error = ''
      this.notice = ''
      try {
        const result = await action()
        await this.refresh()
        // 动作可以直接返回一条消息（比如抽奖要带上轮次号和种子）
        this.notice = successMessage || (typeof result === 'string' ? result : '')
      } catch (e) {
        this.error = e.message
      } finally {
        this.busy = false
      }
    },

    openSignup() {
      this.run(() => api.openSignup(), '签到已开放')
    },
    closeSignup() {
      this.run(() => api.closeSignup(), '签到已关闭')
    },

    addOne() {
      const name = this.newName.trim()
      if (!name) return
      this.run(async () => {
        await api.addParticipant(name)
        this.newName = ''
      }, '已加入名单：' + name)
    },

    remove(person) {
      if (!window.confirm('把「' + person.name + '」移出名单？')) return
      this.run(() => api.deleteParticipant(person.id), '已移出：' + person.name)
    },

    drawRound() {
      const payload = {
        count: this.draw.count,
        label: this.draw.label,
        removeWinners: this.draw.removeWinners,
        seed: this.draw.seed.trim()
      }
      this.run(async () => {
        const res = await api.draw(payload)
        this.draw.seed = ''
        return '第 ' + res.no + ' 轮已抽出，种子 ' + res.seed
      })
    },

    step(delta) {
      const current = this.state.current
      if (!current) return
      this.run(() => api.cursor(current.revealedIndex + delta), '')
    },

    skip() {
      this.run(() => api.skip(), '')
    },

    endRound() {
      this.run(() => api.endRound(), '本轮结束，大屏回到待机')
    },

    resetWinners() {
      if (!window.confirm('重置中奖：保留签到名单，删掉所有轮次与中奖记录。继续？')) return
      this.run(() => api.reset('winners'), '已重置中奖记录，回到"全员未中奖"')
    },

    resetAll() {
      if (!window.confirm('清空整场：连签到名单一起删掉。继续？')) return
      this.run(() => api.reset('all'), '已清空整场')
    },

    async exportFile(what) {
      try {
        await downloadExport(what)
      } catch (e) {
        this.error = e.message
      }
    },

    parseNames(text) {
      try {
        const data = JSON.parse(text)
        if (Array.isArray(data)) {
          return data
            .map((item) => (typeof item === 'string' ? item : (item && item.name) || ''))
            .map((name) => name.trim())
            .filter(Boolean)
        }
      } catch (e) {
        // 不是 JSON，按每行一个名字处理
      }
      return text
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
    },

    importFile(event) {
      const file = event.target.files[0]
      if (!file) return
      const mode = this.replaceOnImport ? 'replace' : 'append'
      const reader = new FileReader()
      reader.onload = (e) => {
        const names = this.parseNames(String(e.target.result))
        this.run(
          () => api.importNames(names, mode),
          mode === 'replace'
            ? '已用文件覆盖名单（' + names.length + ' 个名字）'
            : '已从文件追加 ' + names.length + ' 个名字'
        )
      }
      reader.onerror = () => {
        this.error = '文件读取失败'
      }
      reader.readAsText(file)
      event.target.value = ''
    }
  },
  mounted() {
    this.refreshAccess()
    this.refresh()
    this.timer = window.setInterval(this.refresh, POLL_MS)
  },
  unmounted() {
    if (this.timer) window.clearInterval(this.timer)
  }
}
</script>

<style scoped>
.admin {
  min-height: 100vh;
  padding: 1.5rem clamp(1rem, 4vw, 4rem) 3rem;
  background: #101114;
  color: #e8e8ea;
  font-size: 15px;
  line-height: 1.6;
}

header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

h1 {
  font-size: 1.35rem;
  margin: 0;
  letter-spacing: 0.04em;
}

.who {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 0.85rem;
}

.sub {
  color: #7d818c;
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
}

.card {
  background: #191b20;
  border: 1px solid #2a2d35;
  border-radius: 12px;
  padding: 1rem 1.1rem;
  margin-bottom: 1rem;
}

h2 {
  font-size: 1rem;
  margin: 0 0 0.75rem;
  color: #b9bec9;
  font-weight: 600;
}

.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.6rem;
}

button {
  font: inherit;
  padding: 0.5rem 1rem;
  border-radius: 8px;
  border: 1px solid #3a3f4a;
  background: #23262e;
  color: inherit;
  cursor: pointer;
}

button:hover:enabled {
  background: #2c3039;
}

button:disabled {
  opacity: 0.4;
  cursor: default;
}

button.primary {
  background: #2d5eff;
  border-color: #2d5eff;
  font-weight: 600;
}

button.danger {
  background: #6d3030;
  border-color: #6d3030;
}

button.big {
  font-size: 1.05rem;
  padding: 0.7rem 1.4rem;
}

input[type='text'],
input[type='number'] {
  font: inherit;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  border: 1px solid #3a3f4a;
  background: #101114;
  color: inherit;
  width: 8rem;
}

label {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  color: #b9bec9;
  font-size: 0.9rem;
}

label.check {
  gap: 0.3rem;
}

.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 2rem;
  margin-bottom: 0.9rem;
}

.stat b {
  display: block;
  font-size: 1.7rem;
  font-variant-numeric: tabular-nums;
}

.stat span {
  color: #7d818c;
  font-size: 0.8rem;
}

.hint {
  color: #7d818c;
  font-size: 0.85rem;
}

.line {
  margin: 0 0 0.75rem;
}

.meta code {
  background: #101114;
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  font-size: 0.85rem;
}

.msg {
  margin: 0 0 1rem;
  padding: 0.6rem 0.9rem;
  border-radius: 8px;
}

.error {
  background: #3a1f1f;
  color: #ffb4b4;
}

.notice {
  background: #1e3222;
  color: #a8e6b0;
}

.winners {
  margin: 0.9rem 0 0.5rem;
  padding-left: 1.4rem;
  color: #7d818c;
}

.winners li.revealed {
  color: #ffe066;
  font-weight: 600;
}

.people {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.9rem;
}

.person {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem 0.25rem 0.7rem;
  border-radius: 999px;
  background: #23262e;
  border: 1px solid #2f333d;
}

.person .x {
  padding: 0 0.35rem;
  border: none;
  background: transparent;
  color: #8c9099;
  font-size: 1rem;
  line-height: 1;
}

.person .x:hover:enabled {
  color: #ff8b8b;
  background: transparent;
}

.history {
  padding: 0.35rem 0;
  border-bottom: 1px dashed #2a2d35;
}

.role-list {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  max-height: 16rem;
  overflow-y: auto;
}

.role-row {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

.role-name {
  min-width: 6rem;
}

.role-row select {
  font: inherit;
  padding: 0.35rem 0.5rem;
  border-radius: 0.4rem;
  border: 1px solid #3a3f4a;
  background: #101114;
  color: inherit;
}

footer {
  color: #7d818c;
  font-size: 0.85rem;
}

footer a {
  color: #8ab4ff;
}
</style>
