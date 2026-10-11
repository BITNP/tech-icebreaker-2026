package api

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"icebreaker/internal/auth"
	"icebreaker/internal/config"
	"icebreaker/internal/rbac"
	"icebreaker/internal/store"
)

const (
	adminSub = "admin-sub"
	userSub  = "random-user-sub"
)

type fixture struct {
	h     http.Handler
	auth  *auth.Auth
	admin string
	user  string
}

func newFixture(t *testing.T) *fixture {
	return newFixtureWith(t, fstest.MapFS{})
}

func newFixtureWith(t *testing.T, ui fs.FS) *fixture {
	t.Helper()
	cfg := &config.Config{}
	cfg.Auth.HMACSecret = "test-secret"
	cfg.Auth.AdminSubs = []string{adminSub}
	cfg.Auth.OIDC.Issuer = "https://keycloak.invalid/realms/master"
	cfg.Auth.OIDC.ClientID = "icebreaker"
	cfg.Auth.OIDC.ClientSecret = "x"
	cfg.Auth.OIDC.RedirectURL = "http://127.0.0.1/auth/callback"
	cfg.TokenTTL = time.Hour

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	a := auth.New(cfg)
	h := New(cfg, a, st, ui, log.New(io.Discard, "", 0)).Routes()

	return &fixture{
		h:     h,
		auth:  a,
		admin: a.Issue(adminSub, "管理员", time.Now()),
		user:  a.Issue(userSub, "路人", time.Now()),
	}
}

func (f *fixture) do(t *testing.T, method, path string, body any, token string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// raw 取原始响应体（导出、二维码这类非 JSON 响应用）。
func (f *fixture) raw(t *testing.T, method, path, token string) (int, http.Header, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	return res.StatusCode, res.Header, rec.Body.String()
}

// 现在签到必须登录：没 token 一律 401，姓名取账号里的真名，而且一个账号
// 只能有一条记录（重复点/刷新都走"已经签过"）。
func TestCheckinNeedsLoginAndRecordsOnce(t *testing.T) {
	f := newFixture(t)

	if code, _ := f.do(t, http.MethodPost, "/api/v1/checkin", nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("未登录签到应 401，得到 %d", code)
	}

	code, body := f.checkin(t, "林子煊")
	if code != http.StatusOK {
		t.Fatalf("登录后签到应成功，得到 %d %v", code, body)
	}
	if body["position"] != float64(1) || body["already"] != false {
		t.Fatalf("第一位签到应为 position=1 already=false，得到 %v", body)
	}
	if body["name"] != "林子煊" {
		t.Fatalf("姓名应取账号真名，得到 %v", body["name"])
	}

	// 同一个账号再签：不重复入列，也不报错
	code, again := f.checkin(t, "林子煊")
	if code != http.StatusOK || again["already"] != true {
		t.Fatalf("同账号重复签到应回 already=true，得到 %d %v", code, again)
	}
	if again["count"] != float64(1) {
		t.Fatalf("重复签到不该增加人数，得到 %v", again["count"])
	}

	// 换个账号才是第二位
	if _, body := f.checkin(t, "张家诚"); body["position"] != float64(2) {
		t.Fatalf("第二个账号应为 position=2，得到 %v", body)
	}

	code, state := f.do(t, http.MethodGet, "/api/v1/screen/state", nil, f.user)
	if code != http.StatusOK || state["participantCount"] != float64(2) {
		t.Fatalf("大屏应看到 2 人，得到 %d %v", code, state)
	}
	if state["signupOpen"] != true || state["round"] != nil {
		t.Fatalf("默认开签到且还没开轮，得到 %v", state)
	}
}

// 前端不再传名字：名字来自 token。空名退回 sub，超长名截断，都不该让签到失败。
func TestCheckinNameComesFromToken(t *testing.T) {
	f := newFixture(t)

	code, body := f.do(t, http.MethodPost, "/api/v1/checkin", nil,
		f.auth.Issue("sub-noname", "", time.Now()))
	if code != http.StatusOK {
		t.Fatalf("名字为空也应能签到，得到 %d %v", code, body)
	}
	if body["name"] != "sub-noname" {
		t.Fatalf("空名应退回 sub，得到 %v", body["name"])
	}

	long := "一二三四五六七八九十一二三四五六七八九十一二三四五"
	_, body = f.do(t, http.MethodPost, "/api/v1/checkin", nil,
		f.auth.Issue("sub-long", long, time.Now()))
	if got := []rune(body["name"].(string)); len(got) != maxNameRunes {
		t.Fatalf("超长名应截到 %d 个字符，得到 %d：%v", maxNameRunes, len(got), body["name"])
	}
}

// /me 让前端不必解 token 就知道自己是谁、什么角色、能干什么。
func TestMe(t *testing.T) {
	f := newFixture(t)

	if code, _ := f.do(t, http.MethodGet, "/api/v1/me", nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("未登录访问 /me 应 401，得到 %d", code)
	}
	code, body := f.do(t, http.MethodGet, "/api/v1/me", nil, f.user)
	if code != http.StatusOK || body["name"] != "路人" || body["role"] != rbac.User || body["checkedIn"] != false {
		t.Fatalf("普通账号登录后应是 user 角色：%d %v", code, body)
	}
	can, _ := body["can"].(map[string]any)
	if can["checkin"] != true || can["lottery"] != false || can["directors"] != false || can["roles"] != false {
		t.Fatalf("user 的能力应只有签到，得到 %v", body["can"])
	}
	f.do(t, http.MethodPost, "/api/v1/checkin", nil, f.user)
	_, body = f.do(t, http.MethodGet, "/api/v1/me", nil, f.user)
	if body["checkedIn"] != true {
		t.Fatalf("签到后 /me 应显示 checkedIn=true，得到 %v", body)
	}
	if _, body := f.do(t, http.MethodGet, "/api/v1/me", nil, f.admin); body["role"] != rbac.Admin || body["rootAdmin"] != true {
		t.Fatalf("配置白名单里的账号应是根管理员，得到 %v", body)
	}
}

// 四个角色各管一摊：user 只能签到、minister 管部长介绍、operator 管抽奖与大屏、
// admin 全都有；没显式分配角色时登录即 user。
func TestRoleGatesAndManagement(t *testing.T) {
	f := newFixture(t)
	minister := f.auth.Issue("sub-minister", "部长甲", time.Now())
	operator := f.auth.Issue("sub-operator", "操作员乙", time.Now())

	for _, tok := range []string{minister, operator} {
		code, body := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, tok)
		if code != http.StatusForbidden || body["error"] != "no_permission" {
			t.Fatalf("默认角色不该能进管理台，得到 %d %v", code, body)
		}
	}

	for _, tc := range []struct{ sub, role string }{{"sub-minister", rbac.Minister}, {"sub-operator", rbac.Operator}} {
		payload := map[string]string{"sub": tc.sub, "role": tc.role, "name": tc.sub}
		if code, body := f.do(t, http.MethodPost, "/api/v1/admin/roles", payload, f.admin); code != http.StatusOK {
			t.Fatalf("分配 %s 应成功，得到 %d %v", tc.role, code, body)
		}
	}

	// operator：能控大屏，碰不了角色，也写不了部长介绍
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, operator); code != http.StatusOK {
		t.Fatalf("operator 应能进管理台，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, operator); code == http.StatusForbidden {
		t.Fatalf("operator 应能发起抽奖（空池应是 409 而不是 403），得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/roles", nil, operator); code != http.StatusForbidden {
		t.Fatalf("operator 不该能管角色，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"note": "x"}, operator); code != http.StatusForbidden {
		t.Fatalf("operator 不该能写部长介绍，得到 %d", code)
	}

	// minister：反过来
	if code, body := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"theme": "GenshinImpact", "note": "大家好"}, minister); code != http.StatusOK {
		t.Fatalf("minister 应能写自己的介绍，得到 %d %v", code, body)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, minister); code != http.StatusForbidden {
		t.Fatalf("minister 不该能进管理台，得到 %d", code)
	}

	// admin：角色与部长介绍都能碰
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/roles", nil, f.admin); code != http.StatusOK {
		t.Fatalf("admin 应能管角色，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"note": "我也来"}, f.admin); code != http.StatusOK {
		t.Fatalf("admin 也应能写部长介绍，得到 %d", code)
	}

	// 撤销 = 立刻回到默认 user
	if code, _ := f.do(t, http.MethodDelete, "/api/v1/admin/roles/sub-operator", nil, f.admin); code != http.StatusOK {
		t.Fatalf("撤销角色应成功，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, operator); code != http.StatusForbidden {
		t.Fatalf("撤销后应立刻不能进管理台，得到 %d", code)
	}
	if _, body := f.do(t, http.MethodGet, "/api/v1/me", nil, operator); body["role"] != rbac.User {
		t.Fatalf("撤销后 /me 应回到 user，得到 %v", body)
	}

	// 配置白名单是根管理员，库里的操作动不了它
	if code, body := f.do(t, http.MethodPost, "/api/v1/admin/roles", map[string]string{"sub": adminSub, "role": rbac.User}, f.admin); code != http.StatusConflict || body["error"] != "config_admin_immutable" {
		t.Fatalf("根管理员不该被改，得到 %d %v", code, body)
	}
	if code, body := f.do(t, http.MethodDelete, "/api/v1/admin/roles/"+adminSub, nil, f.admin); code != http.StatusConflict || body["error"] != "config_admin_immutable" {
		t.Fatalf("根管理员不该被撤，得到 %d %v", code, body)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/roles", map[string]string{"sub": "whoever", "role": "superuser"}, f.admin); code != http.StatusBadRequest {
		t.Fatalf("非法角色名应 400，得到 %d", code)
	}
}

// 部长介绍：列表公开；只有部长（与 admin）能写自己的那一条；素材名不信任前端。
func TestDirectors(t *testing.T) {
	f := newFixture(t)
	minister := f.auth.Issue("sub-m1", "部长甲", time.Now())
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/roles", map[string]string{"sub": "sub-m1", "role": rbac.Minister}, f.admin); code != http.StatusOK {
		t.Fatalf("分配 minister 失败：%d", code)
	}

	code, body := f.do(t, http.MethodGet, "/api/v1/directors", nil, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("全站强制登录后，未登录读列表应 401，得到 %d %v", code, body)
	}
	code, body = f.do(t, http.MethodGet, "/api/v1/directors", nil, f.user)
	if code != http.StatusOK {
		t.Fatalf("登录后应能读列表，得到 %d %v", code, body)
	}
	if list, _ := body["directors"].([]any); len(list) != 0 {
		t.Fatalf("初始应为空，得到 %v", body["directors"])
	}

	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"note": "hi"}, ""); code != http.StatusUnauthorized {
		t.Fatalf("未登录写介绍应 401，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"note": "hi"}, f.user); code != http.StatusForbidden {
		t.Fatalf("普通部员写介绍应 403，得到 %d", code)
	}

	// 非法素材名会被替换成已知集合里的值
	_, weird := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"theme": "../../etc/passwd", "shumeiniang": "nope"}, minister)
	if weird["theme"] == "../../etc/passwd" || weird["shumeiniang"] == "nope" {
		t.Fatalf("非法素材名应被替换，得到 %v", weird)
	}

	// 正常写入：姓名取 token 里的真名
	code, body = f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"theme": "GenshinImpact", "shumeiniang": "guitar2", "note": "我是部长甲"}, minister)
	if code != http.StatusOK || body["name"] != "部长甲" || body["theme"] != "GenshinImpact" {
		t.Fatalf("部长写介绍应成功且姓名取真名，得到 %d %v", code, body)
	}

	// 同一个人重复提交是修改，列表里仍只有一条
	_, body = f.do(t, http.MethodGet, "/api/v1/directors", nil, f.user)
	list, _ := body["directors"].([]any)
	if len(list) != 1 {
		t.Fatalf("同一人应只有一条介绍，得到 %v", list)
	}
	first, _ := list[0].(map[string]any)
	if first["name"] != "部长甲" || first["note"] != "我是部长甲" || first["shumeiniang"] != "guitar2" {
		t.Fatalf("列表内容不对：%v", first)
	}

	if code, _ := f.do(t, http.MethodDelete, "/api/v1/directors/me", nil, minister); code != http.StatusOK {
		t.Fatalf("部长应能删自己的介绍，得到 %d", code)
	}
	_, body = f.do(t, http.MethodGet, "/api/v1/directors", nil, f.user)
	if list, _ := body["directors"].([]any); len(list) != 0 {
		t.Fatalf("删完应为空，得到 %v", list)
	}
}

// admin 能代任何人建/改/删部长介绍；minister 只能碰自己那条；
// 姓名一律取组织账号真名（编不出来的 sub 直接拒绝）。
func TestAdminCanEditAnyDirector(t *testing.T) {
	f := newFixture(t)
	minister := f.auth.Issue("sub-minister", "部长甲", time.Now())
	f.do(t, http.MethodPost, "/api/v1/admin/roles", map[string]string{"sub": "sub-minister", "role": rbac.Minister}, f.admin)
	f.checkin(t, "部长乙") // 已签到 → sub-部长乙 的真名可查

	// 既没签到、也没有已有介绍的人：取不到真名，拒绝
	code, body := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"sub": "sub-nobody", "note": "x"}, f.admin)
	if code != http.StatusBadRequest || body["error"] != "unknown_sub" {
		t.Fatalf("取不到真名应 400 unknown_sub，得到 %d %v", code, body)
	}

	// admin 代已签到的部长乙建介绍：姓名取的是乙的真名，不是 admin 自己的
	code, body = f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"sub": "sub-部长乙", "theme": "PJSK", "shumeiniang": "bass", "note": "乙的介绍"}, f.admin)
	if code != http.StatusOK || body["name"] != "部长乙" {
		t.Fatalf("admin 代建介绍应取对方真名，得到 %d %v", code, body)
	}

	// admin 改别人的
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"sub": "sub-部长乙", "note": "admin 改过"}, f.admin); code != http.StatusOK {
		t.Fatalf("admin 应能改别人的介绍，得到 %d", code)
	}
	_, body = f.do(t, http.MethodGet, "/api/v1/directors", nil, f.user)
	list, _ := body["directors"].([]any)
	if len(list) != 1 {
		t.Fatalf("应只有一条介绍，得到 %v", list)
	}
	first, _ := list[0].(map[string]any)
	if first["note"] != "admin 改过" {
		t.Fatalf("列表应反映 admin 的修改，得到 %v", first)
	}

	// minister：改不了别人的、删不了别人的，自己的照旧
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"sub": "sub-部长乙", "note": "我改别人的"}, minister); code != http.StatusForbidden {
		t.Fatalf("minister 不该能改别人的介绍，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodDelete, "/api/v1/directors/sub-部长乙", nil, minister); code != http.StatusForbidden {
		t.Fatalf("minister 不该能删别人的介绍，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/directors", map[string]string{"theme": "Arknights"}, minister); code != http.StatusOK {
		t.Fatalf("minister 应能改自己的介绍，得到 %d", code)
	}

	// admin 删别人的
	if code, _ := f.do(t, http.MethodDelete, "/api/v1/directors/sub-部长乙", nil, f.admin); code != http.StatusOK {
		t.Fatalf("admin 应能删别人的介绍，得到 %d", code)
	}
	_, body = f.do(t, http.MethodGet, "/api/v1/directors", nil, f.user)
	if list, _ := body["directors"].([]any); len(list) != 1 {
		t.Fatalf("删完应只剩 minister 自己那条，得到 %v", list)
	}
	// 自己删自己的入口照旧
	if code, _ := f.do(t, http.MethodDelete, "/api/v1/directors/me", nil, minister); code != http.StatusOK {
		t.Fatalf("自己删自己的入口不该坏，得到 %d", code)
	}
}

// 全站强制登录：过去公开的读接口（大屏状态/名单、部长介绍列表）现在也要求登录，
// 只有静态资源、二维码与 healthz 例外。
func TestEverythingRequiresLogin(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{
		"/api/v1/screen/state",
		"/api/v1/screen/round",
		"/api/v1/directors",
		"/api/v1/me",
		"/api/v1/admin/state",
	} {
		if code, _ := f.do(t, http.MethodGet, path, nil, ""); code != http.StatusUnauthorized {
			t.Fatalf("%s 未登录应 401，得到 %d", path, code)
		}
		if code, _ := f.do(t, http.MethodPost, "/api/v1/checkin", nil, ""); code != http.StatusUnauthorized {
			t.Fatalf("签到未登录应 401，得到 %d", code)
		}
	}
	// 例外：健康检查与二维码（<img> 带不了 Authorization 头）
	if code, _, _ := f.raw(t, http.MethodGet, "/healthz", ""); code != http.StatusOK {
		t.Fatalf("healthz 应保持公开，得到 %d", code)
	}
	if code, header, _ := f.raw(t, http.MethodGet, "/api/v1/qr?data=hi", ""); code != http.StatusOK {
		t.Fatalf("二维码应保持公开，得到 %d", code)
	} else if header.Get("Content-Type") != "image/png" {
		t.Fatalf("二维码应是 PNG，得到 %q", header.Get("Content-Type"))
	}
}

// 登录回跳的目标必须只允许本站相对路径，且 token 要落在 hash 之前。
func TestLoginRedirectTarget(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/#/checkin", "/#/checkin"},
		{"/#/admin", "/#/admin"},
		{"/directors", "/directors"},
		{"", "/"},
		{"https://evil.example/x", "/"},
		{"//evil.example/x", "/"},
		{"#/checkin", "/"},
		{"/a|b", "/"},
	} {
		if got := sanitizeNext(tc.in); got != tc.want {
			t.Fatalf("sanitizeNext(%q) = %q，应为 %q", tc.in, got, tc.want)
		}
	}
	if got := tokenRedirect("/#/checkin", "T"); got != "/?token=T#/checkin" {
		t.Fatalf("hash 路由的回跳应把 token 放在 hash 前，得到 %q", got)
	}
	if got := tokenRedirect("/directors", "T"); got != "/directors?token=T" {
		t.Fatalf("普通路径回跳不对：%q", got)
	}
}

// 管理侧三道门：没 token 401、有 token 但不在白名单 403、白名单 200。
func TestAdminGate(t *testing.T) {
	f := newFixture(t)

	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.user); code != http.StatusForbidden {
		t.Fatalf("非白名单应 403，得到 %d", code)
	}
	if code, body := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin); code != http.StatusOK {
		t.Fatalf("管理员应 200，得到 %d %v", code, body)
	}
	// 篡改过的 token 不认
	if code, _ := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin+"x"); code != http.StatusUnauthorized {
		t.Fatalf("坏签名应 401，得到 %d", code)
	}
}

func TestSignupSwitch(t *testing.T) {
	f := newFixture(t)

	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/checkin/close", nil, f.admin); code != http.StatusOK {
		t.Fatalf("关签到应成功，得到 %d", code)
	}
	if code, body := f.checkin(t, "迟到的"); code != http.StatusConflict {
		t.Fatalf("关签到后签到应 409，得到 %d %v", code, body)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/checkin/open", nil, f.admin); code != http.StatusOK {
		t.Fatalf("开签到应成功，得到 %d", code)
	}
	if code, _ := f.checkin(t, "迟到的"); code != http.StatusOK {
		t.Fatalf("开签到后应能签到，得到 %d", code)
	}
}

func TestDrawFlow(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"甲", "乙", "丙", "丁", "戊"} {
		if code, body := f.checkin(t, name); code != http.StatusOK {
			t.Fatalf("签到 %s 失败：%d %v", name, code, body)
		}
	}

	// 抽的人比池子多：拒绝，并告诉现场还剩几个
	code, body := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 9}, f.admin)
	if code != http.StatusConflict || body["error"] != "not_enough_participants" {
		t.Fatalf("人数不够应 409 not_enough_participants，得到 %d %v", code, body)
	}

	code, body = f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{
		"count": 2, "label": "三等奖", "seed": "test-seed",
	}, f.admin)
	if code != http.StatusOK {
		t.Fatalf("发起抽奖应成功，得到 %d %v", code, body)
	}
	if body["no"] != float64(1) || body["seed"] != "test-seed" {
		t.Fatalf("轮次号/种子不对：%v", body)
	}

	// 大屏预载名单：顺序与人名都在，但不含 seed / 快照 hash
	code, round := f.do(t, http.MethodGet, "/api/v1/screen/round", nil, f.user)
	if code != http.StatusOK {
		t.Fatalf("大屏取名单应成功，得到 %d %v", code, round)
	}
	entries, ok := round["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("应有 2 位中奖者，得到 %v", round["entries"])
	}
	for _, leak := range []string{"seed", "snapshot", "snapshotHash"} {
		if _, bad := round[leak]; bad {
			t.Fatalf("公开接口不该出现 %s：%v", leak, round)
		}
	}

	// 同一个种子必然抽出同一份名单：现场复核靠这个
	code, again := f.do(t, http.MethodPost, "/api/v1/admin/reset", map[string]string{"scope": "winners"}, f.admin)
	if code != http.StatusOK {
		t.Fatalf("重置中奖应成功，得到 %d %v", code, again)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{
		"count": 2, "seed": "test-seed",
	}, f.admin); code != http.StatusOK {
		t.Fatalf("重抽应成功，得到 %d", code)
	}
	_, round2 := f.do(t, http.MethodGet, "/api/v1/screen/round", nil, f.user)
	if names(t, round) != names(t, round2) {
		t.Fatalf("同种子两次抽奖结果应一致：%s vs %s", names(t, round), names(t, round2))
	}

	// 推进 / 跳过 / 结束本轮
	if code, body := f.do(t, http.MethodPost, "/api/v1/admin/rounds/current/cursor", map[string]any{"revealedIndex": 0}, f.admin); code != http.StatusOK {
		t.Fatalf("推进游标应成功，得到 %d %v", code, body)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds/current/skip", nil, f.admin); code != http.StatusOK {
		t.Fatalf("跳过动画应成功，得到 %d", code)
	}
	_, state := f.do(t, http.MethodGet, "/api/v1/screen/state", nil, f.user)
	rd, _ := state["round"].(map[string]any)
	if rd == nil || rd["revealedIndex"] != float64(0) || rd["skipSeq"] != float64(1) {
		t.Fatalf("大屏状态没跟上管理端：%v", state)
	}
	// 游标越界要被夹住，不能让管理端把大屏指到不存在的人
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds/current/cursor", map[string]any{"revealedIndex": 99}, f.admin); code != http.StatusOK {
		t.Fatalf("越界游标应被接受并夹住，得到 %d", code)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/screen/state", nil, f.user)
	rd, _ = state["round"].(map[string]any)
	if rd["revealedIndex"] != float64(1) {
		t.Fatalf("游标应被夹到 1，得到 %v", rd["revealedIndex"])
	}

	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds/current/end", nil, f.admin); code != http.StatusOK {
		t.Fatalf("结束本轮应成功，得到 %d", code)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/screen/state", nil, f.user)
	if state["round"] != nil {
		t.Fatalf("结束后大屏应回到待机，得到 %v", state["round"])
	}
}

// 中奖者退出后续轮次（remove_winners 默认开）：第二轮只能从剩下的人里抽。
func TestWinnersLeaveThePool(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"甲", "乙", "丙"} {
		f.checkin(t, name)
	}
	if code, body := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, f.admin); code != http.StatusOK {
		t.Fatalf("第一轮应成功，得到 %d %v", code, body)
	}
	_, round := f.do(t, http.MethodGet, "/api/v1/screen/round", nil, f.user)
	first := names(t, round)

	if code, body := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 3}, f.admin); code != http.StatusConflict {
		t.Fatalf("只剩 2 人，抽 3 人应 409，得到 %d %v", code, body)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 2}, f.admin); code != http.StatusOK {
		t.Fatalf("第二轮应成功，得到 %d", code)
	}
	_, round2 := f.do(t, http.MethodGet, "/api/v1/screen/round", nil, f.user)
	for _, name := range splitNames(names(t, round2)) {
		if name == first {
			t.Fatalf("第一轮中奖者 %s 不该出现在第二轮（remove_winners 默认开）", name)
		}
	}

	// removeWinners=false 时，中奖者留在池子里，可以再被抽中
	_, _ = f.do(t, http.MethodPost, "/api/v1/admin/reset", map[string]string{"scope": "winners"}, f.admin)
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 3, "removeWinners": false}, f.admin); code != http.StatusOK {
		t.Fatalf("允许重复中奖时应能抽满 3 人，得到 %d", code)
	}
}

func TestManualAddImportDeleteAndReset(t *testing.T) {
	f := newFixture(t)

	if code, body := f.do(t, http.MethodPost, "/api/v1/admin/participants", map[string]string{"name": "手动加的人"}, f.admin); code != http.StatusOK {
		t.Fatalf("手动加人应成功，得到 %d %v", code, body)
	}
	code, body := f.do(t, http.MethodPost, "/api/v1/admin/import", map[string]any{
		"names": []string{"导入甲", "", "导入乙"}, "mode": "append",
	}, f.admin)
	if code != http.StatusOK || body["added"] != float64(2) {
		t.Fatalf("导入应跳过空行并加 2 人，得到 %d %v", code, body)
	}

	_, state := f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin)
	people, _ := state["participants"].([]any)
	if len(people) != 3 {
		t.Fatalf("应有 3 人，得到 %v", state["participants"])
	}
	first := people[0].(map[string]any)

	id := int64(first["id"].(float64))
	if code, body := f.do(t, http.MethodDelete, "/api/v1/admin/participants/"+strconv.FormatInt(id, 10), nil, f.admin); code != http.StatusOK {
		t.Fatalf("删人应成功，得到 %d %v", code, body)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin)
	if people, _ := state["participants"].([]any); len(people) != 2 {
		t.Fatalf("删完应剩 2 人，得到 %v", state["participants"])
	}

	// 覆盖导入：名单换掉，轮次与中奖记录一起清空
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, f.admin); code != http.StatusOK {
		t.Fatalf("先抽一轮，得到 %d", code)
	}
	code, body = f.do(t, http.MethodPost, "/api/v1/admin/import", map[string]any{
		"names": []string{"新甲"}, "mode": "replace",
	}, f.admin)
	if code != http.StatusOK || body["count"] != float64(1) {
		t.Fatalf("覆盖导入后应只剩 1 人，得到 %d %v", code, body)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin)
	if rounds, _ := state["rounds"].([]any); len(rounds) != 0 {
		t.Fatalf("覆盖导入应清空轮次，得到 %v", state["rounds"])
	}
	if state["current"] != nil {
		t.Fatalf("覆盖导入后应回到待机，得到 %v", state["current"])
	}

	// 重置中奖：保留名单，清掉轮次
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, f.admin); code != http.StatusOK {
		t.Fatalf("再抽一轮，得到 %d", code)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/reset", map[string]string{"scope": "winners"}, f.admin); code != http.StatusOK {
		t.Fatalf("重置中奖应成功，得到 %d", code)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin)
	if people, _ := state["participants"].([]any); len(people) != 1 {
		t.Fatalf("重置中奖应保留名单，得到 %v", state["participants"])
	}

	// 清空整场：连名单一起
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/reset", map[string]string{"scope": "all"}, f.admin); code != http.StatusOK {
		t.Fatalf("清空整场应成功，得到 %d", code)
	}
	_, state = f.do(t, http.MethodGet, "/api/v1/admin/state", nil, f.admin)
	if state["participantCount"] != float64(0) {
		t.Fatalf("清空整场后应为 0 人，得到 %v", state["participantCount"])
	}
}

// 重置中奖之后轮次号必须继续往前走：大屏靠"轮次号变了"这个信号重新拉名单，
// 一旦号回退重来，大屏会接着播上一轮的人（2026-10-11 实际踩到过）。
func TestRoundNoKeepsIncreasingAfterReset(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"甲", "乙", "丙"} {
		f.checkin(t, name)
	}

	code, first := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, f.admin)
	if code != http.StatusOK {
		t.Fatalf("第一轮应成功，得到 %d %v", code, first)
	}
	if code, _ := f.do(t, http.MethodPost, "/api/v1/admin/reset", map[string]string{"scope": "winners"}, f.admin); code != http.StatusOK {
		t.Fatalf("重置中奖应成功，得到 %d", code)
	}
	code, second := f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1}, f.admin)
	if code != http.StatusOK {
		t.Fatalf("重置后应能再抽，得到 %d %v", code, second)
	}
	if second["no"].(float64) <= first["no"].(float64) {
		t.Fatalf("重置后轮次号应继续递增，得到 %v → %v", first["no"], second["no"])
	}
}

// 导出是现场复核的落点：结果里要有轮次、名次、姓名和种子。
func TestExports(t *testing.T) {
	f := newFixture(t)
	f.checkin(t, "甲")
	f.do(t, http.MethodPost, "/api/v1/admin/rounds", map[string]any{"count": 1, "seed": "s1"}, f.admin)

	for _, tc := range []struct{ path, want string }{
		{"/api/v1/admin/export/participants", "甲"},
		{"/api/v1/admin/export/results", "s1"},
	} {
		code, header, body := f.raw(t, http.MethodGet, tc.path, f.admin)
		if code != http.StatusOK {
			t.Fatalf("%s 应 200，得到 %d", tc.path, code)
		}
		if !strings.Contains(body, tc.want) {
			t.Fatalf("%s 应包含 %q，得到 %q", tc.path, tc.want, body)
		}
		if ct := header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Fatalf("导出应是纯文本，得到 %q", ct)
		}
	}
}

// account 造一个"某个组织账号"的 token；sub 由名字派生，保证每人不同。
func (f *fixture) account(t *testing.T, name string) string {
	t.Helper()
	return f.auth.Issue("sub-"+name, name, time.Now())
}

// checkin 以某个账号签到（姓名取 token 里的，不再由前端传）。
func (f *fixture) checkin(t *testing.T, name string) (int, map[string]any) {
	t.Helper()
	return f.do(t, http.MethodPost, "/api/v1/checkin", nil, f.account(t, name))
}

// 静态托管：有构建产物就发页面；没有就给一句"前端未构建"，而不是 404。
func TestStaticHandler(t *testing.T) {
	built := newFixtureWith(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div id="app"></div>`)},
	})
	code, header, body := built.raw(t, http.MethodGet, "/", "")
	if code != http.StatusOK || !strings.Contains(body, `id="app"`) {
		t.Fatalf("/ 应返回构建好的页面，得到 %d %q", code, body)
	}
	if header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("静态资源应每次回源校验，得到 %q", header.Get("Cache-Control"))
	}

	unbuilt := newFixture(t)
	if code, _, body := unbuilt.raw(t, http.MethodGet, "/", ""); code != http.StatusServiceUnavailable || !strings.Contains(body, "前端还没构建") {
		t.Fatalf("未构建时应给提示页，得到 %d %q", code, body)
	}

	// 机器路径找不到文件时给 JSON 404，别让脚本拿到一份 HTML
	code, header, body = built.raw(t, http.MethodGet, "/api/v1/nope", "")
	if code != http.StatusNotFound || !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
		t.Fatalf("未知接口应 JSON 404，得到 %d %q %q", code, header.Get("Content-Type"), body)
	}
	if code, _, _ := built.raw(t, http.MethodGet, "/healthz", ""); code != http.StatusOK {
		t.Fatalf("healthz 应 200，得到 %d", code)
	}
}

func names(t *testing.T, round map[string]any) string {
	t.Helper()
	entries, _ := round["entries"].([]any)
	out := ""
	for i, e := range entries {
		m, _ := e.(map[string]any)
		if i > 0 {
			out += ","
		}
		out += m["name"].(string)
	}
	return out
}

func splitNames(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range bytes.Split([]byte(s), []byte(",")) {
		out = append(out, string(p))
	}
	return out
}
