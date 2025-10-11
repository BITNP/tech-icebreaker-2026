<!-- 伪抽奖：固定排序，只随机抽取特效 -->
<template>
  <div class="background"></div>
  <RecruitAnimation ref="bangLottery" style="width: 100vh; height: 100vw;" :theme="theme"/>
  <InfoDisplay ref="infoDisplay" :name="(typeof name === 'string') ? name : ''" :avatar-url="(typeof avatarUrl === 'string') ? avatarUrl : ''" :shumeiniang-image="shumeiniangImage"/>
  <MultipleInfoDisplay ref="multiInfoDisplay" :name="Array.isArray(name) ? name : []" :avatar-url="Array.isArray(avatarUrl) ? avatarUrl : []" :shumeiniang-image="shumeiniangImage"/>

  <div class="ui-container" :hidden="!showUI">
    <div class="bank-title">抽奖名单</div>
    <div class="bank-container">
      <div class="bank-tool-bar">
        <button class="bank-tool-button" @click="uploadFile">导入文件</button>
        <button class="bank-tool-button" @click="downloadBank">导出名单</button>
        <button class="bank-tool-button" @click="clearBank">清空名单</button>
        <button class="bank-tool-button" @click="pick" style="background-color: pink;">一发入魂</button>
      </div>
      <div style="text-align: center; color: gray;">
        <input type="checkbox" v-model="autoRemove" id="autoRemoveCheckbox">
        <label for="autoRemoveCheckbox">自动将中奖者移出名单</label>
        <div style="margin-top: 10px;">
          当前名单有{{ bank.length }}人
          <label for="drawCount">抽取人数：</label>
          <input 
            type="number" 
            v-model.number="drawCount" 
            id="drawCount" 
            min="1" 
            style="width: 60px; margin-left: 10px; margin-right: 10px;"
          />
          <label for="showCountSelect">展示方式：</label>
          <select v-model.number="showCount" id="showCountSelect" style="width: 60px; margin-left: 10px; margin-right: 10px;">
            <option value="1">一抽</option>
            <option value="10">十连抽</option>
          </select>

          <label for="drawCount">随机种子：</label>
          <input 
            type="text" 
            v-model="randomSeed" 
            id="randomSeed" 
            style="width: 60px; margin-left: 10px; margin-right: 10px;"
          />
        </div>
      </div>
      <div class="bank-inner-container">
        <div v-if="bank.length === 0" style="width: 100%; text-align: center;">(空空如也~ 请点击上方按钮导入名单喵~)</div>
        <div v-for="people, i in bank" v-bind:key="i" class="bank-item">{{ people.name }}</div>
      </div>
    </div>
  </div>

  <div style="position: fixed; right: 20px; bottom: 20px; z-index: 999999999" v-show="!showUI && !playingAnim && (index === targets.length - 1)">
    <button @click="returnToUI">返回</button>
    <button @click="downloadResult">导出获奖名单</button>
  </div>
  
  
  <input 
    type="file" 
    ref="fileInput" 
    accept=".txt .json" 
    @change="handleFileUpload" 
    style="display: none"
  />
</template>

<script>
import RecruitAnimation from '@/components/RecruitAnimation.vue'; 
import InfoDisplay from '@/components/InfoDisplay.vue';
import MultipleInfoDisplay from '@/components/MultipleInfoDisplay.vue';
import seededRandomIndex from '@/functions/seededRandomIndex';

const AVAILABLE_SHUMEINIANG_IMAGES = [
  "ice-cream",
  "coffee",
  "cake",
  "bread",
  "air-plane",
];

const AVAILABLE_THEMES = [
  "BanGDream",
  "GenshinImpact",
  "PJSK",
  "WutheringWaves",
  "Arknights",
  "StarRail"
];



const getRandom = (array) => {
  return array[Math.floor(Math.random() * array.length)];
};

export default {
  name: 'App',
  components: {
    RecruitAnimation,
    InfoDisplay,
    MultipleInfoDisplay
  },
  data() {
    return {
      bank: [],
      targets: [],

      name: '', // 人员姓名
      theme: '',
      avatarUrl: '',
      shumeiniangImage: '',

      index: -1,
      playingAnim: false,
      fullScreen: false,

      autoRemove: true,
      drawCount: 1,
      showCount: 1,
      randomSeed: 'smn123',

      showUI: true
    }
  },
  methods: {
    uploadFile() {
      this.$refs.fileInput.click();
    },
    clearBank() {
      this.bank = [];
    },
    handleFileUpload(event) {
      const file = event.target.files[0];
      if (!file) return;

      const reader = new FileReader();

      reader.onload = (e) => {
        const fileContent = e.target.result;
        let people = [];
        try {
          people = JSON.parse(fileContent);
        } catch(e) {
          console.log(`Error occurred when trying to parse lottery info as json.`, e);

          people = fileContent.split('\n').map((name) => {return {name: name}});
        }

        this.bank = [];
        for (let person of people) {
          if (!person.shumeiniangImage) {
            person.shumeiniangImage = getRandom(AVAILABLE_SHUMEINIANG_IMAGES);
          }
          if (!person.theme) {
            person.theme = getRandom(AVAILABLE_THEMES);
          }
          this.bank.push(person);
        }
        
      }
      
      reader.onerror = () => {
        alert('文件读取失败');
      }
      
      reader.readAsText(file);
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
                console.warn("全屏失败:", err);
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

    displayLotteryAnim(person) {
      let name, theme, avatarUrl, shumeiniangImage;
      if (Array.isArray(person)) {
        name = person.map((p) => p.name);
        theme = person[0].theme;
        avatarUrl = person.map((p) => p.avatarUrl);
        shumeiniangImage = person[0].shumeiniangImage;
      } else {
        name = person.name;
        theme = person.theme;
        avatarUrl = person.avatarUrl;
        shumeiniangImage = person.shumeiniangImage
      }

      const self = this;
      self.showUI = false;
      self.playingAnim = true;
      self.$refs.infoDisplay.hide();
      self.$refs.multiInfoDisplay.hide();
      self.theme = theme;
      self.$nextTick(() => {
        self.name = name;
        self.avatarUrl = avatarUrl;
        self.shumeiniangImage = shumeiniangImage;
        
        self.$refs.bangLottery.play()
          .then(() => {
            if (Array.isArray(name)) {
              self.$refs.multiInfoDisplay.display();
            } else {
              self.$refs.infoDisplay.display();
            }
            self.playingAnim = false;
          });
        }
      );
    },

    skipAnimation() {
      this.$refs.bangLottery.reset();
      if (Array.isArray(this.name)) {
        this.$refs.multiInfoDisplay.display();
      } else {
        this.$refs.infoDisplay.display();
      }
      this.playingAnim = false;
    },

    pick() {
      if (this.bank.length === 0) {
        alert("名单为空！");
        return;
      } else if (this.bank.length < this.drawCount) {
        alert("名单人数少于抽取人数！");
        return;
      }

      const seed = "testSeed001"; // TODO: Set seed by user
      const indexArray = seededRandomIndex(this.bank, this.drawCount, seed);
      const draws = indexArray.map((i) => this.bank[i]);

      if (this.autoRemove) {
        // 按降序排序 indexArray，避免删除元素后索引变化的影响
        const sortedIndices = [...indexArray].sort((a, b) => b - a);
        for (const index of sortedIndices) {
          this.bank.splice(index, 1);
        }
      }

      if (this.showCount === 1) {
        this.targets = draws;
        this.index = -1;
      } else {
        const groupedDraws = [];
        for (let i = 0; i < draws.length; i += this.showCount) {
          groupedDraws.push(draws.slice(i, i + this.showCount));
        }
        this.targets = groupedDraws;
        this.index = -1;
      }

      // 发送一个模拟下箭头按键被按下的事件
      const downArrowEvent = new KeyboardEvent('keydown', {
        key: 'ArrowDown',
        code: 'ArrowDown',
        keyCode: 40,
        which: 40,
        bubbles: true,
        cancelable: true
      });
      document.dispatchEvent(downArrowEvent);
    },

    returnToUI() {
      this.$refs.bangLottery.reset();
      this.$refs.infoDisplay.hide();
      this.$refs.multiInfoDisplay.hide();
      this.showUI = true;
    },

    downloadResult() {
      // 下载抽奖结果
      const result = [];
      for (let target of this.targets) {
        if (Array.isArray(target)) {
          for (let person of target) {
            result.push(person.name);
          }
        } else {
          result.push(target.name);
        }
      }

      const text = result.join('\n');
      const blob = new Blob([text], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'lottery_result.txt';
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    },

    downloadBank() {
      // 下载抽奖结果
      const result = [];
      for (let person of this.bank) {
        result.push(person.name);
      }

      const text = result.join('\n');
      const blob = new Blob([text], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'bank.txt';
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }
  },
  mounted() {
    document.addEventListener('keydown', (e) => {
      if (e.key === " ") {
        this.skipAnimation();
      }
      if (e.key === "ArrowDown") {
        if (this.playingAnim) {
          this.skipAnimation();
        } else {
          if (this.index + 1 >= this.targets.length) return;
          this.index += 1;
          const person = this.targets[this.index];
          this.displayLotteryAnim(person);
        }
      } else if (e.key === "ArrowUp") {
        if (this.index - 1 < 0) return;
        this.index -= 1;
        const person = this.targets[this.index];
        this.displayLotteryAnim(person);
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

.ui-container {
  position: fixed;
  width: 100vw;
  height: 100vh;
  top: 0;
  left: 0;
  margin: 0;
}

.bank-container {
  padding-top: 10px;
  position: relative;
  margin-left: 10vw;
  width: 80vw;
  top: 10%;
  height: 90%;
  background: white;
  border-radius: 20px 20px 0 0;
}

.bank-title {
  margin-top: 10vh;
  width: 100%;
  text-align: center;
  color: white;
  font-size: 5rem;
}

.bank-tool-bar {
  position: relative;
  width: 80%;
  height: 5vh;
  text-align: center;
  display: flex;
  justify-content: center;
  margin-left: 10%;
  margin-top: 15px;
  margin-bottom: 5px;
  color: black;
  font-size: 2rem;
}

.bank-tool-button {
  width: 20%;
  margin-left: 2%;
  margin-right: 2%;
}

.bank-inner-container {
  position: relative;
  width: 100%;
  height: 55%;
  overflow-y: scroll;
}

.bank-item {
  position: relative;
  width: 80%;
  height: 10vh;
  text-align: center;
  margin-left: 10%;
  margin-top: 15px;
  margin-bottom: 5px;
  color: black;
  font-size: 2rem;
  border: 1px solid black;
  border-radius: 10px;
}

</style>
