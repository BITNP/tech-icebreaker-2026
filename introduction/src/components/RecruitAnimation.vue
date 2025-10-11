<!-- BanG Dream 招募动画 -->
<template>
  <div ref="background" class="background"></div>
  <div ref="whiteCover" class="white-cover" hidden></div>

  <video ref="videoPlayer"
    class="video-player"
    hidden
  >
    <source ref="videoSource" src="" type="video/mp4">
    您的浏览器不支持HTML5视频播放。
  </video>

  <audio ref="audioPlayer" class="audio-player">
    <source ref="audioSource" src="" type="audio/wav">
    您的浏览器不支持HTML5音频播放。
  </audio>
</template>

<script>

const THEMES = {
  "BanGDream": {video: "/media/BanG_Dream_Recruit.mp4", audio: "/media/BanG_Dream_Recruit.wav"},
  "GenshinImpact": {video: "/media/Genshin.mp4", audio: ""},
  "PJSK": {video: "/media/PJSK.mp4", audio: ""},
  "WutheringWaves": {video: "/media/WutheringWaves.mp4", audio: "/media/WutheringWaves.wav"},
  "Arknights": {video: "/media/Arknights.mp4", audio: ""},
  "Wuhuamixin": {video: "/media/wuhuamixin.mp4", audio: ""},
  "StarRail": {video: "/media/StarRail.mp4", audio: "/media/StarRail.wav"},
  "DeltaForce": {video: "/media/DeltaForce.mp4", audio: ""},
}

export default {
  name: 'RecruitAnimation',
  components: {
    // ...
  },
  props: {
    theme: {
      type: String,
      default: ''
    }
  },
  data() {
    return {
      videoUrl: "",
      audioUrl: ""
    }
  },
  watch: {
    theme: {
      handler(newVal) {
        if (THEMES[newVal]) {
          this.videoUrl = THEMES[newVal].video;
          this.audioUrl = THEMES[newVal].audio;

          if (this.$refs.videoSource) {
            this.$refs.videoSource.src = THEMES[newVal].video;
          }

          if (this.$refs.audioSource) {
            this.$refs.audioSource.src = THEMES[newVal].audio;
          }
          this.reset();
        }
      },
      immediate: true
    },
  },
  methods: {
    /**
     * 重置为初始状态
     */
    reset() {
      if (this.$refs.whiteCover) {
        this.$refs.whiteCover.hidden = true;
        this.$refs.whiteCover.style.opacity = 0;
      }

      if (this.$refs.videoPlayer) {
        this.$refs.videoPlayer.hidden = true;
        this.$refs.videoPlayer.pause();
        this.$refs.videoPlayer.style.opacity = 0;
        this.$refs.videoPlayer.currentTime = 0;
      }
      if (this.$refs.audioPlayer) {
        this.$refs.audioPlayer.pause();
        this.$refs.audioPlayer.currentTime = 0;
      }
    },

    /**
     * 播放视频和音频
     * 此方法会尝试播放视频和音频元素，并在播放失败时打印错误信息
     * 播放成功后会移除点击事件监听器
     */
    play() {
      // 将视频和音频进度调为最开始
      if (this.$refs.videoPlayer) {
        this.$refs.videoPlayer.hidden = false;
      }
      if (this.$refs.whiteCover) {
        this.$refs.whiteCover.hidden = false;
      }
      return new Promise((resolve) => {
        if (this.theme === 'random' || !THEMES[this.theme]) {
          const themeKeys = Object.keys(THEMES);
          const randomIndex = Math.floor(Math.random() * themeKeys.length);
          const randomTheme = themeKeys[randomIndex];
          this.audioUrl = THEMES[randomTheme].audio;
          this.videoUrl = THEMES[randomTheme].video;
        } else {
          this.audioUrl = THEMES[this.theme].audio;
          this.videoUrl = THEMES[this.theme].video;
        }
        this.$nextTick(() => {
          this.$refs.audioPlayer.load();
          this.$refs.videoPlayer.load();

          if (this.$refs.whiteCover) {
            this.$refs.whiteCover.style.opacity = 0;
          }

          if (this.$refs.videoPlayer) {
            // 视频
            this.$refs.videoPlayer.currentTime = 0;
            this.$refs.videoPlayer.style.opacity = 1;
            this.$refs.videoPlayer.play().catch(error => {
              console.error('视频播放失败:', error);
            });
          }
          if (this.$refs.audioPlayer) {
            // 音频
            this.$refs.audioPlayer.currentTime = 0;
            this.$refs.audioPlayer.play().catch(error => {
              console.error('音频播放失败:', error);
            });
          }

          if (this.$refs.videoPlayer) {
            this.$refs.videoPlayer.addEventListener('ended', () => {
              // 视频播放结束
              this.$refs.whiteCover.style.opacity = 1;
              setTimeout(() => {
                this.$refs.videoPlayer.style.opacity = 0;
                this.$refs.whiteCover.style.opacity = 0;
                setTimeout(resolve, 200); // 多等一小会儿再 resolve
              }, 1000);
            });
          }
        });
      })
      
    }
  },
  mounted() {

    // if (this.$refs.videoSource)
    //   this.$refs.videoSource.src = THEMES[this.theme].video;

    // if (this.$refs.audioSource)
    //   this.$refs.audioSource.src = THEMES[this.theme].audio;
  }
}
</script>

<style>
.background {
  position: fixed;
  background-color: black;
  left: 0;
  top: 0;
  margin: 0;
  padding: 0;
  width: 100vw;
  height: 100vh;
  z-index: -999;
}

.white-cover {
  position: fixed;
  background-color: white;
  left: 0;
  top: 0;
  margin: 0;
  padding: 0;
  width: 100vw;
  height: 100vh;
  z-index: 999;
  opacity: 0;
  transition: opacity 0.5s ease;
}

.video-player {
  position: fixed;
  left: 0;
  top: 0;
  margin: 0;
  padding: 0;
  width: 100vw;
  height: 100vh;
  z-index: 99;
  opacity: 0;
  /* transition: opacity 1s ease; */
  object-fit: cover;
}
</style>
