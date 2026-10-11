package rbac

import "testing"

func TestCapabilityMatrix(t *testing.T) {
	// user：只能签到
	if !Can(User, CapCheckin) {
		t.Fatal("user 应能签到")
	}
	for _, c := range []Capability{CapDirectors, CapLottery, CapRoles} {
		if Can(User, c) {
			t.Fatalf("user 不该有 %s", c)
		}
	}
	// minister 与 operator 是并列的两摊，互不覆盖
	if !Can(Minister, CapDirectors) || Can(Minister, CapLottery) || Can(Minister, CapRoles) {
		t.Fatal("minister 应只管部长介绍")
	}
	if !Can(Operator, CapLottery) || Can(Operator, CapDirectors) || Can(Operator, CapRoles) {
		t.Fatal("operator 应只管抽奖与大屏")
	}
	// admin：全都有（含管角色）
	for _, c := range []Capability{CapCheckin, CapDirectors, CapLottery, CapRoles} {
		if !Can(Admin, c) {
			t.Fatalf("admin 应有 %s", c)
		}
	}
	// 未登录（空角色）与不存在的角色：什么都没有
	for _, role := range []string{"", "root", "superuser", "ADMIN"} {
		for _, c := range []Capability{CapCheckin, CapDirectors, CapLottery, CapRoles} {
			if Can(role, c) {
				t.Fatalf("角色 %q 不该有 %s", role, c)
			}
		}
		if Valid(role) {
			t.Fatalf("角色 %q 不该被认为合法", role)
		}
	}
	if Default != User {
		t.Fatalf("登录后的默认角色应是 user，得到 %q", Default)
	}
	if len(All) != 4 {
		t.Fatalf("合法角色应有四个，得到 %v", All)
	}
}
