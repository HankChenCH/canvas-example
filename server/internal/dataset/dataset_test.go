package dataset

import (
	"encoding/json"
	"strings"
	"testing"
)

// mustCode 断言校验失败且 code 对号
func mustCode(t *testing.T, got *Error, wantCode string) {
	t.Helper()
	if got == nil {
		t.Fatalf("校验应失败(code=%s),实得通过", wantCode)
	}
	if got.Code != wantCode {
		t.Fatalf("code = %s, 期望 %s(消息:%s)", got.Code, wantCode, got.Message)
	}
}

// seedSchema 形状对齐 seed/dataset-schema.json(声明 $schema draft-07 + definitions/$ref)
const seedSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"definitions": {"certificate": {"type": "object", "required": ["name"]}},
	"properties": {"certificates": {"type": "array", "items": {"$ref": "#/definitions/certificate"}}},
	"required": ["certificates"]
}`

func TestValidate_SeedShapePasses(t *testing.T) {
	if got := Validate(json.RawMessage(seedSchema), json.RawMessage(`{"certificates":[{"name":"林晚晴"}]}`)); got != nil {
		t.Fatalf("seed 同形数据源应通过: %v", got)
	}
}

// TestValidate_SchemaInvalid schema 本身不合法(spec §2.4 #6 第一码)
func TestValidate_SchemaInvalid(t *testing.T) {
	cases := map[string]json.RawMessage{
		"键缺失(null)":    json.RawMessage(`null`),
		"空载荷":          nil,
		"标量形态":         json.RawMessage(`42`),
		"type 非字符串/数组": json.RawMessage(`{"type":42}`),
		"required 非数组": json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","required":"name"}`),
		"$ref 悬空":      json.RawMessage(`{"$ref":"#/definitions/nope"}`),
	}
	for _, schema := range cases {
		got := Validate(schema, json.RawMessage(`{}`))
		mustCode(t, got, CodeSchemaInvalid)
	}
}

// TestValidate_SchemaMismatch data 不过 schema(spec §2.4 #6 第二码)
func TestValidate_SchemaMismatch(t *testing.T) {
	cases := map[string]struct {
		schema json.RawMessage
		data   json.RawMessage
	}{
		"根类型不符":                  {json.RawMessage(`{"type":"object"}`), json.RawMessage(`[1,2]`)},
		"必填缺失":                   {json.RawMessage(seedSchema), json.RawMessage(`{}`)},
		"行缺必填字段":                 {json.RawMessage(seedSchema), json.RawMessage(`{"certificates":[{"course":"x"}]}`)},
		"data 缺失":                {json.RawMessage(`{"type":"object"}`), nil},
		"data 为 null 但 schema 拒": {json.RawMessage(`{"type":"object"}`), json.RawMessage(`null`)},
	}
	for _, tc := range cases {
		got := Validate(tc.schema, tc.data)
		mustCode(t, got, CodeDatasetSchemaMismatch)
	}
}

func TestValidate_MessageReadable(t *testing.T) {
	got := Validate(json.RawMessage(`{"type":"object"}`), json.RawMessage(`[]`))
	if got == nil || strings.TrimSpace(got.Message) == "" {
		t.Fatalf("失败应带可读 message: %v", got)
	}
}

// TestValidate_BooleanSchemaAllowed 布尔是合法 JSON Schema(draft-07 §4.3);
// 未声明 $schema 的 schema 按 draft-07 裁决(spec §2.4 #6)
func TestValidate_BooleanSchemaAllowed(t *testing.T) {
	if got := Validate(json.RawMessage(`true`), json.RawMessage(`"任意"`)); got != nil {
		t.Fatalf("true schema 应放行一切实例: %v", got)
	}
	// 2020-12 独有关键字(prefixItems)在 draft-07 下未声明 → 不是合法 draft-07 断言位,
	// 但按 draft-07 语义它是普通未知关键字(注解位)不报错;真正分叉形态是 $schema 声明
	// 2020-12 的 schema——库按声明 draft 裁决,此处只钉未声明形态走 draft-07:
	if got := Validate(json.RawMessage(`{"properties":{"a":{"type":"string"}},"additionalProperties":false}`), json.RawMessage(`{"a":"x"}`)); got != nil {
		t.Fatalf("未声明 $schema 的合法 draft-07 schema 应通过: %v", got)
	}
}
