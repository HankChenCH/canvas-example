module example/server

go 1.25.0

// 本地两个 module(与 fixtures/validate/go.mod 同形):核心 module + 位图后端嵌套 module
replace github.com/HankChenCH/go-canvas => ../../go-canvas

replace github.com/HankChenCH/go-canvas/image-renderer => ../../go-canvas/image-renderer

require (
	github.com/HankChenCH/go-canvas v0.0.0-00010101000000-000000000000
	github.com/HankChenCH/go-canvas/image-renderer v0.0.0-00010101000000-000000000000
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	modernc.org/sqlite v1.55.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/yeqown/go-qrcode/v2 v2.3.0 // indirect
	github.com/yeqown/go-qrcode/writer/standard v1.4.0 // indirect
	github.com/yeqown/reedsolomon v1.0.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
