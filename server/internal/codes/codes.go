// Package codes:稳定错误码单源(spec §2.5 总表)。code 是三端一致性锚点,
// 按 code 判定语义、message 不参与程序判定——全表集中一处,api/preflight/
// render 共同消费,不各自持有字面量。标 ◆ 的为服务端自有 code(不属
// go-canvas sentinel);go-canvas sentinel 段经 errors.Is 在各包 mapper 对号。
package codes

const (
	// 请求体形态段(◆)
	InvalidJSON      = "invalid_json"
	RequestTooLarge  = "request_too_large"
	TemplateNotFound = "template_not_found"

	// 解码(FromGraph)
	UnknownLayerType     = "unknown_layer_type"
	TemplateRowsConflict = "template_rows_conflict"
	RowsPathMissing      = "rows_path_missing"

	// 填充(hydrate)
	RowsPathInvalid          = "rows_path_invalid"
	ExpressionRowOutsideLoop = "expression_row_outside_loop"
	ReservedRootKey          = "reserved_root_key"
	ExpressionEmptyResource  = "expression_empty_resource"
	ExpressionTypeMismatch   = "expression_type_mismatch"
	ExpressionSyntaxError    = "expression_syntax_error"

	// 文档编译(链校验 + 流分配 + 分页)
	FlowChainInvalid         = "flow_chain_invalid"
	FlowRowsPathInconsistent = "flow_rows_path_inconsistent"
	PaginateTargetInvalid    = "paginate_target_invalid"
	ContentOverflow          = "content_overflow"

	// 数据源 draft-07(◆)
	SchemaInvalid         = "schema_invalid"
	DatasetSchemaMismatch = "dataset_schema_mismatch"

	// 物化(资源维度)
	ResourceDownloadFailed = "resource_download_failed"
	ResourceSaveFailed     = "resource_save_failed"
	QRGenerateFailed       = "qr_generate_failed"

	// 渲染控制面
	RenderDeadlineExceeded = "render_deadline_exceeded"
	RenderCancelled        = "render_cancelled" // 不写响应,仅日志

	// 服务端自有(◆)
	RenderOutputWriteFailed = "render_output_write_failed"
	InternalError           = "internal_error"
)
