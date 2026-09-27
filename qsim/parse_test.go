package qsim

import (
	"errors"
	"testing"
)

// assertErrorKind 断言 err 是 *CircuitError 且类别匹配，并把输入与
// 判定依据写入测试日志，满足可解释性要求。
func assertErrorKind(t *testing.T, input string, err error, want ErrorKind) {
	t.Helper()
	var ce *CircuitError
	if !errors.As(err, &ce) {
		t.Fatalf("输入电路:\n%s\n判定依据: 期望 *CircuitError(%s)，实际得到 %v", input, want, err)
	}
	t.Logf("输入电路:\n%s\n判定依据: 解析按预期被拒绝，类别=%s 行号=%d 信息=%q",
		input, ce.Kind, ce.Line, ce.Message)
	if ce.Kind != want {
		t.Fatalf("输入电路:\n%s\n判定依据: 期望错误类别 %s，实际 %s(%s)",
			input, want, ce.Kind, ce.Message)
	}
}

func TestParseValidCircuit(t *testing.T) {
	src := `// 一个纠缠 + 条件操作电路
QUBITS 3
CBITS 2
H q[0]
RX(pi/2) q[1]
RZ(-pi/4) q[2]
CX q[0], q[1]
CCX q[0], q[1], q[2]
MEASURE q[0] -> c[0]
IF (c[0]) X q[2]
MEASURE q[2] -> c[1]
`
	t.Logf("输入电路:\n%s", src)
	c, err := Parse(src)
	if err != nil {
		t.Fatalf("判定依据: 期望解析成功，实际 %v", err)
	}
	t.Logf("判定依据: 解析成功 nq=%d nc=%d ops=%d", c.NQubits, c.NClbits, len(c.Operations))
	if c.NQubits != 3 || c.NClbits != 2 {
		t.Fatalf("声明数量不符: %+v", c)
	}
	if len(c.Operations) != 8 {
		t.Fatalf("操作数量不符: %d", len(c.Operations))
	}
	cond := c.Operations[6]
	if cond.Type != OpCondition || cond.IfClbit != 0 || cond.Body == nil ||
		cond.Body.Name != "X" || len(cond.Body.Qubits) != 1 || cond.Body.Qubits[0] != 2 {
		t.Fatalf("条件操作结构不符: %+v", cond)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		kind ErrorKind
	}{
		{
			"未知门操作",
			"QUBITS 1\nFOOBAR q[0]\n",
			KindUnknownOp,
		},
		{
			"量子比特越界",
			"QUBITS 2\nX q[5]\n",
			KindOutOfRange,
		},
		{
			"测量写入经典比特越界",
			"QUBITS 1\nCBITS 1\nMEASURE q[0] -> c[3]\n",
			KindOutOfRange,
		},
		{
			"条件引用经典比特越界",
			"QUBITS 2\nCBITS 1\nIF (c[2]) X q[1]\n",
			KindOutOfRange,
		},
		{
			"重复声明量子比特",
			"QUBITS 2\nQUBITS 1\n",
			KindDuplicateDecl,
		},
		{
			"重复声明经典比特",
			"CBITS 2\nCBITS 2\n",
			KindDuplicateDecl,
		},
		{
			"测量语法不完整(缺箭头)",
			"QUBITS 1\nCBITS 1\nMEASURE q[0] c[0]\n",
			KindSyntax,
		},
		{
			"条件缺右括号",
			"QUBITS 1\nCBITS 1\nIF (c[0] X q[0]\n",
			KindSyntax,
		},
		{
			"条件后缺少门操作",
			"QUBITS 1\nCBITS 1\nIF (c[0])\n",
			KindSyntax,
		},
		{
			"门参数个数错误",
			"QUBITS 1\nRX q[0]\n",
			KindInvalidArg,
		},
		{
			"门参数表达式非法",
			"QUBITS 1\nRX(foo) q[0]\n",
			KindInvalidArg,
		},
		{
			"门比特数错误(CX 只给一个比特)",
			"QUBITS 2\nCX q[0]\n",
			KindInvalidArg,
		},
		{
			"同一操作内重复比特",
			"QUBITS 2\nCX q[0], q[0]\n",
			KindInvalidArg,
		},
		{
			"QUBITS 数量不是整数",
			"QUBITS abc\n",
			KindInvalidArg,
		},
		{
			"操作数语法非法",
			"QUBITS 1\nX x[0]\n",
			KindSyntax,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src)
			if err == nil {
				t.Fatalf("输入电路:\n%s\n判定依据: 期望返回 %s 错误，但解析成功", tc.src, tc.kind)
			}
			assertErrorKind(t, tc.src, err, tc.kind)
		})
	}
}

func TestParseAngleExpressions(t *testing.T) {
	src := "QUBITS 1\nRX(2*pi) q[0]\nRY(pi/3) q[0]\nRZ(-(pi/2)) q[0]\nP(0.25) q[0]\n"
	t.Logf("输入电路:\n%s", src)
	c, err := Parse(src)
	if err != nil {
		t.Fatalf("判定依据: 角度表达式应被接受，实际 %v", err)
	}
	t.Log("判定依据: 四个含角度表达式的门全部解析成功")
	if len(c.Operations) != 4 {
		t.Fatalf("操作数不符: %d", len(c.Operations))
	}
}

func TestParseLineNumbers(t *testing.T) {
	src := "\n\nQUBITS 1\nX q[9]\n"
	_, err := Parse(src)
	var ce *CircuitError
	if !errors.As(err, &ce) || ce.Line != 4 {
		t.Fatalf("输入电路:\n%s\n判定依据: 期望第 4 行报错，实际 %v", src, err)
	}
	t.Logf("输入电路:\n%s\n判定依据: 正确定位到第 %d 行的越界引用", src, ce.Line)
}
