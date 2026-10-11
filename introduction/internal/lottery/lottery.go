// Package lottery 负责候选池的快照凭证与可复现洗牌。
//
// 现场抽奖最怕"凭什么是他"，所以这里只做两件事：把参与名单和种子钉成
// 一份可复核的凭证（snapshot + hash），再用种子确定性地洗牌取前 N 个。
// 同样的名单 + 同样的 seed + 同样的轮次号，任何时候重算出同一份结果。
package lottery

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	mrand "math/rand/v2"
	"strconv"
	"strings"
)

// 内置素材池，与前端素材文件名一致。Config.Draw 里填了就用配置的。
var (
	DefaultThemes = []string{
		"BanGDream", "GenshinImpact", "PJSK", "WutheringWaves",
		"Arknights", "StarRail", "Wuhuamixin", "DeltaForce",
	}
	DefaultImages = []string{
		"ice-cream", "coffee", "cake", "bread", "air-plane",
	}
	// AllImages 是前端素材里全部树莓娘图（部长介绍可选全套，签到仍只用上面五个）。
	AllImages = []string{
		"ice-cream", "coffee", "cake", "bread", "air-plane",
		"bass", "drum", "keyboard", "guitar1", "guitar2",
	}
)

// Pick 从素材池里随机取一个（入池时定主题/树莓娘图，无种子要求）。
func Pick(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[mrand.IntN(len(list))]
}

// NewSeed 生成一枚随机种子（16 字节，base64url）。
func NewSeed() (string, error) {
	buf := make([]byte, 16)
	if _, err := cryptorand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Snapshot 把参与名单与种子、轮次号钉成凭证，返回规范化文本与 sha256。
// ids 必须已按升序排列（调用方保证），这样同一份名单永远得到同一个 hash。
func Snapshot(roundNo int64, seed string, ids []int64) (text, hash string) {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	text = fmt.Sprintf("round=%d\nseed=%s\nids=%s\n", roundNo, seed, strings.Join(parts, ","))
	sum := sha256.Sum256([]byte(text))
	return text, hex.EncodeToString(sum[:])
}

// Shuffle 用 seed 确定性洗牌，返回洗好的下标顺序（0..len(ids)-1）。
//
// 随机源是 ChaCha8：同一 seed 必得同一序列，且不像 `Math.sin` 那种写法
// 会退化成可预测的短周期序列（旧前端版本就是那样，作为抽奖凭证不合格）。
func Shuffle(ids []int64, seed string) []int {
	sum := sha256.Sum256([]byte(seed))
	rng := mrand.New(mrand.NewChaCha8(sum))

	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	// Fisher-Yates
	for i := len(order) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	return order
}
