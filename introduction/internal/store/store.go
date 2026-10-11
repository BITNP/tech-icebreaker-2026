// Package store 是 SQLite 里的四张表：签到名单、轮次、中奖记录、运行态。
//
// 名单与结果都要活过后端重启（现场崩一次不能让大家重新签到），所以这里
// 全部落盘。中奖记录里冗余一份中奖当时的姓名/主题/树莓娘图：结果一旦产生
// 就不该再被后续改动（改名单、删人）牵动。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS participants (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  sub         TEXT,                                 -- 组织账号唯一标识；手动加的人为空
  name        TEXT    NOT NULL,
  theme       TEXT    NOT NULL DEFAULT '',
  shumeiniang TEXT    NOT NULL DEFAULT '',
  source      TEXT    NOT NULL DEFAULT 'checkin',  -- checkin | manual | import
  created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rounds (
  no             INTEGER PRIMARY KEY,               -- 1..N，自增
  label          TEXT    NOT NULL DEFAULT '',       -- "三等奖"/"特别奖"
  draw_count     INTEGER NOT NULL,
  remove_winners INTEGER NOT NULL,                  -- 中奖者是否退出后续轮次
  seed           TEXT    NOT NULL,
  snapshot       TEXT    NOT NULL,                  -- 规范化参与名单（复核用）
  snapshot_hash  TEXT    NOT NULL,
  revealed_index INTEGER NOT NULL DEFAULT -1,       -- -1 = 还没揭晓任何人
  skip_seq       INTEGER NOT NULL DEFAULT 0,        -- 管理端"跳过动画"信号
  created_at     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS draws (
  round_no       INTEGER NOT NULL,
  position       INTEGER NOT NULL,                  -- 1..draw_count，决定大屏顺序
  participant_id INTEGER NOT NULL,
  name           TEXT    NOT NULL,                  -- 中奖当时的展示字段（快照）
  theme          TEXT    NOT NULL,
  shumeiniang    TEXT    NOT NULL,
  PRIMARY KEY (round_no, position)
);

CREATE TABLE IF NOT EXISTS state (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  signup_open   INTEGER NOT NULL DEFAULT 1,         -- 默认开：现场忘开比误开放代价大
  current_round INTEGER,                            -- NULL = 待机
  next_round    INTEGER NOT NULL DEFAULT 1          -- 单调递增，重置也不回退
);
`

type Store struct{ db *sql.DB }

// Participant 一行签到记录。允许重名（不做防伪，按主键区分人）。
type Participant struct {
	ID        int64
	Sub       string
	Name      string
	Theme     string
	Image     string
	Source    string
	CreatedAt int64
}

// Winner 一次中奖记录，含中奖当时的展示字段。
type Winner struct {
	ParticipantID int64
	Name          string
	Theme         string
	Image         string
}

// Round 一轮抽奖。
type Round struct {
	No            int64
	Label         string
	DrawCount     int
	RemoveWinners bool
	Seed          string
	Snapshot      string
	SnapshotHash  string
	RevealedIndex int
	SkipSeq       int64
	CreatedAt     int64
}

// State 运行态：签到开关与当前轮次（0 = 待机）。
type State struct {
	SignupOpen   bool
	CurrentRound int64
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", path, err)
	}
	// 写入是低频事件（签到、开轮），单写连接一次性规避 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表: %w", err)
	}
	if _, err := db.Exec(rolesSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建角色表: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("迁移: %w", err)
	}
	// 运行态那一行放在迁移之后插入：老库里 state 已经存在但没有 next_round 列，
	// 先插就会撞上"no column named next_round"。
	if _, err := db.Exec(`INSERT OR IGNORE INTO state (id) VALUES (1)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化 state: %w", err)
	}
	return s, nil
}

// migrate 给早先建的库补列。现场拉到新二进制时不用删库重来。
func (s *Store) migrate() error {
	ok, err := s.hasColumn("state", "next_round")
	if err != nil {
		return err
	}
	if !ok {
		if _, err := s.db.Exec(`ALTER TABLE state ADD COLUMN next_round INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
		// 已经有轮次的话，计数抬到 max+1，免得新轮和旧轮撞号。
		if _, err := s.db.Exec(
			`UPDATE state SET next_round = (SELECT COALESCE(MAX(no), 0) + 1 FROM rounds) WHERE id = 1`); err != nil {
			return err
		}
	}

	// 老库里 participants 没有 sub，补上；唯一索引必须等补完列再建。
	if ok, err = s.hasColumn("participants", "sub"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.Exec(`ALTER TABLE participants ADD COLUMN sub TEXT`); err != nil {
			return err
		}
	}
	// 「一个组织账号只能有一条签到」就靠这个索引兜底（NULL 不参与唯一性，
	// 手动加的人、老数据都是 NULL，互不冲突）。
	_, err = s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_participants_sub ON participants(sub)`)
	return err
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	return n > 0, err
}

func (s *Store) Close() error { return s.db.Close() }

// ---------- 名单 ----------

// AddParticipant 落一条签到。sub 为空表示手动加人/文件导入。
func (s *Store) AddParticipant(sub, name, theme, image, source string, now int64) (Participant, error) {
	p := Participant{Sub: sub, Name: name, Theme: theme, Image: image, Source: source, CreatedAt: now}
	res, err := s.db.Exec(
		`INSERT INTO participants (sub, name, theme, shumeiniang, source, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(sub), name, theme, image, source, now,
	)
	if err != nil {
		return Participant{}, err
	}
	p.ID, err = res.LastInsertId()
	return p, err
}

// nullIfEmpty 让空 sub 落成 NULL：唯一索引放行多个 NULL，但空串会被当成同一个值。
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ParticipantBySub 按组织账号查签到记录。sub 为空时永远查不到。
func (s *Store) ParticipantBySub(sub string) (Participant, bool, error) {
	if sub == "" {
		return Participant{}, false, nil
	}
	p, err := scanParticipant(s.db.QueryRow(
		`SELECT id, COALESCE(sub, ''), name, theme, shumeiniang, source, created_at
		 FROM participants WHERE sub = ?`, sub))
	if err == sql.ErrNoRows {
		return Participant{}, false, nil
	}
	if err != nil {
		return Participant{}, false, err
	}
	return p, true, nil
}

// Position 这个人在名单里排第几（按签到先后，1 起）。
func (s *Store) Position(id int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM participants WHERE id <= ?`, id).Scan(&n)
	return n, err
}

// IsDuplicate 判断是不是撞了唯一索引（同一账号并发重复签到）。
func IsDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func scanParticipant(r rowScanner) (Participant, error) {
	var p Participant
	err := r.Scan(&p.ID, &p.Sub, &p.Name, &p.Theme, &p.Image, &p.Source, &p.CreatedAt)
	return p, err
}

func (s *Store) CountParticipants() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM participants`).Scan(&n)
	return n, err
}

func (s *Store) Participants() ([]Participant, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(sub, ''), name, theme, shumeiniang, source, created_at
		 FROM participants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Participant{}
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteParticipant 删一个人；历史中奖记录不动（draws 里存的是快照）。
func (s *Store) DeleteParticipant(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM participants WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ClearParticipants 清空名单（文件导入 mode=replace 用）。
func (s *Store) ClearParticipants() error {
	_, err := s.db.Exec(`DELETE FROM participants`)
	return err
}

// EligiblePool 候选池：所有签到者，减去"在 remove_winners 的轮次里中过奖"的人。
// 顺序按 id 升序 —— 这是抽奖快照的规范顺序，保证同样名单得到同样 hash。
func (s *Store) EligiblePool() ([]Participant, error) {
	rows, err := s.db.Query(`
		SELECT p.id, COALESCE(p.sub, ''), p.name, p.theme, p.shumeiniang, p.source, p.created_at
		FROM participants p
		WHERE NOT EXISTS (
			SELECT 1 FROM draws d JOIN rounds r ON r.no = d.round_no
			WHERE d.participant_id = p.id AND r.remove_winners = 1
		)
		ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Participant{}
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- 运行态 ----------

func (s *Store) State() (State, error) {
	var st State
	var cur sql.NullInt64
	var open int
	if err := s.db.QueryRow(`SELECT signup_open, current_round FROM state WHERE id = 1`).
		Scan(&open, &cur); err != nil {
		return State{}, err
	}
	st.SignupOpen = open != 0
	if cur.Valid {
		st.CurrentRound = cur.Int64
	}
	return st, nil
}

func (s *Store) SetSignupOpen(open bool) error {
	v := 0
	if open {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE state SET signup_open = ? WHERE id = 1`, v)
	return err
}

// ---------- 轮次 ----------

const roundCols = `no, label, draw_count, remove_winners, seed, snapshot, snapshot_hash,
	revealed_index, skip_seq, created_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanRound(r rowScanner) (Round, error) {
	var rd Round
	err := r.Scan(&rd.No, &rd.Label, &rd.DrawCount, &rd.RemoveWinners, &rd.Seed,
		&rd.Snapshot, &rd.SnapshotHash, &rd.RevealedIndex, &rd.SkipSeq, &rd.CreatedAt)
	return rd, err
}

// NextRoundNo 取下一个轮次号。它来自单调计数器而不是 MAX(no)：
// 重置中奖会删掉所有轮次，若按 MAX 续号就会从 1 重来，大屏那个"轮次号变了
// 就重新拉名单"的判断会失效，于是接着播上一轮的人。
func (s *Store) NextRoundNo() (int64, error) {
	var no int64
	err := s.db.QueryRow(`SELECT next_round FROM state WHERE id = 1`).Scan(&no)
	return no, err
}

// RoundByNo 取一轮；不存在返回 false。
func (s *Store) RoundByNo(no int64) (Round, bool, error) {
	rd, err := scanRound(s.db.QueryRow(`SELECT `+roundCols+` FROM rounds WHERE no = ?`, no))
	if err == sql.ErrNoRows {
		return Round{}, false, nil
	}
	if err != nil {
		return Round{}, false, err
	}
	return rd, true, nil
}

func (s *Store) Rounds() ([]Round, error) {
	rows, err := s.db.Query(`SELECT ` + roundCols + ` FROM rounds ORDER BY no`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Round{}
	for rows.Next() {
		rd, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rd)
	}
	return out, rows.Err()
}

// CreateRound 落一轮中奖结果，并把它设为当前轮次（游标归位、跳过信号清零）。
func (s *Store) CreateRound(rd Round, winners []Winner) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO rounds (no, label, draw_count, remove_winners, seed, snapshot,
		   snapshot_hash, revealed_index, skip_seq, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, -1, 0, ?)`,
		rd.No, rd.Label, rd.DrawCount, rd.RemoveWinners, rd.Seed, rd.Snapshot,
		rd.SnapshotHash, rd.CreatedAt,
	); err != nil {
		return err
	}
	for i, w := range winners {
		if _, err := tx.Exec(
			`INSERT INTO draws (round_no, position, participant_id, name, theme, shumeiniang)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			rd.No, i+1, w.ParticipantID, w.Name, w.Theme, w.Image,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`UPDATE state SET current_round = ?, next_round = ? WHERE id = 1`, rd.No, rd.No+1); err != nil {
		return err
	}
	return tx.Commit()
}

// Winners 一轮的中奖名单，按名次顺序。
func (s *Store) Winners(no int64) ([]Winner, error) {
	rows, err := s.db.Query(
		`SELECT participant_id, name, theme, shumeiniang FROM draws WHERE round_no = ? ORDER BY position`, no)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Winner{}
	for rows.Next() {
		var w Winner
		if err := rows.Scan(&w.ParticipantID, &w.Name, &w.Theme, &w.Image); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetCursor 推进/回退揭晓进度，取值范围 [-1, draw_count-1]。
func (s *Store) SetCursor(no int64, index int) error {
	var count int
	if err := s.db.QueryRow(`SELECT draw_count FROM rounds WHERE no = ?`, no).Scan(&count); err != nil {
		return err
	}
	if index < -1 {
		index = -1
	}
	if index > count-1 {
		index = count - 1
	}
	_, err := s.db.Exec(`UPDATE rounds SET revealed_index = ? WHERE no = ?`, index, no)
	return err
}

// BumpSkip 通知大屏跳过当前动画。
func (s *Store) BumpSkip(no int64) error {
	_, err := s.db.Exec(`UPDATE rounds SET skip_seq = skip_seq + 1 WHERE no = ?`, no)
	return err
}

// EndRound 回到待机（轮次本身留着，还能查历史与导出）。
func (s *Store) EndRound() error {
	_, err := s.db.Exec(`UPDATE state SET current_round = NULL WHERE id = 1`)
	return err
}

// ---------- 重置 ----------

// Reset scope=winners：保留名单，删掉所有轮次与中奖记录（回到"全员未中奖"）。
// Reset scope=all：连名单一起删（等于重新开一场）。
func (s *Store) Reset(scope string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`DELETE FROM draws`,
		`DELETE FROM rounds`,
		`UPDATE state SET current_round = NULL WHERE id = 1`,
	}
	if scope == "all" {
		stmts = append(stmts, `DELETE FROM participants`)
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
