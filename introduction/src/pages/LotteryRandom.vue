<!-- 抽奖 -->
<template>
  <div class="background"></div>
  <RecruitAnimation ref="bangLottery" style="width: 100vh; height: 100vw;" :theme="theme"/>
  <InfoDisplay ref="infoDisplay" :name="name" :avatar-url="avatarUrl" :shumeiniang-image="shumeiniangImage"/>
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

let bank = [];

const getRandom = (array) => {
  return array[Math.floor(Math.random() * array.length)];
};

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
    }
  },
  methods: {
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

        bank = [];
        for (let person of people) {
          if (!person.shumeiniangImage) {
            person.shumeiniangImage = getRandom(AVAILABLE_SHUMEINIANG_IMAGES);
          }
          if (!person.theme) {
            person.theme = getRandom(AVAILABLE_THEMES);
          }
          bank.push(person);
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
    document.addEventListener('keydown', (e) => {
      if (e.key === " ") {
        this.skipAnimation();
      }
      if (e.key === "ArrowDown") {
        if (this.playingAnim) {
          this.skipAnimation();
        } else {
          const director = getRandom(bank);
          this.displayLotteryAnim(director.name, director.avatarUrl, getRandom(AVAILABLE_THEMES), getRandom(AVAILABLE_SHUMEINIANG_IMAGES));
        }
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

    document.addEventListener('keydown', (e) => {
      if (e.key === 'u' || e.key === 'U') {
        this.$refs.fileInput.click();
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
</style>
