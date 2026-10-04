// Package seed:启动播种(spec §5.2)——五份 fixture 读入默认模板 +
// seed/assets 幂等补齐到运行时 assets/(字体一份 + student-1..5.png)。
package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"example/server/internal/store"
)

// DefaultTemplateName 默认模板名(spec §5.2 钉名)
const DefaultTemplateName = "结业证书 · 批量打印页"

// Load 从 seedDir 读五份 fixture。文件随仓库逐字节分发(spec §5.2,来源 05 票
// fixture,已过解码往返与编译断言验证),此处仅按 JSON 语义读入,graph/流链
// 的契约正确性不在此复制校验。
func Load(seedDir string) (store.TemplateContent, error) {
	readJSON := func(name string) (json.RawMessage, error) {
		b, err := os.ReadFile(filepath.Join(seedDir, name))
		if err != nil {
			return nil, fmt.Errorf("读 seed %s: %w", name, err)
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("seed %s 不是合法 JSON", name)
		}
		return json.RawMessage(b), nil
	}
	var frameMain, frameCont, flowChain, schema, dataset json.RawMessage
	for _, slot := range []struct {
		file   string
		target *json.RawMessage
	}{
		{"frame-main.json", &frameMain},
		{"frame-continuation.json", &frameCont},
		{"flow-chain.json", &flowChain},
		{"dataset-schema.json", &schema},
		{"dataset.json", &dataset},
	} {
		raw, err := readJSON(slot.file)
		if err != nil {
			return store.TemplateContent{}, err
		}
		*slot.target = raw
	}
	return store.TemplateContent{
		Name: DefaultTemplateName,
		Canvases: []store.CanvasEntry{
			{Name: "主页", Graph: frameMain},
			{Name: "续页", Graph: frameCont},
		},
		FlowChain:     flowChain,
		DatasetSchema: schema,
		Dataset:       dataset,
	}, nil
}

// CopyAssets 把 seedAssets 下全部文件幂等补齐到 assetsDir:缺则拷、在则跳
// (重启不重复补,spec §5.2);只增不删——assets/u 的用户上传不被触碰。
// 返回本次新拷贝文件数。
func CopyAssets(seedAssets, assetsDir string) (int, error) {
	copied := 0
	err := filepath.WalkDir(seedAssets, func(src string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(seedAssets, src)
		if err != nil {
			return err
		}
		dst := filepath.Join(assetsDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if _, err := os.Lstat(dst); err == nil {
			return nil // 已存在即跳过(幂等)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return copied, fmt.Errorf("补齐 seed assets → %s: %w", assetsDir, err)
	}
	return copied, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
