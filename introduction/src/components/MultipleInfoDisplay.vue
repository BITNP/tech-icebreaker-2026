<!-- 个人信息展示页 -->
<template>
    <div class="container">
        <!-- <div :class="(show && displayAvatarUrl) ? 'avatar' : 'avatar avatar-hidden'" :style="{ backgroundImage: `url(${displayAvatarUrl})`, backgroundSize: 'cover' }"></div> -->

        <div class="fullscreen-grid">
            
            <div v-for="(row, rowIndex) in 5" :key="`row-${rowIndex}`" class="grid-row">
                <div v-for="(col, colIndex) in 2" :key="`col-${rowIndex}-${colIndex}`" class="grid-item">
                    <div :class="((colIndex % 2) ? 'info-box-right' : 'info-box-right') + ((show && displayName[rowIndex * 2 + colIndex]) ? '' : ' hidden')">
                        <div class="name"> {{ displayName[rowIndex * 2 + colIndex] || '' }} </div>
                    </div>
                </div>
            </div>
        </div>

        
        <div :class="show ? 'shumeiniang' : 'shumeiniang shumeiniang-hidden'" :style="{ backgroundImage: `url(${shumeiniangUrl})`}"></div>
    </div>
    
</template>

<script>

const SHUMEINIANG_IMAGES = {
  "ice-cream": {url: "/media/shumeiniang-ice-cream.png"},
  "coffee": {url: "/media/shumeiniang-coffee.png"},
  "cake": {url: "/media/shumeiniang-cake.png"},
  "bread": {url: "/media/shumeiniang-bread.png"},
  "air-plane": {url: "/media/shumeiniang-air-plane.png"},

  "bass": {url: "/media/shumeiniang-bass.png"},
  "drum": {url: "/media/shumeiniang-drum.png"},
  "keyboard": {url: "/media/shumeiniang-keyboard.png"},
  "guitar1": {url: "/media/shumeiniang-guitar1.png"},
  "guitar2": {url: "/media/shumeiniang-guitar2.png"},
}
export default {
  name: 'MultipleInfoDisplay',
  components: {
    // ...
  },
  props: {
    name: {
      type: Array[String],
      default: []
    },
    avatarUrl: {
        type: Array[String],
        default: []
    },
    shumeiniangImage: {
        type: String,
        default: 'random'
    }
  },
  data() {
    return {
        show: false,
        shumeiniangUrl: "",
        displayName: [],
        displayAvatarUrl: [],
    }
  },
  methods: {
    hide() {
        this.displayName = [];
        this.displayAvatarUrl = [];
        this.show = false;
    },
    display() {
        this.displayName = this.name;
        this.displayAvatarUrl = this.avatarUrl;
        if (this.shumeiniangImage === 'random' || !SHUMEINIANG_IMAGES[this.shumeiniangImage]) {
            const keys = Object.keys(SHUMEINIANG_IMAGES);
            const randomKey = keys[Math.floor(Math.random() * keys.length)];
            this.shumeiniangUrl = SHUMEINIANG_IMAGES[randomKey].url;
        } else {
            this.shumeiniangUrl = SHUMEINIANG_IMAGES[this.shumeiniangImage].url;
        }

        this.$nextTick(() => {
            this.show = true;
        });
        
    }
  },
  mounted() {
    // ...
  }
}
</script>

<style scoped>
/* Animations */
@keyframes avatar-hover {
    0% { transform: rotateZ(10deg)}
    50% { transform: rotateZ(-10deg)}
    100% { transform: rotateZ(10deg)}
}

@keyframes info-box-right-hover {
    0% { transform: rotateY(-15deg) rotateZ(5deg) translate(0, 0);}
    25% { transform: rotateY(-20deg) rotateZ(10deg) translate(-10px, 10px)}
    50% { transform: rotateY(-25deg) rotateZ(5deg) translate(0, 20px)}
    75% { transform: rotateY(-20deg) rotateZ(0deg) translate(10px, 10px)}
    100% { transform: rotateY(-15deg) rotateZ(5deg) translate(0, 0)}
}

@keyframes info-box-left-hover {
    0% { transform: rotateY(-15deg) rotateZ(5deg) translate(0, 0);}
    25% { transform: rotateY(-20deg) rotateZ(10deg) translate(-10px, 10px)}
    50% { transform: rotateY(-25deg) rotateZ(5deg) translate(0, 20px)}
    75% { transform: rotateY(-20deg) rotateZ(0deg) translate(10px, 10px)}
    100% { transform: rotateY(-15deg) rotateZ(5deg) translate(0, 0)}
}

/* Styles */
.container {
    width: 100vw;
    height: 100vh;
    position: fixed;
    transform-style: preserve-3d;
    perspective: 800px;
    left: 0;
    top: 0;
    margin: 0;
    padding: 0;
}

.info-box-right {
    margin-left: 10%;
    width: 80%;
    height: 80%;
    background-color: #52a5ff;
    border: 10px solid #aee8ff;
    border-radius: 30px;
    opacity: 1;
    z-index: 5;
    transform: rotateY(-15deg) rotateZ(5deg);
    transition: opacity 0.5s ease, transform 0.8s ease;
    animation: info-box-right-hover 10s infinite linear 0.8s;
}

/* DEPRECATED */
.info-box-left {
    margin-left: 10%;
    width: 80%;
    height: 80%;
    background-color: #52a5ff;
    border: 10px solid #aee8ff;
    border-radius: 30px;
    opacity: 1;
    z-index: 5;
    transform: rotateY(15deg) rotateZ(-5deg);
    transition: opacity 0.5s ease, transform 0.8s ease;
    animation: info-box-left-hover 10s infinite linear 0.8s;
}

.info-box-left.hidden {
    opacity: 0;
    transform: scale(0.1) translateX(-1500px) rotateY(-540deg);
    animation: none;
}

.info-box-right.hidden {
    opacity: 0;
    transform: scale(0.1) translateX(1500px) rotateY(-180deg);
    animation: none;
}

.avatar {
    position: fixed;
    top: 30vh;
    left: 30vw;
    width: 30vh;
    aspect-ratio: 1;
    border-radius: 50%;
    object-fit: cover;
    border: 3px solid #fff;
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
    opacity: 1;
    z-index: 5;
    transform: scale(1);
    transition: opacity 1s ease, transform 1s ease;
    animation: avatar-hover 6s infinite ease-in-out 1s;
}
.avatar-hidden {
    opacity: 0;
    transform: scale(0.1) rotateY(360deg);
}

.name {
    font-family: 'Microsoft YaHei', sans-serif; /* 可爱风格字体 */
    color: #e2f7ff;
    margin: 15px 0 10px;
    font-size: 5em;
    text-shadow: 1px 1px 2px rgba(0, 0, 0, 0.5);
    text-align: center;
}
.department {
    color: #fff;
    font-size: 16px;
    text-shadow: 1px 1px 2px rgba(0, 0, 0, 0.5);
}

.shumeiniang {
    position: fixed;
    width: 100vw;
    height: 100vh;
    /* height: auto; */
    aspect-ratio: 16/9;
    left: 0;
    top: 0;
    background-repeat: no-repeat;
    background-size: auto 100%;
    opacity: 1;
    transform: translate(0) rotateZ(-15deg);
    transition: opacity 0.5s ease, transform 0.5s ease;
    z-index: 2;
    /* animation: shumeiniang-hover 6s infinite ease-in-out 1s; */
}
.shumeiniang-hidden {
    opacity: 0;
    transform: translateY(100px) rotateZ(10deg) scaleX(0.1) scaleY(0.05);
    animation: none;
}

.fullscreen-grid {
    position: fixed;
    width: 80vw;
    height: 100vh;
    left: 20vw;
    top: 0;
    display: grid;
    grid-template-rows: repeat(5, 1fr);
    grid-template-columns: repeat(2, 1fr);
    z-index: 3;
}

.grid-row {
    display: contents;
}

.grid-item {
    transform-style: preserve-3d;
    perspective: 800px;
}
</style>