package icebreaker

import (
	"io/fs"
	"testing"
)

// 前端产物必须落在模块内的 dist/（go:embed 不能引用上级目录）。
// 仓库里只有 dist/.gitkeep，也要保证 go build 能过。
func TestFrontendDirIsEmbedded(t *testing.T) {
	info, err := fs.Stat(Frontend, "dist")
	if err != nil {
		t.Fatalf("dist 没被嵌进来：%v", err)
	}
	if !info.IsDir() {
		t.Fatal("dist 应该是目录")
	}
}
