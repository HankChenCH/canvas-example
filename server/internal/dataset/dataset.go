// Package dataset:数据源 draft-07 完整校验(spec §2.4 #6)。校验权威在服务端,
// 前端不复制校验器(04 票锚点①);两个稳定码归 §2.5 数据源段,HTTP 均 400。
package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// 稳定错误码(spec §2.5 数据源段,服务端自有 ◆ 码)
const (
	CodeSchemaInvalid         = "schema_invalid"
	CodeDatasetSchemaMismatch = "dataset_schema_mismatch"
)

// Error 校验失败:Code 为稳定错误码,Message 中文可读(不参与程序判定)
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// datasetResourceURL 编译资源位:仅库内寻址用,$ref 片段路径相对它解析
const datasetResourceURL = "urn:example:dataset"

// Validate draft-07 完整校验:解析 + 编译 schema 的任何失败归 schema_invalid
// (含 null/标量等非法 schema 形态);实例不过编译后的 schema 归
// dataset_schema_mismatch。未声明 $schema 的 schema 按 draft-07 裁决(声明了
// 其他 draft 的按声明走——库对 $schema 尊重,默认值由 DefaultDraft 钉死 draft-07)。
// 通过返回 nil。
func Validate(schemaRaw, dataRaw json.RawMessage) *Error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaRaw))
	if err != nil {
		return &Error{CodeSchemaInvalid, fmt.Sprintf("schema 不是合法 JSON 文档: %v", err)}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	if err := compiler.AddResource(datasetResourceURL, doc); err != nil {
		return &Error{CodeSchemaInvalid, fmt.Sprintf("schema 资源不可用: %v", err)}
	}
	schema, err := compiler.Compile(datasetResourceURL)
	if err != nil {
		return &Error{CodeSchemaInvalid, fmt.Sprintf("schema 不合法(draft-07 编译失败): %v", err)}
	}

	var instance any
	if err := json.Unmarshal(dataRaw, &instance); err != nil {
		// dataRaw 由请求体 JSON 内提取,恒为合法 JSON——此路防御性兜底
		return &Error{CodeDatasetSchemaMismatch, fmt.Sprintf("data 不是合法 JSON: %v", err)}
	}
	if err := schema.Validate(instance); err != nil {
		return &Error{CodeDatasetSchemaMismatch, fmt.Sprintf("数据未通过 schema 校验: %v", err)}
	}
	return nil
}
