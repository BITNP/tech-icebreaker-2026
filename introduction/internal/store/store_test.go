package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// 现场升级二进制时数据库是旧的：Open 必须能就地补列，而不是让运维删库。
func TestOpenMigratesOldStateTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("造旧库: %v", err)
	}
	// 旧版 state：没有 next_round
	mustExec(t, db, `CREATE TABLE state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		signup_open INTEGER NOT NULL DEFAULT 1,
		current_round INTEGER)`)
	mustExec(t, db, `INSERT INTO state (id, signup_open, current_round) VALUES (1, 0, 3)`)
	// 旧版 rounds：已经抽到第 3 轮
	mustExec(t, db, `CREATE TABLE rounds (
		no INTEGER PRIMARY KEY, label TEXT NOT NULL DEFAULT '', draw_count INTEGER NOT NULL,
		remove_winners INTEGER NOT NULL, seed TEXT NOT NULL, snapshot TEXT NOT NULL,
		snapshot_hash TEXT NOT NULL, revealed_index INTEGER NOT NULL DEFAULT -1,
		skip_seq INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL)`)
	mustExec(t, db, `INSERT INTO rounds (no, draw_count, remove_winners, seed, snapshot,
		snapshot_hash, created_at) VALUES (3, 1, 1, 's', 'snap', 'hash', 0)`)
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧库: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("旧库应能直接打开并迁移: %v", err)
	}
	defer st.Close()

	// 旧数据还在
	if state, err := st.State(); err != nil {
		t.Fatalf("读运行态: %v", err)
	} else if state.SignupOpen || state.CurrentRound != 3 {
		t.Fatalf("旧运行态应被保留（关签到、当前第 3 轮），得到 %+v", state)
	}
	if rounds, err := st.Rounds(); err != nil || len(rounds) != 1 {
		t.Fatalf("旧轮次应还在，得到 %v %v", rounds, err)
	}

	// 迁移把计数抬到 max+1，续号不撞车
	no, err := st.NextRoundNo()
	if err != nil {
		t.Fatalf("读下一轮号: %v", err)
	}
	if no != 4 {
		t.Fatalf("迁移后下一轮号应是 4（最大轮次 3 + 1），得到 %d", no)
	}
	if err := st.CreateRound(Round{No: no, DrawCount: 1, RemoveWinners: true, Seed: "s2"}, nil); err != nil {
		t.Fatalf("迁移后应能开新轮: %v", err)
	}
	if next, err := st.NextRoundNo(); err != nil || next != 5 {
		t.Fatalf("开完一轮后应指向 5，得到 %d %v", next, err)
	}
}

// 老库升级：participants 补出 sub 列与唯一索引；手动加的人（sub 空）不受唯一性影响，
// 同一个组织账号第二次签到会被索引挡下。
func TestOpenMigratesParticipantsSub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old3.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("造旧库: %v", err)
	}
	mustExec(t, db, `CREATE TABLE participants (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
		theme TEXT NOT NULL DEFAULT '', shumeiniang TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'checkin', created_at INTEGER NOT NULL)`)
	mustExec(t, db, `INSERT INTO participants (name, created_at) VALUES ('老数据', 1)`)
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧库: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("旧库应能迁移: %v", err)
	}
	defer st.Close()

	people, err := st.Participants()
	if err != nil || len(people) != 1 || people[0].Name != "老数据" || people[0].Sub != "" {
		t.Fatalf("老数据应保留且 sub 为空，得到 %v %v", people, err)
	}

	// 手动加的人没有 sub，加多少个都不冲突
	for _, name := range []string{"手动甲", "手动乙"} {
		if _, err := st.AddParticipant("", name, "t", "i", "manual", 2); err != nil {
			t.Fatalf("手动加人 %s 失败: %v", name, err)
		}
	}

	// 同一个组织账号只能有一条
	if _, err := st.AddParticipant("sub-x", "周熙丁", "t", "i", "checkin", 3); err != nil {
		t.Fatalf("首次签到: %v", err)
	}
	if _, err := st.AddParticipant("sub-x", "周熙丁", "t", "i", "checkin", 4); !IsDuplicate(err) {
		t.Fatalf("同 sub 再签应被唯一索引挡下，得到 %v", err)
	}
	p, ok, err := st.ParticipantBySub("sub-x")
	if err != nil || !ok || p.Name != "周熙丁" {
		t.Fatalf("BySub 应查回记录，得到 %+v %v %v", p, ok, err)
	}
	if _, err := st.Position(p.ID); err != nil {
		t.Fatalf("排位: %v", err)
	}
	if _, ok, _ := st.ParticipantBySub(""); ok {
		t.Fatal("空 sub 不该查到人")
	}
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("准备旧库失败: %v\n%s", err, query)
	}
}
