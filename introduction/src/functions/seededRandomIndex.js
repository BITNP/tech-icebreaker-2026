/**
 * 使用种子从数组中随机抽取N个不重复的index
 * @param {Array} array - 源数组
 * @param {number} n - 要抽取的元素数量
 * @param {number|string} seed - 随机种子
 * @returns {Array} 包含随机抽取元素的数组
 */
export default function seededRandomIndex(array, n, seed) {
    // 参数校验
    if (!Array.isArray(array)) throw new Error('第一个参数必须是数组');
    if (n > array.length) throw new Error('抽取数量不能超过数组长度');
    
    // 创建一个简单的种子随机数生成器
    function seededRandom(seed) {
        let x = typeof seed === 'string' 
            ? seed.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0)
            : seed;
        
        // 使用线性同余生成器算法
        return function() {
            x = Math.sin(x) * 10000;
            return x - Math.floor(x);
        };
    }
    
    const random = seededRandom(seed);

    const indexArray = [...array.keys()];
    
    // Fisher-Yates洗牌算法，使用种子随机数
    for (let i = indexArray.length - 1; i > 0; i--) {
        const j = Math.floor(random() * (i + 1));
        [indexArray[i], indexArray[j]] = [indexArray[j], indexArray[i]];
    }
    
    // 前n个索引
    const indexes = indexArray.slice(0, n);
    return indexes;
}