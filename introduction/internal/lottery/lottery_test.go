package lottery

import "testing"

func TestShuffleIsDeterministic(t *testing.T) {
	ids := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	first := Shuffle(ids, "seed-a")
	second := Shuffle(ids, "seed-a")
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("同种子两次洗牌不一致：%v vs %v", first, second)
		}
	}
	if same := Shuffle(ids, "seed-b"); equalOrder(first, same) {
		t.Fatalf("换种子应得到不同顺序：%v", same)
	}
	// 洗牌必须是排列：不重不漏
	seen := map[int]bool{}
	for _, v := range first {
		if v < 0 || v >= len(ids) || seen[v] {
			t.Fatalf("洗牌结果不是排列：%v", first)
		}
		seen[v] = true
	}
}

func TestSnapshotIsCanonical(t *testing.T) {
	_, h1 := Snapshot(1, "s", []int64{1, 2, 3})
	_, h2 := Snapshot(1, "s", []int64{1, 2, 3})
	if h1 != h2 {
		t.Fatalf("同样输入应得同样 hash：%s vs %s", h1, h2)
	}
	for _, tc := range []struct {
		no   int64
		seed string
		ids  []int64
	}{
		{2, "s", []int64{1, 2, 3}},
		{1, "s2", []int64{1, 2, 3}},
		{1, "s", []int64{1, 2, 4}},
		{1, "s", []int64{1, 2}},
	} {
		if _, h := Snapshot(tc.no, tc.seed, tc.ids); h == h1 {
			t.Fatalf("轮次/种子/名单任一变化都应改变 hash：%+v", tc)
		}
	}
}

func TestSeedIsRandomEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		s, err := NewSeed()
		if err != nil {
			t.Fatalf("生成种子: %v", err)
		}
		if seen[s] {
			t.Fatalf("种子重复：%s", s)
		}
		seen[s] = true
	}
}

func TestPickStaysInList(t *testing.T) {
	list := []string{"a", "b", "c"}
	got := map[string]bool{}
	for i := 0; i < 200; i++ {
		v := Pick(list)
		if !contains(list, v) {
			t.Fatalf("Pick 返回了列表外的值：%q", v)
		}
		got[v] = true
	}
	if len(got) != len(list) {
		t.Fatalf("200 次抽样应覆盖全部 3 个值，得到 %v", got)
	}
	if Pick(nil) != "" {
		t.Fatalf("空列表应返回空串")
	}
}

func equalOrder(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
