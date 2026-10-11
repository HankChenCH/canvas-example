// 模板版本读写(模板草稿-发布-版本化,工单 01/02,spec §2/§3):版本史存储面。
// 领域纪律:版本行 INSERT-only 永无 UPDATE(不可变性是存储层纪律,不靠约束表达);
// 版本号每模板自增从 1 起,事务内 MAX(version)+1 计算,UNIQUE(template_id, version)
// 兜底;快照粒度 = 文档整体(canvases + flow_chain 列文本原样固化,data_source_id
// 是绑定不是内容,不进快照);「最新已发布」= ORDER BY version DESC LIMIT 1。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrTemplateVersionNotFound 版本寻址失败(版本不存在,含对无版本模板指定),
// handler 层映射 404 version_not_found(spec §4 渲染选版)
var ErrTemplateVersionNotFound = errors.New("template version not found")

// TemplateVersionSummary 版本列表摘要:轻量两字段,不吐 canvases 载荷(spec §3)
type TemplateVersionSummary struct {
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
}

// TemplateVersionRecord 单版本全量(spec §3):canvases 与 TemplateRecord.Canvases
// 同形(RawMessage 原样进出,零校验);flowChain null 透传(镜像 templates 列)
type TemplateVersionRecord struct {
	Version   int64           `json:"version"`
	Canvases  []CanvasEntry   `json:"canvases"`
	FlowChain json.RawMessage `json:"flowChain"`
	CreatedAt string          `json:"createdAt"`
}

// BackfillTemplateVersionsV1 存量回填(spec §2):凡零版本行的模板,现
// canvases+flow_chain 列文本原样固化为 v1——不重新 marshal,字节面诚实;
// created_at = 回填运行时刻(版本的出生时间,非内容最后编辑时间)。谓词幂等
// (「无版本行」判定,重跑 no-op 且 created_at 不变);单事务原子,返回回填模板数。
func (s *Store) BackfillTemplateVersionsV1(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开启版本回填事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Commit 后为 no-op

	rows, err := tx.QueryContext(ctx,
		`SELECT t.id, t.canvases, t.flow_chain FROM templates t
		 WHERE NOT EXISTS (SELECT 1 FROM template_versions v WHERE v.template_id = t.id)`)
	if err != nil {
		return 0, fmt.Errorf("查零版本模板: %w", err)
	}
	type draftRow struct {
		id       int64
		canvases string
		chain    sql.NullString
	}
	var drafts []draftRow
	for rows.Next() {
		var d draftRow
		if err := rows.Scan(&d.id, &d.canvases, &d.chain); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("扫描零版本模板: %w", err)
		}
		drafts = append(drafts, d)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("遍历零版本模板: %w", err)
	}
	_ = rows.Close()

	created := nowRFC3339() // 整批一个运行时刻(同一轮回填的版本同寿)
	for _, d := range drafts {
		var chain any
		if d.chain.Valid {
			chain = d.chain.String
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO template_versions (template_id, version, canvases, flow_chain, created_at)
			 VALUES (?, 1, ?, ?, ?)`,
			d.id, d.canvases, chain, created,
		); err != nil {
			return 0, fmt.Errorf("回填模板 %d 为 v1: %w", d.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交版本回填: %w", err)
	}
	return int64(len(drafts)), nil
}

// PublishTemplateVersion 发布动作的存储面(spec §3):单事务读草稿列文本,与最新
// 版本行文本字节比较(canvases 与 flow_chain 两者都等才算 duplicate,flow_chain
// 按 SQL NULL 对齐)——相等幂等跳过,回 (最新版本号, 既有 createdAt, true);
// 不等则 INSERT 新行(version = MAX+1 从 1 起,INSERT-only),回 (新版本号,
// 新 createdAt, false)。发布不触碰 templates 行(updated_at 不动)。发布读的是
// 已保存草稿(handler 层先过 preflight 守门)。模板不存在 → ErrNotFound
func (s *Store) PublishTemplateVersion(ctx context.Context, id int64) (version int64, createdAt string, duplicate bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", false, fmt.Errorf("开启发布事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Commit 后为 no-op

	var draftCanvases string
	var draftChain sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT canvases, flow_chain FROM templates WHERE id = ?`, id,
	).Scan(&draftCanvases, &draftChain)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, ErrNotFound
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("读模板 %d 草稿列: %w", id, err)
	}

	var latestVersion int64
	var latestCanvases string
	var latestChain sql.NullString
	var latestCreatedAt string
	hasLatest := true
	err = tx.QueryRowContext(ctx,
		`SELECT version, canvases, flow_chain, created_at FROM template_versions
		 WHERE template_id = ? ORDER BY version DESC LIMIT 1`, id,
	).Scan(&latestVersion, &latestCanvases, &latestChain, &latestCreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		hasLatest = false
	} else if err != nil {
		return 0, "", false, fmt.Errorf("查模板 %d 最新版本: %w", id, err)
	}

	if hasLatest && latestCanvases == draftCanvases && nullTextEqual(latestChain, draftChain) {
		if err := tx.Commit(); err != nil { // 只读事务,提交即收尾
			return 0, "", false, fmt.Errorf("提交发布判定: %w", err)
		}
		return latestVersion, latestCreatedAt, true, nil
	}

	var nextVersion int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM template_versions WHERE template_id = ?`, id,
	).Scan(&nextVersion); err != nil {
		return 0, "", false, fmt.Errorf("算模板 %d 下一版本号: %w", id, err)
	}
	now := nowRFC3339()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO template_versions (template_id, version, canvases, flow_chain, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, nextVersion, draftCanvases, nullTextOrNil(draftChain), now,
	); err != nil {
		return 0, "", false, fmt.Errorf("插入模板 %d 版本 %d: %w", id, nextVersion, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, "", false, fmt.Errorf("提交发布: %w", err)
	}
	return nextVersion, now, false, nil
}

// ListTemplateVersions 版本列表,version 降序(最新在前);空 = 未发布(空数组,
// 发布态永远是推导态,spec §3)。模板不存在 → ErrNotFound(与 GetTemplate 惯例一致)
func (s *Store) ListTemplateVersions(ctx context.Context, id int64) ([]TemplateVersionSummary, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM templates WHERE id = ?`, id,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("查模板 %d: %w", id, err)
	}
	if exists == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT version, created_at FROM template_versions WHERE template_id = ? ORDER BY version DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("查询模板 %d 版本列表: %w", id, err)
	}
	defer rows.Close()
	items := []TemplateVersionSummary{} // 空版本史也回 [],非 null
	for rows.Next() {
		var item TemplateVersionSummary
		if err := rows.Scan(&item.Version, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描版本摘要: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetTemplateVersion 单版本全量(spec §3):canvases 文本原样解出、flow_chain 可空。
// 版本不存在(含对无版本模板指定——无版本模板查任何版本都落空)→
// ErrTemplateVersionNotFound
func (s *Store) GetTemplateVersion(ctx context.Context, id int64, version int64) (*TemplateVersionRecord, error) {
	var rec TemplateVersionRecord
	var canvases string
	var chain sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT version, canvases, flow_chain, created_at FROM template_versions
		 WHERE template_id = ? AND version = ?`, id, version,
	).Scan(&rec.Version, &canvases, &chain, &rec.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTemplateVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询模板 %d 版本 %d: %w", id, version, err)
	}
	if err := json.Unmarshal([]byte(canvases), &rec.Canvases); err != nil {
		return nil, fmt.Errorf("解析模板 %d 版本 %d canvases: %w", id, version, err)
	}
	rec.FlowChain = rawOrNull(chain) // store.go 同款:NULL 列 → nil → JSON null 透传
	return &rec, nil
}

// LatestTemplateVersion 最新已发布版本(spec §4):无版本行 ok=false 不报错——
// 渲染缺省语义由此区分「未发布 409」与「指定版本不存在 404」
func (s *Store) LatestTemplateVersion(ctx context.Context, id int64) (version int64, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT version FROM template_versions WHERE template_id = ? ORDER BY version DESC LIMIT 1`, id,
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("查模板 %d 最新版本: %w", id, err)
	}
	return version, true, nil
}

// RevertTemplateToVersion 回滚的存储面(spec §5):单事务原子复制——读版本行,
// 写 templates.canvases/flow_chain 并刷新 updated_at(回滚改写草稿内容 = 内容编辑,
// 与「发布不动」形成语义对照);name/data_source_id/editor_state 都不碰。
// 版本线线性:回滚永不产生新版本。模板不存在 → ErrNotFound;版本不存在 →
// ErrTemplateVersionNotFound(回滚,草稿列不被部分改写)
func (s *Store) RevertTemplateToVersion(ctx context.Context, id int64, version int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启回滚事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Commit 后为 no-op

	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM templates WHERE id = ?`, id,
	).Scan(&exists); err != nil {
		return fmt.Errorf("查模板 %d: %w", id, err)
	}
	if exists == 0 {
		return ErrNotFound
	}

	var canvases string
	var chain sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT canvases, flow_chain FROM template_versions WHERE template_id = ? AND version = ?`,
		id, version,
	).Scan(&canvases, &chain)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTemplateVersionNotFound
	}
	if err != nil {
		return fmt.Errorf("读模板 %d 版本 %d: %w", id, version, err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE templates SET canvases = ?, flow_chain = ?, updated_at = ? WHERE id = ?`,
		canvases, nullTextOrNil(chain), nowRFC3339(), id,
	); err != nil {
		return fmt.Errorf("回滚模板 %d 至版本 %d: %w", id, version, err)
	}
	return tx.Commit()
}

// nullTextEqual 两个可空文本列字节面相等:NULL 与 NULL 相等,NULL 与任何文本不等
func nullTextEqual(a, b sql.NullString) bool {
	if a.Valid != b.Valid {
		return false
	}
	if !a.Valid {
		return true
	}
	return a.String == b.String
}

// nullTextOrNil NullString → 入库值(NULL 列落 SQL nil,文本原样)
func nullTextOrNil(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}
