<!-- 首页 -->
<template>
  <div class="background"></div>

  <!-- 修改这个div的样式，添加flex布局使其内容居中 -->
  <div style="position: fixed; width: 100vw; height: 100vh; display: flex; flex-direction: column; justify-content: center; align-items: center;">
    <div class="title-container">
      <h1 class="title">网络开拓者协会技术部破冰会</h1>
      <p class="date">2026年10月11日</p>
    </div>
    <br>
    <br>
    <br>
    <div class="buttons-container">
      <button @click="goToDirectors" class="tech-button">部长介绍</button>
      <!-- <button @click="goToLotteryOrdered" class="tech-button">部员介绍</button>
      <button @click="goToLotteryRandom" class="tech-button">抽奖游戏</button> -->
      <button @click="goToCheckin" class="tech-button">现场签到</button>
      <button @click="goToLottery" class="tech-button">抽奖游戏</button>
    </div>
  </div>

</template>

<script>
export default {
  name: 'IndexPage',
  data() {
    return {
      // 不需要额外的数据
    }
  },
  methods: {
    // 导航到部长介绍页面
    goToDirectors() {
      console.log("click!")
      window.location.hash = '/directors';
    },
    
    // 导航到部员介绍页面
    goToLotteryOrdered() {
      window.location.hash = '/lottery-ordered';
    },
    
    // 导航到抽奖游戏页面
    goToLotteryRandom() {
      window.location.hash = '/lottery-random';
    },

    goToLottery() {
      // 新的抽奖界面
      window.location.hash = '/lottery';
    },

    // 签到页（扫码进来的也是这个页面）
    goToCheckin() {
      window.location.hash = '/checkin';
    }
  },
  mounted() {
    // Vue组件名称应该与文件名保持一致
    this.$options.name = 'IndexPage';
  }
}
</script>

<style scoped>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body, html {
  height: 100%;
  overflow: hidden;
}

.background {
  width: 100vw;
  height: 100vh;
  position: fixed;
  top: 0;
  left: 0;
  margin: 0;
  background-size: cover;
  background-image: url("@/assets/bg0.jpeg");
  display: flex;
  justify-content: center;
  align-items: center;
}

.title-container {
  text-align: center;
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2rem;
}

.title {
  font-size: 3.5rem;
  color: white;
  text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.5);
  font-family: 'Microsoft YaHei', 'PingFang SC', sans-serif;
  font-weight: bold;
}

.date {
  font-size: 1.5rem;
  color: white;
  text-shadow: 1px 1px 2px rgba(0, 0, 0, 0.5);
  opacity: 0.9;
}

/* 按钮容器样式 - 移除margin-top以更好地居中 */
.buttons-container {
  display: flex;
  gap: 1.5rem;
  justify-content: center;
  flex-wrap: wrap;
  /* 移除margin-top，依靠父容器的flex布局实现居中 */
}

.tech-button {
  padding: 0.8rem 1.5rem;
  background-color: #2d5eff;
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 1rem;
  cursor: pointer;
  transition: background-color 0.3s;
}

</style>
