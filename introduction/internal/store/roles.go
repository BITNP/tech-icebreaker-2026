// 角色分配与"部长介绍"两张表。
//
// 角色只有一条记录对应一个账号（sub 主键）：一个人一个角色。配置文件里的
// admin_subs 是"根管理员"，不落这张表、也不可撤销（见 internal/api 的角色解析）。
package store

import "database/sql"

const rolesSchema = `
CREATE TABLE IF NOT EXISTS roles (
  sub        TEXT PRIMARY KEY,
  role       TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',   -- 记一下名字，管理台好认人
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS directors (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  sub         TEXT    NOT NULL,          -- 谁的介绍（部长自己的账号）
  name        TEXT    NOT NULL,          -- 取账号真名
  theme       TEXT    NOT NULL DEFAULT '',
  shumeiniang TEXT    NOT NULL DEFAULT '',
  note        TEXT    NOT NULL DEFAULT '',  -- 一句话自我介绍
  updated_at  INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_directors_sub ON directors(sub);
`

// RoleRow 一条角色分配。
type RoleRow struct {
	Sub       string
	Role      string
	Name      string
	UpdatedAt int64
}

// Director 一条部长介绍。
type Director struct {
	ID        int64
	Sub       string
	Name      string
	Theme     string
	Image     string
	Note      string
	UpdatedAt int64
}

// RoleOf 取某账号的角色；没分配过返回空串（调用方按默认角色处理）。
func (s *Store) RoleOf(sub string) (string, error) {
	var role string
	err := s.db.QueryRow(`SELECT role FROM roles WHERE sub = ?`, sub).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

// SetRole 分配/改角色（一个人一个角色，所以是 upsert）。
func (s *Store) SetRole(sub, role, name string, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO roles (sub, role, name, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(sub) DO UPDATE SET role = excluded.role, name = excluded.name,
		   updated_at = excluded.updated_at`,
		sub, role, name, now)
	return err
}

// DeleteRole 撤销角色（回到默认 user）。返回是否真的删了一行。
func (s *Store) DeleteRole(sub string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM roles WHERE sub = ?`, sub)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) Roles() ([]RoleRow, error) {
	rows, err := s.db.Query(`SELECT sub, role, name, updated_at FROM roles ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleRow{}
	for rows.Next() {
		var r RoleRow
		if err := rows.Scan(&r.Sub, &r.Role, &r.Name, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountDBAdmins 库里显式给了 admin 的人数（不含配置文件里的根管理员）。
func (s *Store) CountDBAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM roles WHERE role = ?`, "admin").Scan(&n)
	return n, err
}

// Directors 部长介绍列表，按添加顺序。
func (s *Store) Directors() ([]Director, error) {
	rows, err := s.db.Query(`
		SELECT id, sub, name, theme, shumeiniang, note, updated_at
		FROM directors ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Director{}
	for rows.Next() {
		var d Director
		if err := rows.Scan(&d.ID, &d.Sub, &d.Name, &d.Theme, &d.Image, &d.Note, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpsertDirector 写某账号的部长介绍（一个人一条，重复提交就是修改自己的）。
func (s *Store) UpsertDirector(d Director, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO directors (sub, name, theme, shumeiniang, note, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(sub) DO UPDATE SET name = excluded.name, theme = excluded.theme,
		   shumeiniang = excluded.shumeiniang, note = excluded.note,
		   updated_at = excluded.updated_at`,
		d.Sub, d.Name, d.Theme, d.Image, d.Note, now)
	return err
}

func (s *Store) DeleteDirectorBySub(sub string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM directors WHERE sub = ?`, sub)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
