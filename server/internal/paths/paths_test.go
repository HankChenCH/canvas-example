package paths

import "testing"

// 资源路径三合一归一(spec §2.2):剥一个前导斜杠按 CWD 相对读盘,
// "/assets/u/x.png" 与 "assets/u/x.png" 落同一磁盘文件
func TestLocalize(t *testing.T) {
	cases := []struct{ ref, want string }{
		{"/assets/u/x.png", "assets/u/x.png"}, // URL/graph 引用串形态
		{"assets/u/x.png", "assets/u/x.png"},  // 磁盘相对形态(DB 存储)
		{"/assets/fonts/NotoSansSC-Regular.otf", "assets/fonts/NotoSansSC-Regular.otf"},
		{"", ""},
		{"/", ""},
	}
	for _, c := range cases {
		if got := ToLocalPath(c.ref); got != c.want {
			t.Errorf("ToLocalPath(%q) = %q, 期望 %q", c.ref, got, c.want)
		}
	}
}
