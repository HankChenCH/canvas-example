// example/server 入口:路由、seed、静态、启动预检(spec §1.1)。
//
// 运行约定:进程 CWD = example/server,所有相对路径资源按此解析(spec §1.2)——
// dev 用 `cd example/server && go run .`,compose 运行相 WORKDIR=/app 同构。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/HankChenCH/go-canvas/image-renderer/typography"

	"example/server/internal/api"
	"example/server/internal/seed"
	"example/server/internal/store"
)

// runtimeDirs 运行时目录,启动即建(spec §1.1;gitignored,不入库)
var runtimeDirs = []string{"data", "assets/u", "renders", ".cache"}

// fontPath 绘制字体:graph 内按此相对路径引用(spec §5.3 三合一形态)
const fontPath = "assets/fonts/NotoSansSC-Regular.otf"

const listenAddr = ":8080" // 监听 :8080 固定,无配置面(spec §1.4)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "example-server:", err)
		os.Exit(1)
	}
}

func run() error {
	for _, dir := range runtimeDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("建运行目录 %s: %w", dir, err)
		}
	}

	st, err := store.Open(filepath.Join("data", "app.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	// 启动播种(spec §5.2):资源补齐 + 数据源实体按名取回或插入 + templates 表
	// 空则插默认模板(引用该数据源),三者幂等
	if _, err := seed.CopyAssets(filepath.Join("seed", "assets"), "assets"); err != nil {
		return err
	}
	content, err := seed.Load("seed")
	if err != nil {
		return err
	}
	dsID, err := st.SeedDataSourceIfMissing(context.Background(), content.DataSource)
	if err != nil {
		return err
	}
	content.Template.DataSourceID = &dsID
	if err := st.SeedTemplateIfEmpty(context.Background(), content.Template); err != nil {
		return err
	}

	// 字体启动探针(spec §2.7):失败 fail-fast 拒绝启动——内置点阵仅覆盖
	// ASCII,静默降级中文必成方块,故不降级
	if err := probeFont(fontPath); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           api.New(st),
		ReadHeaderTimeout: 10 * time.Second,
	}
	cwd, _ := os.Getwd()
	log.Printf("example-server listening on %s (CWD=%s)", listenAddr, cwd)
	return srv.ListenAndServe()
}

// probeFont 字体启动探针:加载口径与渲染端一致(visualcheck pickFont 同款,
// typography.LoadFontFace 单一入口),face 即用即弃——渲染端按(字体,字号)自持缓存
func probeFont(path string) error {
	face, err := typography.LoadFontFace(path, 12)
	if err != nil {
		return fmt.Errorf("字体启动预检失败(%s): %w", path, err)
	}
	return face.Close()
}
