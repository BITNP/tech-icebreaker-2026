// Package rbac 定义角色与能力矩阵。
//
// 四个角色是并列的，不是简单的等级高低：minister 管"部长介绍"，operator 管
// 抽奖与大屏，两者互不覆盖；admin 是唯一能管角色的角色，同时拥有全部能力。
// 未登录不属于任何角色，只能看大屏。
package rbac

// 角色名。存库、写日志、前端判断都用这几个字符串。
const (
	User     = "user"     // 普通部员：只能签到
	Minister = "minister" // 部长：能往"部长介绍"里加自己的内容
	Operator = "operator" // 操作员：控制抽奖与大屏
	Admin    = "admin"    // 管理员：全部权限，含管理角色
)

// Default 登录后没有显式角色时的默认角色。
const Default = User

// Capability 一项能力。
type Capability string

const (
	CapCheckin   Capability = "checkin"   // 签到
	CapDirectors Capability = "directors" // 编辑部长介绍
	CapLottery   Capability = "lottery"   // 抽奖与大屏控制、名单管理
	CapRoles     Capability = "roles"     // 查看/授予/撤销角色
)

var matrix = map[string]map[Capability]bool{
	User:     {CapCheckin: true},
	Minister: {CapCheckin: true, CapDirectors: true},
	Operator: {CapCheckin: true, CapLottery: true},
	Admin:    {CapCheckin: true, CapDirectors: true, CapLottery: true, CapRoles: true},
}

// All 是全部合法角色，用于校验配置与接口入参。
var All = []string{User, Minister, Operator, Admin}

// Valid 判断角色名是否合法。
func Valid(role string) bool {
	_, ok := matrix[role]
	return ok
}

// Can 报告 role 是否拥有能力 c。
func Can(role string, c Capability) bool {
	return matrix[role][c]
}
