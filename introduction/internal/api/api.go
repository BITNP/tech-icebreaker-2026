// Package api 提供公开侧（签到、大屏）、管理侧（Bearer + 白名单）与静态托管。
//
// 公开侧不需要任何凭据：到场的人扫码输名字就进池，大屏谁打开都能看。
// 管理侧一律要求 Keycloak 登录换来的 HMAC token，且 sub 必须在
// config.auth.admin_subs 白名单里 —— 权限判定只在这里做，前端藏按钮不算数。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"

	"icebreaker/internal/auth"
	"icebreaker/internal/config"
	"icebreaker/internal/lottery"
	"icebreaker/internal/rbac"
	"icebreaker/internal/store"
)

// maxNameRunes 姓名长度上限（字符数，不是字节数）。只做技术性校验：
// 现场签到不设防，但也不接受把大屏撑爆或把库塞满的输入。
const maxNameRunes = 20

// maxNoteRunes 是部长介绍里"一句话"的长度上限。
const maxNoteRunes = 60

type Server struct {
	cfg    *config.Config
	auth   *auth.Auth
	st     *store.Store
	admins map[string]bool
	themes []string
	images []string
	ui     fs.FS
	log    *log.Logger
}

func New(cfg *config.Config, a *auth.Auth, st *store.Store, frontend fs.FS, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	admins := map[string]bool{}
	for _, sub := range cfg.Auth.AdminSubs {
		if sub = strings.TrimSpace(sub); sub != "" {
			admins[sub] = true
		}
	}
	themes, images := cfg.Draw.Themes, cfg.Draw.Images
	if len(themes) == 0 {
		themes = lottery.DefaultThemes
	}
	if len(images) == 0 {
		images = lottery.DefaultImages
	}
	return &Server{
		cfg: cfg, auth: a, st: st, admins: admins,
		themes: themes, images: images, ui: frontend, log: logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// 登录才能看的：大屏状态与名单、部长介绍列表。全站强制登录后，访客
	// 连大屏都进不去（user 及以上都能看）。
	mux.Handle("GET /api/v1/screen/state", s.gate(rbac.CapCheckin, s.handleScreenState))
	mux.Handle("GET /api/v1/screen/round", s.gate(rbac.CapCheckin, s.handleScreenRound))
	mux.Handle("GET /api/v1/directors", s.gate(rbac.CapCheckin, s.handleDirectors))
	// 签到二维码仍公开：它只是一张包含本站地址的图，且 <img> 带不了 Authorization 头。
	// 大屏本身要登录才看得到，二维码不会因此泄露任何东西。
	mux.HandleFunc("GET /api/v1/qr", s.handleQR)
	// 登录就能用的：签到、看自己是谁
	mux.Handle("POST /api/v1/checkin", s.user(s.handleCheckin))
	mux.Handle("GET /api/v1/me", s.user(s.handleMe))
	// 部长介绍：写要 minister 及以上
	mux.Handle("POST /api/v1/directors", s.gate(rbac.CapDirectors, s.handleSaveDirector))
	// /me 是自己删自己；/{sub} 给别人删（只有 admin 能删别人的）
	mux.Handle("DELETE /api/v1/directors/me", s.gate(rbac.CapDirectors, s.handleDeleteDirector))
	mux.Handle("DELETE /api/v1/directors/{sub}", s.gate(rbac.CapDirectors, s.handleDeleteDirector))

	// 鉴权入口（只有管理员用得到）
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)

	// 操作员侧：抽奖、大屏、名单
	mux.Handle("GET /api/v1/admin/state", s.gate(rbac.CapLottery, s.handleAdminState))
	mux.Handle("POST /api/v1/admin/checkin/open", s.gate(rbac.CapLottery, s.handleSignupOpen))
	mux.Handle("POST /api/v1/admin/checkin/close", s.gate(rbac.CapLottery, s.handleSignupClose))
	mux.Handle("POST /api/v1/admin/participants", s.gate(rbac.CapLottery, s.handleAddParticipant))
	mux.Handle("DELETE /api/v1/admin/participants/{id}", s.gate(rbac.CapLottery, s.handleDeleteParticipant))
	mux.Handle("POST /api/v1/admin/import", s.gate(rbac.CapLottery, s.handleImport))
	mux.Handle("POST /api/v1/admin/rounds", s.gate(rbac.CapLottery, s.handleDraw))
	mux.Handle("POST /api/v1/admin/rounds/current/cursor", s.gate(rbac.CapLottery, s.handleCursor))
	mux.Handle("POST /api/v1/admin/rounds/current/skip", s.gate(rbac.CapLottery, s.handleSkip))
	mux.Handle("POST /api/v1/admin/rounds/current/end", s.gate(rbac.CapLottery, s.handleEndRound))
	mux.Handle("POST /api/v1/admin/reset", s.gate(rbac.CapLottery, s.handleReset))
	mux.Handle("GET /api/v1/admin/export/participants", s.gate(rbac.CapLottery, s.handleExportParticipants))
	mux.Handle("GET /api/v1/admin/export/results", s.gate(rbac.CapLottery, s.handleExportResults))
	// 角色管理：只有 admin
	mux.Handle("GET /api/v1/admin/roles", s.gate(rbac.CapRoles, s.handleListRoles))
	mux.Handle("POST /api/v1/admin/roles", s.gate(rbac.CapRoles, s.handleSetRole))
	mux.Handle("DELETE /api/v1/admin/roles/{sub}", s.gate(rbac.CapRoles, s.handleDeleteRole))

	mux.Handle("GET /", s.staticHandler())

	return s.logRequests(mux)
}

// ---------- 中间件 ----------

// userFn 是"登录了就行"的接口（签到、看自己是谁）。
type userFn func(w http.ResponseWriter, r *http.Request, sub, name string)

// user 要求合法 HMAC token，但不看白名单：任何组织账号都能签到。
func (s *Server) user(h userFn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, name, err := s.auth.Verify(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r, sub, name)
	})
}

type adminFn func(w http.ResponseWriter, r *http.Request, sub, name string)

// role 解析一个账号的有效角色：
// 配置文件里的 admin_subs 是根管理员（永远 admin、不可撤销），
// 否则看库里的分配，再否则就是默认角色 user。
func (s *Server) role(sub string) string {
	if s.admins[sub] {
		return rbac.Admin
	}
	role, err := s.st.RoleOf(sub)
	if err != nil {
		s.log.Printf("读角色失败 sub=%s: %v", sub, err)
		return rbac.Default
	}
	if role == "" {
		return rbac.Default
	}
	return role
}

// gate 要求"登录 + 拥有某项能力"，能力不够 403。
func (s *Server) gate(cap rbac.Capability, h userFn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, name, err := s.auth.Verify(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		role := s.role(sub)
		if !rbac.Can(role, cap) {
			s.log.Printf("拒绝请求：sub=%s role=%s 缺少能力 %s（%s %s）", sub, role, cap, r.Method, r.URL.Path)
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "no_permission", "need": string(cap), "role": role,
			})
			return
		}
		h(w, r, sub, name)
	})
}

// logRequests 记访问日志。只记 r.URL.Path，绝不记 query
// （/auth/callback 的 code、回跳地址里的 token 都只在 query 里）。
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// ---------- 公开侧 ----------

type entry struct {
	Name        string `json:"name"`
	Theme       string `json:"theme"`
	Shumeiniang string `json:"shumeiniang"`
}

type roundState struct {
	No            int64  `json:"no"`
	Label         string `json:"label"`
	Count         int    `json:"count"`
	RevealedIndex int    `json:"revealedIndex"`
	SkipSeq       int64  `json:"skipSeq"`
}

// handleCheckin 签到。姓名取组织账号里的真名，不接受前端传名字；
// 一个 sub 只会有一条记录（重复签到返回已签到，不报错）。
func (s *Server) handleCheckin(w http.ResponseWriter, r *http.Request, sub, name string) {
	st, err := s.st.State()
	if err != nil {
		s.fail(w, err)
		return
	}
	if !st.SignupOpen {
		writeError(w, http.StatusConflict, "signup_closed")
		return
	}

	count, err := s.st.CountParticipants()
	if err != nil {
		s.fail(w, err)
		return
	}

	// 已经签过：原样告诉他第几位，不重复入列，也不报错（刷新/重复点都走这里）。
	if p, ok, err := s.st.ParticipantBySub(sub); err != nil {
		s.fail(w, err)
		return
	} else if ok {
		position, err := s.st.Position(p.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"name": p.Name, "position": position, "count": count, "already": true,
		})
		return
	}

	p, err := s.st.AddParticipant(sub, claimName(name, sub), lottery.Pick(s.themes),
		lottery.Pick(s.images), "checkin", time.Now().UnixMilli())
	if err != nil {
		// 并发双击：唯一索引挡下第二次，按"已签到"回。
		if store.IsDuplicate(err) {
			writeJSON(w, http.StatusOK, map[string]any{
				"name": claimName(name, sub), "count": count, "already": true,
			})
			return
		}
		s.fail(w, err)
		return
	}
	count, err = s.st.CountParticipants()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("签到：%s（第 %d 位，sub=%s）", p.Name, count, sub)
	writeJSON(w, http.StatusOK, map[string]any{
		"name": p.Name, "position": count, "count": count, "already": false,
	})
}

// handleMe 告诉前端"你是谁、什么角色、能干什么"，省得前端去解 token。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, sub, name string) {
	checkedIn := false
	if _, ok, err := s.st.ParticipantBySub(sub); err != nil {
		s.fail(w, err)
		return
	} else {
		checkedIn = ok
	}
	role := s.role(sub)
	writeJSON(w, http.StatusOK, map[string]any{
		"sub": sub, "name": claimName(name, sub), "role": role, "checkedIn": checkedIn,
		// 根管理员（配置里写死的）在管理台里要能看出来
		"rootAdmin": s.admins[sub],
		"can": map[string]bool{
			"checkin":   rbac.Can(role, rbac.CapCheckin),
			"directors": rbac.Can(role, rbac.CapDirectors),
			"lottery":   rbac.Can(role, rbac.CapLottery),
			"roles":     rbac.Can(role, rbac.CapRoles),
		},
	})
}

// claimName 兜住"组织账号里名字为空"的情况：退回 sub，并且不让超长名字把大屏撑爆。
func claimName(name, sub string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return sub
	}
	if runes := []rune(name); len(runes) > maxNameRunes {
		return string(runes[:maxNameRunes])
	}
	return name
}

func (s *Server) handleScreenState(w http.ResponseWriter, r *http.Request, _, _ string) {
	st, err := s.st.State()
	if err != nil {
		s.fail(w, err)
		return
	}
	count, err := s.st.CountParticipants()
	if err != nil {
		s.fail(w, err)
		return
	}
	out := struct {
		SignupOpen       bool        `json:"signupOpen"`
		ParticipantCount int         `json:"participantCount"`
		Round            *roundState `json:"round"`
	}{SignupOpen: st.SignupOpen, ParticipantCount: count}

	if st.CurrentRound > 0 {
		rd, ok, err := s.st.RoundByNo(st.CurrentRound)
		if err != nil {
			s.fail(w, err)
			return
		}
		if ok {
			out.Round = &roundState{
				No: rd.No, Label: rd.Label, Count: rd.DrawCount,
				RevealedIndex: rd.RevealedIndex, SkipSeq: rd.SkipSeq,
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleScreenRound 大屏开轮时一次性预载本轮完整名单；动画与网络无关。
// 不含 seed / 快照 hash —— 那是管理侧的复核材料，不往公开接口上放。
func (s *Server) handleScreenRound(w http.ResponseWriter, r *http.Request, _, _ string) {
	st, err := s.st.State()
	if err != nil {
		s.fail(w, err)
		return
	}
	if st.CurrentRound == 0 {
		writeError(w, http.StatusNotFound, "no_round")
		return
	}
	rd, ok, err := s.st.RoundByNo(st.CurrentRound)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no_round")
		return
	}
	winners, err := s.st.Winners(rd.No)
	if err != nil {
		s.fail(w, err)
		return
	}
	entries := make([]entry, 0, len(winners))
	for _, x := range winners {
		entries = append(entries, entry{Name: x.Name, Theme: x.Theme, Shumeiniang: x.Image})
	}
	writeJSON(w, http.StatusOK, struct {
		No            int64   `json:"no"`
		Label         string  `json:"label"`
		Count         int     `json:"count"`
		RevealedIndex int     `json:"revealedIndex"`
		SkipSeq       int64   `json:"skipSeq"`
		Entries       []entry `json:"entries"`
	}{rd.No, rd.Label, rd.DrawCount, rd.RevealedIndex, rd.SkipSeq, entries})
}

// handleQR 现场签到二维码：前端把签到页地址传进来，服务端出 PNG。
func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	data := r.URL.Query().Get("data")
	if data == "" || len(data) > 512 {
		writeError(w, http.StatusBadRequest, "bad_data")
		return
	}
	png, err := qrcode.Encode(data, qrcode.Medium, 512)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=600")
	_, _ = w.Write(png)
}

// ---------- 管理侧 ----------

type adminParticipant struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Sub       string `json:"sub"`
	Role      string `json:"role"`
	Source    string `json:"source"`
	CreatedAt int64  `json:"createdAt"`
}

type adminRound struct {
	No            int64   `json:"no"`
	Label         string  `json:"label"`
	Count         int     `json:"count"`
	RemoveWinners bool    `json:"removeWinners"`
	Seed          string  `json:"seed"`
	SnapshotHash  string  `json:"snapshotHash"`
	RevealedIndex int     `json:"revealedIndex"`
	SkipSeq       int64   `json:"skipSeq"`
	CreatedAt     int64   `json:"createdAt"`
	Winners       []entry `json:"winners"`
}

func (s *Server) handleAdminState(w http.ResponseWriter, r *http.Request, sub, name string) {
	st, err := s.st.State()
	if err != nil {
		s.fail(w, err)
		return
	}
	people, err := s.st.Participants()
	if err != nil {
		s.fail(w, err)
		return
	}
	rounds, err := s.st.Rounds()
	if err != nil {
		s.fail(w, err)
		return
	}
	pool, err := s.st.EligiblePool()
	if err != nil {
		s.fail(w, err)
		return
	}

	participants := make([]adminParticipant, 0, len(people))
	for _, p := range people {
		role := ""
		if p.Sub != "" {
			role = s.role(p.Sub)
		}
		participants = append(participants, adminParticipant{
			ID: p.ID, Name: p.Name, Sub: p.Sub, Role: role,
			Source: p.Source, CreatedAt: p.CreatedAt,
		})
	}
	history := make([]adminRound, 0, len(rounds))
	var current *adminRound
	for _, rd := range rounds {
		winners, err := s.st.Winners(rd.No)
		if err != nil {
			s.fail(w, err)
			return
		}
		entries := make([]entry, 0, len(winners))
		for _, x := range winners {
			entries = append(entries, entry{Name: x.Name, Theme: x.Theme, Shumeiniang: x.Image})
		}
		ar := adminRound{
			No: rd.No, Label: rd.Label, Count: rd.DrawCount, RemoveWinners: rd.RemoveWinners,
			Seed: rd.Seed, SnapshotHash: rd.SnapshotHash, RevealedIndex: rd.RevealedIndex,
			SkipSeq: rd.SkipSeq, CreatedAt: rd.CreatedAt, Winners: entries,
		}
		history = append(history, ar)
		if rd.No == st.CurrentRound {
			cp := ar
			current = &cp
		}
	}

	writeJSON(w, http.StatusOK, struct {
		Sub              string             `json:"sub"`
		Name             string             `json:"name"`
		SignupOpen       bool               `json:"signupOpen"`
		ParticipantCount int                `json:"participantCount"`
		EligibleCount    int                `json:"eligibleCount"`
		Participants     []adminParticipant `json:"participants"`
		Current          *adminRound        `json:"current"`
		Rounds           []adminRound       `json:"rounds"`
	}{
		Sub: sub, Name: name,
		SignupOpen:       st.SignupOpen,
		ParticipantCount: len(people),
		EligibleCount:    len(pool),
		Participants:     participants,
		Current:          current,
		Rounds:           history,
	})
}

func (s *Server) handleSignupOpen(w http.ResponseWriter, r *http.Request, _, _ string) {
	if err := s.st.SetSignupOpen(true); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("签到开关：开")
	writeJSON(w, http.StatusOK, map[string]bool{"signupOpen": true})
}

func (s *Server) handleSignupClose(w http.ResponseWriter, r *http.Request, _, _ string) {
	if err := s.st.SetSignupOpen(false); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("签到开关：关")
	writeJSON(w, http.StatusOK, map[string]bool{"signupOpen": false})
}

func (s *Server) handleAddParticipant(w http.ResponseWriter, r *http.Request, _, _ string) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, ok := cleanName(w, req.Name)
	if !ok {
		return
	}
	if _, err := s.st.AddParticipant("", name, lottery.Pick(s.themes), lottery.Pick(s.images), "manual", time.Now().UnixMilli()); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("手动加人：%s", name)
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (s *Server) handleDeleteParticipant(w http.ResponseWriter, r *http.Request, _, _ string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_id")
		return
	}
	ok, err := s.st.DeleteParticipant(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.log.Printf("删人：id=%d", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// handleImport 文件导入：前端把 .txt/.json 解析成姓名数组传上来，
// mode=append 追加、mode=replace 覆盖（覆盖会连轮次与中奖记录一起清掉，
// 否则旧结果会挂在已经不存在的名单上）。
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request, _, _ string) {
	var req struct {
		Names []string `json:"names"`
		Mode  string   `json:"mode"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Mode == "" {
		req.Mode = "append"
	}
	if req.Mode != "append" && req.Mode != "replace" {
		writeError(w, http.StatusBadRequest, "bad_mode")
		return
	}
	if req.Mode == "replace" {
		if err := s.st.Reset("all"); err != nil {
			s.fail(w, err)
			return
		}
	}
	added := 0
	for _, raw := range req.Names {
		name := strings.TrimSpace(raw)
		if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
			continue
		}
		if _, err := s.st.AddParticipant("", name, lottery.Pick(s.themes), lottery.Pick(s.images), "import", time.Now().UnixMilli()); err != nil {
			s.fail(w, err)
			return
		}
		added++
	}
	count, err := s.st.CountParticipants()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("导入名单：mode=%s 加入 %d 人，当前共 %d 人", req.Mode, added, count)
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "count": count})
}

func (s *Server) handleDraw(w http.ResponseWriter, r *http.Request, _, _ string) {
	var req struct {
		Count         int    `json:"count"`
		Label         string `json:"label"`
		RemoveWinners *bool  `json:"removeWinners"`
		Seed          string `json:"seed"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Count < 1 {
		writeError(w, http.StatusBadRequest, "bad_count")
		return
	}

	pool, err := s.st.EligiblePool()
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(pool) == 0 {
		writeError(w, http.StatusConflict, "pool_empty")
		return
	}
	if req.Count > len(pool) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "not_enough_participants", "available": len(pool),
		})
		return
	}

	remove := true
	if req.RemoveWinners != nil {
		remove = *req.RemoveWinners
	}
	no, err := s.st.NextRoundNo()
	if err != nil {
		s.fail(w, err)
		return
	}

	seed := strings.TrimSpace(req.Seed)
	if seed == "" {
		if seed, err = lottery.NewSeed(); err != nil {
			s.fail(w, err)
			return
		}
	}

	ids := make([]int64, len(pool))
	for i, p := range pool {
		ids[i] = p.ID
	}
	snapshot, hash := lottery.Snapshot(no, seed, ids)
	order := lottery.Shuffle(ids, seed)
	winners := make([]store.Winner, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		p := pool[order[i]]
		winners = append(winners, store.Winner{
			ParticipantID: p.ID, Name: p.Name, Theme: p.Theme, Image: p.Image,
		})
	}

	rd := store.Round{
		No: no, Label: strings.TrimSpace(req.Label), DrawCount: req.Count,
		RemoveWinners: remove, Seed: seed, Snapshot: snapshot, SnapshotHash: hash,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.st.CreateRound(rd, winners); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("发起抽奖：round=%d 从 %d 人里抽 %d 人 remove_winners=%v seed=%s hash=%s",
		no, len(pool), req.Count, remove, seed, hash[:12])
	writeJSON(w, http.StatusOK, map[string]any{
		"no": no, "count": req.Count, "seed": seed, "snapshotHash": hash,
	})
}

// currentRound 取当前轮次，没有就写 409 返回 false。
func (s *Server) currentRound(w http.ResponseWriter) (store.Round, bool) {
	st, err := s.st.State()
	if err != nil {
		s.fail(w, err)
		return store.Round{}, false
	}
	if st.CurrentRound == 0 {
		writeError(w, http.StatusConflict, "no_current_round")
		return store.Round{}, false
	}
	rd, ok, err := s.st.RoundByNo(st.CurrentRound)
	if err != nil {
		s.fail(w, err)
		return store.Round{}, false
	}
	if !ok {
		writeError(w, http.StatusConflict, "no_current_round")
		return store.Round{}, false
	}
	return rd, true
}

func (s *Server) handleCursor(w http.ResponseWriter, r *http.Request, _, _ string) {
	rd, ok := s.currentRound(w)
	if !ok {
		return
	}
	var req struct {
		RevealedIndex *int `json:"revealedIndex"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RevealedIndex == nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.st.SetCursor(rd.No, *req.RevealedIndex); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revealedIndex": *req.RevealedIndex})
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request, _, _ string) {
	rd, ok := s.currentRound(w)
	if !ok {
		return
	}
	if err := s.st.BumpSkip(rd.No); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skipSeq": rd.SkipSeq + 1})
}

func (s *Server) handleEndRound(w http.ResponseWriter, r *http.Request, _, _ string) {
	if err := s.st.EndRound(); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("结束本轮，大屏回到待机")
	writeJSON(w, http.StatusOK, map[string]string{"state": "idle"})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request, _, _ string) {
	var req struct {
		Scope string `json:"scope"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Scope != "winners" && req.Scope != "all" {
		writeError(w, http.StatusBadRequest, "bad_scope")
		return
	}
	if err := s.st.Reset(req.Scope); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("重置：scope=%s", req.Scope)
	writeJSON(w, http.StatusOK, map[string]string{"reset": req.Scope})
}

func (s *Server) handleExportParticipants(w http.ResponseWriter, r *http.Request, _, _ string) {
	people, err := s.st.Participants()
	if err != nil {
		s.fail(w, err)
		return
	}
	var b strings.Builder
	for _, p := range people {
		b.WriteString(p.Name)
		b.WriteByte('\n')
	}
	writeAttachment(w, "participants.txt", b.String())
}

func (s *Server) handleExportResults(w http.ResponseWriter, r *http.Request, _, _ string) {
	rounds, err := s.st.Rounds()
	if err != nil {
		s.fail(w, err)
		return
	}
	var b strings.Builder
	b.WriteString("轮次\t标签\t名次\t姓名\t种子\n")
	for _, rd := range rounds {
		winners, err := s.st.Winners(rd.No)
		if err != nil {
			s.fail(w, err)
			return
		}
		for i, x := range winners {
			b.WriteString(strconv.FormatInt(rd.No, 10))
			b.WriteByte('\t')
			b.WriteString(rd.Label)
			b.WriteByte('\t')
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteByte('\t')
			b.WriteString(x.Name)
			b.WriteByte('\t')
			b.WriteString(rd.Seed)
			b.WriteByte('\n')
		}
	}
	writeAttachment(w, "results.txt", b.String())
}

// ---------- 鉴权入口 ----------

// ---------- 部长介绍 ----------

// handleDirectors 列表：介绍页与大屏都要读；全站强制登录后，user 及以上都能看。
func (s *Server) handleDirectors(w http.ResponseWriter, r *http.Request, _, _ string) {
	list, err := s.st.Directors()
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, map[string]any{
			"sub": d.Sub, "name": d.Name, "theme": d.Theme,
			"shumeiniang": d.Image, "note": d.Note,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"directors": out})
}

// handleSaveDirector 部长写自己的介绍（一个人一条，重复提交即修改）；
// admin 可以带 sub 指定替谁写。姓名一律取组织账号里的真名，素材名限定在已知集合里。
func (s *Server) handleSaveDirector(w http.ResponseWriter, r *http.Request, sub, name string) {
	var req struct {
		Sub         string `json:"sub"`
		Theme       string `json:"theme"`
		Shumeiniang string `json:"shumeiniang"`
		Note        string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	target := strings.TrimSpace(req.Sub)
	if target == "" {
		target = sub
	}
	if target != sub && s.role(sub) != rbac.Admin {
		// 只有 admin 能代别人写
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "no_permission", "need": string(rbac.CapRoles), "role": s.role(sub),
		})
		return
	}
	targetName, err := s.directorTargetName(target, sub, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	note := strings.TrimSpace(req.Note)
	if runes := []rune(note); len(runes) > maxNoteRunes {
		note = string(runes[:maxNoteRunes])
	}
	d := store.Director{
		Sub:   target,
		Name:  targetName,
		Theme: pickOne(req.Theme, s.themes, lottery.DefaultThemes),
		Image: pickOne(req.Shumeiniang, s.images, lottery.AllImages),
		Note:  note,
	}
	if err := s.st.UpsertDirector(d, time.Now().UnixMilli()); err != nil {
		s.fail(w, err)
		return
	}
	by := "本人"
	if target != sub {
		by = "admin 代为"
	}
	s.log.Printf("部长介绍：%s（sub=%s，%s）写入", d.Name, target, by)
	writeJSON(w, http.StatusOK, map[string]any{
		"name": d.Name, "theme": d.Theme, "shumeiniang": d.Image, "note": d.Note,
	})
}

// directorTargetName 决定介绍上显示的姓名：自己用 token 里的真名；替别人写时
// 从签到名单里取他的真名（没签到过就沿用他原有介绍里的名字），不接受前端传名字。
func (s *Server) directorTargetName(target, callerSub, callerName string) (string, error) {
	if target == callerSub {
		return claimName(callerName, callerSub), nil
	}
	if p, ok, err := s.st.ParticipantBySub(target); err != nil {
		return "", err
	} else if ok {
		return p.Name, nil
	}
	list, err := s.st.Directors()
	if err != nil {
		return "", err
	}
	for _, d := range list {
		if d.Sub == target {
			return d.Name, nil
		}
	}
	return "", errUnknownSub
}

// handleDeleteDirector 删介绍：/me 删自己的；/{sub} 删别人的要 admin。
func (s *Server) handleDeleteDirector(w http.ResponseWriter, r *http.Request, sub, _ string) {
	target := r.PathValue("sub")
	if target == "" || target == "me" {
		target = sub
	}
	if target != sub && s.role(sub) != rbac.Admin {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "no_permission", "need": string(rbac.CapRoles), "role": s.role(sub),
		})
		return
	}
	ok, err := s.st.DeleteDirectorBySub(target)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.log.Printf("部长介绍：sub=%s 的介绍被 sub=%s 删除", target, sub)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "sub": target})
}

// pickOne：前端传来的素材名合法就用它，否则从候选里随机挑一个。
// 候选列表按顺序找，第一个非空的当兜底池。
func pickOne(v string, lists ...[]string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		for _, list := range lists {
			for _, x := range list {
				if x == v {
					return v
				}
			}
		}
	}
	for _, list := range lists {
		if len(list) > 0 {
			return lottery.Pick(list)
		}
	}
	return ""
}

// ---------- 角色管理（只有 admin） ----------

// handleListRoles 给出"配置文件里的根管理员"和"库里显式分配的角色"。
// 管理台把它和签到名单拼起来，就能直接给人点角色。
func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request, _, _ string) {
	rows, err := s.st.Roles()
	if err != nil {
		s.fail(w, err)
		return
	}
	assigned := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		assigned = append(assigned, map[string]any{
			"sub": row.Sub, "role": row.Role, "name": row.Name, "updatedAt": row.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"roles":        rbac.All,
		"configAdmins": s.cfg.Auth.AdminSubs,
		"assigned":     assigned,
	})
}

func (s *Server) handleSetRole(w http.ResponseWriter, r *http.Request, _, _ string) {
	var req struct {
		Sub  string `json:"sub"`
		Role string `json:"role"`
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, role := strings.TrimSpace(req.Sub), strings.TrimSpace(req.Role)
	if sub == "" {
		writeError(w, http.StatusBadRequest, "bad_sub")
		return
	}
	if !rbac.Valid(role) {
		writeError(w, http.StatusBadRequest, "bad_role")
		return
	}
	if s.admins[sub] {
		// 配置文件里的根管理员是"最后一个保险"，库里的操作动不了它。
		writeError(w, http.StatusConflict, "config_admin_immutable")
		return
	}
	if err := s.st.SetRole(sub, role, strings.TrimSpace(req.Name), time.Now().UnixMilli()); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Printf("角色分配：sub=%s → %s", sub, role)
	writeJSON(w, http.StatusOK, map[string]string{"sub": sub, "role": role})
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request, _, _ string) {
	sub := r.PathValue("sub")
	if sub == "" {
		writeError(w, http.StatusBadRequest, "bad_sub")
		return
	}
	if s.admins[sub] {
		writeError(w, http.StatusConflict, "config_admin_immutable")
		return
	}
	// 别把最后一个 admin 撤了，否则没人能再分配角色（根管理员除外，但它也提醒一下）。
	if role, err := s.st.RoleOf(sub); err != nil {
		s.fail(w, err)
		return
	} else if role == rbac.Admin {
		n, err := s.st.CountDBAdmins()
		if err != nil {
			s.fail(w, err)
			return
		}
		if n <= 1 && len(s.cfg.Auth.AdminSubs) == 0 {
			writeError(w, http.StatusConflict, "last_admin")
			return
		}
	}
	ok, err := s.st.DeleteRole(sub)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	s.log.Printf("角色撤销：sub=%s（回到默认 user）", sub)
	writeJSON(w, http.StatusOK, map[string]string{"sub": sub, "role": rbac.Default})
}

// errUnknownSub 出现在"admin 要代某人写介绍，但这个人既没签到、也没有已有介绍"时：
// 姓名必须是组织账号里的真名，编不出来就只能拒绝。
var errUnknownSub = errors.New("unknown_sub")

// Warmup 在后台预热 OIDC 发现文档。
//
// provider 是懒加载的（Keycloak 抖一下也要能开服），代价是重启后第一次登录
// 得现场等十几秒。启动后主动拉一次，就把这段等待挪到没人看的时候。
// 失败只记一行日志、不影响服务——真正登录时会再试一次。
func (s *Server) Warmup(ctx context.Context) {
	start := time.Now()
	if _, err := s.auth.LoginURL(ctx, "warmup", "warmup"); err != nil {
		s.log.Printf("预热 OIDC 失败（不影响启动，登录时会重试）: %v", err)
		return
	}
	s.log.Printf("OIDC 发现文档就绪（耗时 %s），第一次登录不用等", time.Since(start).Round(time.Millisecond))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, nonce := s.auth.NewState(sanitizeNext(r.URL.Query().Get("next")))
	url, err := s.auth.LoginURL(r.Context(), state, nonce)
	if err != nil {
		s.log.Printf("生成登录地址失败: %v", err)
		http.Error(w, "Keycloak 暂不可用，请稍后重试", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	// 回跳地址的 query 里有 code 与 state，别把它泄给下一个站点。
	w.Header().Set("Referrer-Policy", "no-referrer")

	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "Keycloak 返回错误: "+e, http.StatusBadRequest)
		return
	}
	nonce, next, err := s.auth.CheckState(q.Get("state"))
	if err != nil {
		http.Error(w, "state 校验失败（请从本站重新发起登录）", http.StatusBadRequest)
		return
	}
	sub, name, err := s.auth.Exchange(r.Context(), q.Get("code"), nonce)
	if err != nil {
		s.log.Printf("登录失败: %v", err)
		http.Error(w, "登录失败，请重试", http.StatusUnauthorized)
		return
	}
	// 任何人都能登录：普通部员登录是为了签到，只有管理接口才看白名单。
	token := s.auth.Issue(sub, name, time.Now())
	s.log.Printf("登录成功：%s（sub=%s，管理员=%v）", name, sub, s.admins[sub])
	http.Redirect(w, r, tokenRedirect(next, token), http.StatusFound)
}

// sanitizeNext 只放行本站相对路径，挡住开放重定向。
func sanitizeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.ContainsAny(next, "|") {
		return "/"
	}
	return next
}

// tokenRedirect 把 token 塞进 query，且必须落在 hash 之前：
// 前端是 hash 路由，/#/checkin 要变成 /?token=xxx#/checkin。
func tokenRedirect(next, token string) string {
	pathPart, hash := next, ""
	if i := strings.Index(next, "#"); i >= 0 {
		pathPart, hash = next[:i], next[i:]
	}
	sep := "?"
	if strings.Contains(pathPart, "?") {
		sep = "&"
	}
	return pathPart + sep + "token=" + token + hash
}

// ---------- 静态托管 ----------

// 前端是 hash 路由，只有 / 需要落到 index.html，其余都是真实文件。
func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		// 没构建前端时（仓库里只有 dist/.gitkeep），给一句能看懂的话，
		// 而不是 404 让运维以为后端没起来。
		if name == "index.html" {
			if _, err := fs.Stat(s.ui, name); err != nil {
				writeNotBuilt(w)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		fi, err := fs.Stat(s.ui, name)
		if err != nil || fi.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
				writeError(w, http.StatusNotFound, "not_found")
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, s.ui, name)
	})
}

// ---------- 输出 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeAttachment(w http.ResponseWriter, filename, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = io.WriteString(w, body)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Printf("内部错误: %v", err)
	writeError(w, http.StatusInternalServerError, "internal_error")
}

// writeNotBuilt 是"前端还没构建"的提示页：二进制能起来、接口能用，
// 但磁盘上只有 dist/.gitkeep。看到这句话去跑 npm run build 就行。
func writeNotBuilt(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>前端未构建</title></head>
<body style="font-family:system-ui,sans-serif;background:#111;color:#eee;padding:2rem;line-height:1.8">
<h1>前端还没构建</h1>
<p>后端已经起来了（接口与 <code>/auth/*</code> 正常），但二进制里没有前端文件。</p>
<p>在 <code>introduction/</code> 下跑 <code>npm install &amp;&amp; npm run build</code>，
再重新 <code>go build ./cmd/icebreaker</code> 即可。</p>
</body></html>`)
}

// decodeJSON 读 JSON 体（上限 64KB），失败时已写回 400。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return false
	}
	return true
}

// cleanName 校验并清理姓名，失败时已写回 400。
func cleanName(w http.ResponseWriter, raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		writeError(w, http.StatusBadRequest, "bad_name")
		return "", false
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		writeError(w, http.StatusBadRequest, "name_too_long")
		return "", false
	}
	return name, true
}
