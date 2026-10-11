// Package icebreaker 把构建好的前端挂进二进制。
//
// go:embed 不能引用上级目录，所以 Go 模块根目录就是 introduction/，
// 前端构建产物固定落在 ./dist（npm run build 的默认输出）。
package icebreaker

import "embed"

//go:embed all:dist
var Frontend embed.FS
