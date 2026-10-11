<!-- 部长自我介绍 -->
<template>
  <div class="background"></div>
  <RecruitAnimation ref="bangLottery" style="width: 100vh; height: 100vw;" :theme="theme"/>
  <InfoDisplay ref="infoDisplay" :name="name" :avatar-url="avatarUrl" :shumeiniang-image="shumeiniangImage"/>

  <!-- 部长专属：填写/修改自己的介绍（admin 也有这个权限） -->
  <div v-if="canEdit" class="editor">
    <div class="editor-title">我的部长介绍</div>
    <label v-if="isAdmin">编辑谁的
      <select :value="target" @change="pick($event.target.value)">
        <option v-for="c in candidates" :key="c.sub" :value="c.sub">
          {{ c.name }}{{ c.hasIntro ? '（已有介绍）' : '' }}
        </option>
      </select>
    </label>
    <p class="editor-name">{{ targetName }}</p>
    <label>主题
      <select v-model="form.theme">
        <option v-for="t in THEMES" :key="t" :value="t">{{ t }}</option>
      </select>
    </label>
    <label>树莓娘
      <select v-model="form.shumeiniang">
        <option v-for="i in IMAGES" :key="i" :value="i">{{ i }}</option>
      </select>
    </label>
    <label>一句话
      <input v-model="form.note" type="text" maxlength="60" placeholder="比如：负责网协的服务器运维" />
    </label>
    <div class="editor-actions">
      <button :disabled="busy" @click="save">保存</button>
      <button :disabled="busy" @click="remove">{{ target === me.sub ? '删除我的' : '删除这条' }}</button>
      <span v-if="message" class="editor-msg">{{ message }}</span>
    </div>
  </div>
</template>

<script>
import RecruitAnimation from '@/components/RecruitAnimation.vue'; 
import InfoDisplay from '@/components/InfoDisplay.vue';
import { api, hasToken, login } from '@/api';

// 内置名单：服务端一条介绍都没有时用它（原来的十位部长）。
const DIRECTORS = [
  {name: "林子煊", avatarUrl: "/avatar/azusa.jpeg", theme: "GenshinImpact", shumeiniangImage: "guitar2"},
  {name: "张家诚", avatarUrl: "/avatar/mugi.jpeg", theme: "Arknights", shumeiniangImage: "keyboard"},
  {name: "袁靖", avatarUrl: "/avatar/mio.jpeg", theme: "WutheringWaves", shumeiniangImage: "bass"},
  {name: "杨浩天", avatarUrl: "/avatar/ritsu.jpeg", theme: "PJSK", shumeiniangImage: "drum"},
  {name: "郭逸戈", avatarUrl: "/avatar/yui.jpeg", theme: "BanGDream", shumeiniangImage: "guitar1"},
  {name: "江昆", avatarUrl: "", theme: "Wuhuamixin", shumeiniangImage: "ice-cream"},
  {name: "宋宇阳", avatarUrl: "", theme: "GenshinImpact", shumeiniangImage: "bread"},
  {name: "张一航", avatarUrl: "", theme: "Arknights", shumeiniangImage: "cake"},
  {name: "杨尚昕", avatarUrl: "", theme: "DeltaForce", shumeiniangImage: "air-plane"},
  {name: "陈超", avatarUrl: "", theme: "GenshinImpact", shumeiniangImage: "coffee"},
];

const THEMES = ["BanGDream", "GenshinImpact", "PJSK", "WutheringWaves", "Arknights", "StarRail", "Wuhuamixin", "DeltaForce"];
const IMAGES = ["ice-cream", "coffee", "cake", "bread", "air-plane", "bass", "drum", "keyboard", "guitar1", "guitar2"];

export default {
  name: 'App',
  components: {
    RecruitAnimation,
    InfoDisplay
  },
  data() {
    return {
      name: '', // 人员姓名
      theme: '',
      avatarUrl: '',
      index: -1,
      shumeiniangImage: '',
      playingAnim: false,
      fullScreen: false,

      people: DIRECTORS,   // 服务端有介绍就用服务端的，没有就退回内置名单
      me: { sub: '', name: '', role: '' },
      canEdit: false,
      isAdmin: false,
      candidates: [],      // admin 能编辑的对象：签到过的人 + 已有介绍的人
      target: '',          // 当前在编辑谁（非 admin 恒为自己）
      busy: false,
      message: '',
      form: { theme: 'GenshinImpact', shumeiniang: 'ice-cream', note: '' },
      THEMES,
      IMAGES
    }
  },
  computed: {
    targetName() {
      const hit = this.candidates.find((c) => c.sub === this.target)
      return hit ? hit.name : this.me.name
    }
  },
  methods: {
    // pick 换一个编辑对象，并把表单填成对方已有的介绍
    pick(sub) {
      this.target = sub
      const entry = this.people.find((p) => p.sub === sub)
      this.form.theme = entry ? entry.theme : this.form.theme
      this.form.shumeiniang = entry ? entry.shumeiniangImage : this.form.shumeiniang
      this.form.note = entry ? entry.note || '' : ''
      this.message = ''
    },

    async load() {
      // 名单：公开接口
      try {
        const res = await api.directors()
        const list = (res && res.directors) || []
        if (list.length) {
          this.people = list.map((d) => ({
            name: d.name,
            avatarUrl: '',
            theme: d.theme,
            shumeiniangImage: d.shumeiniang,
            sub: d.sub,
            note: d.note
          }))
        }
      } catch (e) {
        // 读不到就还用内置名单，介绍页不该因为后端抖动而空白
      }
      // 权限：登录了才有 me；部长能编辑自己那条，admin 能编辑所有人的
      if (!hasToken()) return
      try {
        this.me = await api.me()
        this.canEdit = !!(this.me.can && this.me.can.directors)
        this.isAdmin = this.me.role === 'admin'
      } catch (e) {
        // token 失效时静默重登，别在这儿打断介绍页
        login()
        return
      }
      if (!this.canEdit) return

      // admin 候选人 = 签到名单里的人 + 已有介绍的人；非 admin 只有自己
      let people = []
      if (this.isAdmin) {
        try {
          const state = await api.adminState()
          people = (state.participants || []).filter((p) => p.sub)
        } catch (e) {
          // 拉不到名单就只列已有介绍的人
        }
      }
      const map = new Map()
      for (const person of people) map.set(person.sub, { sub: person.sub, name: person.name, hasIntro: false })
      for (const entry of this.people.filter((p) => p.sub)) {
        const hit = map.get(entry.sub)
        map.set(entry.sub, { sub: entry.sub, name: (hit && hit.name) || entry.name, hasIntro: true })
      }
      if (!map.has(this.me.sub)) map.set(this.me.sub, { sub: this.me.sub, name: this.me.name, hasIntro: false })
      this.candidates = [...map.values()]

      // 默认编辑自己；已经在编别人的就别打断
      if (!this.target || !this.candidates.some((c) => c.sub === this.target)) this.target = this.me.sub
      const mine = this.people.find((p) => p.sub === this.target)
      if (mine) {
        this.form.theme = mine.theme || this.form.theme
        this.form.shumeiniang = mine.shumeiniangImage || this.form.shumeiniang
        this.form.note = mine.note || ''
      }
    },

    async save() {
      if (this.busy) return
      this.busy = true
      this.message = ''
      try {
        await api.saveDirector({ ...this.form, sub: this.target })
        this.message = '已保存'
        await this.load()
      } catch (e) {
        this.message = e.message
      } finally {
        this.busy = false
      }
    },

    async remove() {
      if (this.busy) return
      const mine = this.target === this.me.sub
      if (!window.confirm(mine ? '删除我的部长介绍？' : '删除「' + this.targetName + '」的部长介绍？')) return
      this.busy = true
      this.message = ''
      try {
        if (mine) {
          await api.deleteMyDirector()
        } else {
          await api.deleteDirector(this.target)
        }
        this.message = '已删除'
        await this.load()
      } catch (e) {
        this.message = e.message
      } finally {
        this.busy = false
      }
    },
    enterFullscreen() {
        const element = document.documentElement; // 整个页面全屏
        const requestMethod =
            element.requestFullscreen ||
            element.webkitRequestFullscreen ||
            element.mozRequestFullScreen ||
            element.msRequestFullscreen;

        if (requestMethod) {
            requestMethod.call(element).catch((err) => {
                console.error("全屏失败:", err);
                this.enableFullScreen = false; // 失败时重置状态
            });
        }
    },

    exitFullscreen() {
        const exitMethod =
            document.exitFullscreen ||
            document.webkitExitFullscreen ||
            document.mozCancelFullScreen ||
            document.msExitFullscreen;

        if (exitMethod) {
            exitMethod.call(document);
        }
    },
    displayLotteryAnim(name, avatarUrl, theme, shumeiniangImage) {
      const self = this;

      this.playingAnim = true;
      // console.log(this.$refs.infoDisplay);
      this.$refs.infoDisplay.hide();
      this.theme = theme;
      this.$nextTick(() => {
        this.name = name;
        this.avatarUrl = avatarUrl;
        this.shumeiniangImage = shumeiniangImage;
        
        this.$refs.bangLottery.play()
          .then(() => {
            self.$refs.infoDisplay.display();
            this.playingAnim = false;
          });
        }
      );
    },

    skipAnimation() {
      this.$refs.bangLottery.reset();
      this.$refs.infoDisplay.display();
      this.playingAnim = false;
    }
  },
  mounted() {
    this.load();

    document.addEventListener('keydown', (e) => {
      if (e.key === " ") {
        this.skipAnimation();
      }
      if (e.key === "ArrowDown") {
        if (this.playingAnim) {
          this.skipAnimation();
        } else {
          if (this.index + 1 >= this.people.length) return;
          this.index += 1;
          const director = this.people[this.index];
          this.displayLotteryAnim(director.name, director.avatarUrl, director.theme, director.shumeiniangImage);
        }
      } else if (e.key === "ArrowUp") {
        if (this.index - 1 < 0) return;
        this.index -= 1;
        const director = this.people[this.index];
        this.displayLotteryAnim(director.name, director.avatarUrl, director.theme, director.shumeiniangImage);
      }
    });

    document.addEventListener('keydown', (e) => {
      // 进入/退出全屏
      if (e.key === 'f' || e.key === 'F') {
        try {
          if (this.fullScreen) {
            this.exitFullscreen();
            this.fullScreen = false;
          } else {
            this.enterFullscreen();
            this.fullScreen = true;
          }
        } catch(e) {
          console.log("It seems you toggled full screen mode manually. Press key 'f' instead.");
        }
      }
    });
  }
}
</script>

<style>
.background {
  width: 100vw;
  height: 100vh;
  top: 0;
  left: 0;
  margin: 0;
  background-size: cover;
  background-image: url("@/assets/bg0.jpeg");
}

/* 部长专属的自我介绍编辑面板：平时不碍事，只有 minister/admin 看得到 */
.editor {
  position: fixed;
  right: 1.5rem;
  bottom: 1.5rem;
  width: 20rem;
  padding: 1rem 1.1rem;
  border-radius: 0.8rem;
  background: rgba(16, 17, 20, 0.92);
  border: 1px solid #2f333d;
  color: #e8e8ea;
  font-size: 0.9rem;
  z-index: 1000;
}

.editor-title {
  font-weight: 600;
  margin-bottom: 0.4rem;
}

.editor-name {
  margin: 0 0 0.6rem;
  color: #ffe066;
  font-size: 1.1rem;
}

.editor label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.45rem;
}

.editor select,
.editor input {
  flex: 1;
  min-width: 0;
  font: inherit;
  padding: 0.3rem 0.4rem;
  border-radius: 0.35rem;
  border: 1px solid #3a3f4a;
  background: #101114;
  color: inherit;
}

.editor-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

.editor button {
  font: inherit;
  padding: 0.35rem 0.8rem;
  border-radius: 0.35rem;
  border: 1px solid #3a3f4a;
  background: #23262e;
  color: inherit;
  cursor: pointer;
}

.editor button:disabled {
  opacity: 0.5;
  cursor: default;
}

.editor-msg {
  color: #a8e6b0;
}
</style>
