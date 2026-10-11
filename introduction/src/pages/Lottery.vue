<!--
  大屏：公开直连，只负责显示。

  推进由管理台控制 —— 这里 1.5 秒轮询一次状态，游标变了就播下一位、
  跳过信号变了就跳过当前动画。名单在开轮时一次性预载，动画本身不依赖网络。
-->
<template>
  <div class="background"></div>
  <RecruitAnimation ref="bangLottery" style="width: 100vh; height: 100vw;" :theme="theme" />
  <InfoDisplay ref="infoDisplay" :name="name" avatar-url="" :shumeiniang-image="shumeiniangImage" />

  <div v-if="!connected" class="banner">与服务器连接中断，正在重试…（若一直这样，刷新本页会重新登录）</div>
  <div v-else-if="needsGesture" class="banner">
    浏览器的自动播放被拦了：点一下屏幕或按任意键解锁，会自动重播这一位
  </div>

  <!-- 待机：人数 + 签到二维码 -->
  <div v-if="!round" class="idle">
    <h1 class="idle-title">破冰会抽奖</h1>
    <p class="idle-count">已签到 <b>{{ count }}</b> 人</p>
    <button class="fullscreen-btn" @click="enterFullscreen">点击进入全屏演示</button>
    <img class="qr" :src="qrSrc" alt="签到二维码" />
    <p class="idle-hint">扫码签到 · {{ checkinURL }}</p>
    <p class="idle-hint">{{ signupOpen ? '签到开放中' : '签到已关闭' }}</p>
  </div>

  <!-- 抽奖中：左上角轮次信息 -->
  <div v-else class="hud">
    <div class="hud-label">{{ round.label || ('第 ' + round.no + ' 轮') }}</div>
    <div class="hud-progress">{{ Math.max(round.revealedIndex + 1, 0) }} / {{ round.count }}</div>
  </div>
</template>

<script>
import RecruitAnimation from '@/components/RecruitAnimation.vue'
import InfoDisplay from '@/components/InfoDisplay.vue'
import { api } from '@/api'

const POLL_MS = 1500

export default {
  name: 'LotteryScreen',
  components: {
    RecruitAnimation,
    InfoDisplay
  },
  data() {
    return {
      connected: true,
      needsGesture: false,
      signupOpen: false,
      count: 0,
      round: null,
      entries: [],
      theme: '',
      name: '',
      shumeiniangImage: '',
      playing: false,
      prevIndex: -1,
      prevSkip: 0,
      loadedRound: 0,
      stalledIndex: null,
      watchdog: null,
      timer: null,
      checkinURL: window.location.origin + '/#/checkin'
    }
  },
  computed: {
    qrSrc() {
      return api.qr(this.checkinURL)
    }
  },
  methods: {
    async poll() {
      let state
      try {
        state = await api.screenState()
        this.connected = true
      } catch (e) {
        this.connected = false
        return
      }

      this.signupOpen = state.signupOpen
      this.count = state.participantCount

      if (!state.round) {
        if (this.round) this.resetStage()
        this.round = null
        this.entries = []
        this.loadedRound = 0
        return
      }

      if (state.round.no !== this.loadedRound) {
        await this.loadRound()
        if (!this.round) return
      }
      this.round = state.round
      this.follow(state.round)
    },

    async loadRound() {
      let round
      try {
        round = await api.screenRound()
        this.connected = true
      } catch (e) {
        this.connected = false
        return
      }
      this.entries = round.entries || []
      this.loadedRound = round.no
      this.round = round
      // 就位不等于推进：先把画面清干净，再按当前游标直接摆好，不重放动画。
      // 大屏中途刷新时靠这一步"无声"恢复到已经揭晓的那一位。
      this.prevSkip = round.skipSeq
      this.resetStage()
      this.prevIndex = round.revealedIndex
      if (round.revealedIndex >= 0) this.showIndex(round.revealedIndex, false)
    },

    follow(round) {
      if (round.revealedIndex !== this.prevIndex) {
        // 往前推进才播动画；回退时直接显示
        const animate = round.revealedIndex > this.prevIndex
        this.prevIndex = round.revealedIndex
        this.showIndex(round.revealedIndex, animate)
      }
      if (round.skipSeq !== this.prevSkip) {
        this.prevSkip = round.skipSeq
        if (this.playing) this.skipAnimation()
      }
    },

    showIndex(index, animate) {
      const person = this.entries[index]
      if (!person) {
        this.resetStage()
        return
      }

      const self = this
      if (!animate) {
        this.clearWatchdog()
        this.stalledIndex = null
        this.$refs.bangLottery.reset()
        this.playing = false
        this.theme = person.theme
        this.name = person.name
        this.shumeiniangImage = person.shumeiniang
        this.$nextTick(() => {
          self.$refs.infoDisplay.display()
        })
        return
      }

      this.playing = true
      this.$refs.infoDisplay.hide()
      this.theme = person.theme
      this.$nextTick(() => {
        self.name = person.name
        self.shumeiniangImage = person.shumeiniang
        // RecruitAnimation.play() 只在视频 "ended" 时 resolve，而且它把
        // 内部 play() 的拒绝自己吞了 —— 浏览器拦下自动播放时，外层既不会
        // resolve 也不会 reject，整场就卡在黑屏上。所以自己上闹钟盯着。
        self.clearWatchdog()
        self.watchdog = window.setTimeout(() => {
          const video = self.$refs.bangLottery.$refs.videoPlayer
          if (video && video.paused) {
            self.stalledIndex = index
            self.needsGesture = true
            self.playing = false
            self.$refs.bangLottery.reset()
            self.$refs.infoDisplay.display()
          }
        }, 1500)
        self.$refs.bangLottery
          .play()
          .then(() => {
            self.clearWatchdog()
            self.$refs.infoDisplay.display()
            self.playing = false
          })
          .catch(() => {
            self.clearWatchdog()
            self.stalledIndex = index
            self.needsGesture = true
            self.playing = false
            self.$refs.infoDisplay.display()
          })
      })
    },

    clearWatchdog() {
      if (this.watchdog) {
        window.clearTimeout(this.watchdog)
        this.watchdog = null
      }
    },

    // 第一次交互就解锁自动播放（Chrome 规则：文档上有过一次用户手势之后
    // 才允许带声音的自动播放）。被拦下的那一位顺手重播，现场不用回退再前进。
    onFirstGesture() {
      this.needsGesture = false
      const stalled = this.stalledIndex
      this.stalledIndex = null
      if (stalled !== null && stalled >= 0 && stalled === this.prevIndex) {
        this.showIndex(stalled, true)
      }
    },

    onFullscreenKey(e) {
      if (e.key === 'f' || e.key === 'F') this.enterFullscreen()
    },

    skipAnimation() {
      this.clearWatchdog()
      this.stalledIndex = null
      this.$refs.bangLottery.reset()
      this.$refs.infoDisplay.display()
      this.playing = false
    },

    resetStage() {
      this.clearWatchdog()
      this.$refs.bangLottery.reset()
      this.$refs.infoDisplay.hide()
      this.playing = false
      this.theme = ''
      this.name = ''
      this.shumeiniangImage = ''
    },

    enterFullscreen() {
      const element = document.documentElement
      const request =
        element.requestFullscreen ||
        element.webkitRequestFullscreen ||
        element.mozRequestFullScreen ||
        element.msRequestFullscreen
      if (request) {
        request.call(element).catch(() => {
          // 手动按 F 也能进全屏
        })
      }
      // 这次点击同时解锁后续的自动播放
      this.needsGesture = false
    }
  },
  mounted() {
    this.poll()
    this.timer = window.setInterval(this.poll, POLL_MS)

    document.addEventListener('pointerdown', this.onFirstGesture)
    document.addEventListener('keydown', this.onFirstGesture)
    document.addEventListener('keydown', this.onFullscreenKey)
  },
  unmounted() {
    if (this.timer) window.clearInterval(this.timer)
    this.clearWatchdog()
    document.removeEventListener('pointerdown', this.onFirstGesture)
    document.removeEventListener('keydown', this.onFirstGesture)
    document.removeEventListener('keydown', this.onFullscreenKey)
  }
}
</script>

<style>
.background {
  position: fixed;
  inset: 0;
  margin: 0;
  background-size: cover;
  background-image: url("@/assets/bg0.jpeg");
}

.idle {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1.2rem;
  color: #fff;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.6);
}

.idle-title {
  font-size: 3.5rem;
  margin: 0;
  letter-spacing: 0.08em;
}

.idle-count {
  font-size: 2rem;
  margin: 0;
}

.idle-count b {
  font-size: 3rem;
  color: #ffe066;
}

.qr {
  width: 18rem;
  height: 18rem;
  background: #fff;
  padding: 0.75rem;
  border-radius: 0.75rem;
}

.idle-hint {
  margin: 0;
  font-size: 1.1rem;
  opacity: 0.9;
}

.fullscreen-btn {
  font-size: 1.1rem;
  padding: 0.6rem 1.4rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.6);
  background: rgba(0, 0, 0, 0.35);
  color: #fff;
  cursor: pointer;
}

.hud {
  position: fixed;
  top: 1.5rem;
  left: 2rem;
  color: #fff;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.7);
  font-size: 2rem;
}

.hud-progress {
  font-size: 1.25rem;
  opacity: 0.85;
}

.banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  padding: 0.5rem;
  text-align: center;
  background: #c0392b;
  color: #fff;
  font-size: 1rem;
  z-index: 10;
}
</style>
