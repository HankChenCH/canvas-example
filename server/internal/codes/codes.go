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

	// 数据源寻址(◆;数据源是独立实体,与模板寻址分码)
	DataSourceNotFound = "data_source_not_found"

	// 数据源删除守卫(◆;spec §2.4 #6e,28 票):仍被模板引用时拒绝删除
	DataSourceInUse = "data_source_in_use"

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
	// ADR 0020 资源表达式单值门:已标记 Image/Qr 的表达式必须恰为一个裸路径片段
	ExpressionResourceSingleRef = "expression_resource_single_ref"

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

	// 模板版本化(◆;模板草稿-发布-版本化 spec §3/§4):发布守门 422、渲染缺省
	// 从未发布 409(状态冲突型,对齐 data_source_in_use)、版本寻址失败 404
	// (对齐 template_not_found 模式)
	InvalidGraph        = "invalid_graph"
	TemplateUnpublished = "template_unpublished"
	VersionNotFound     = "version_not_found"
)
