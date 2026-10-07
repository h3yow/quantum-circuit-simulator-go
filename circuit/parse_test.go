package circuit

import (
	"errors"
	"testing"
)

func TestParseValidCircuit(t *testing.T) {
	input := `
# Bell 态 + 测量 + 条件操作
qubits 2
bits 2
H 0
CNOT 0 1
MEASURE 0 -> 0
IF 0==1 X 1
MEASURE 1 -> 1
`
	t.Logf("输入电路:\n%s", input)
	c, err := Parse(input)
	if err != nil {
		t.Fatalf("合法电路解析失败: %v", err)
	}
	if c.NumQubits != 2 || c.NumBits != 2 || len(c.Ops) != 5 {
		t.Fatalf("解析结果不符: %+v", c)
	}
	if c.Ops[3].Kind != OpConditional || c.Ops[3].CondBit != 0 || c.Ops[3].CondValue != 1 {
		t.Fatalf("条件操作解析不符: %+v", c.Ops[3])
	}
	t.Logf("判定依据: 声明与 5 条操作全部正确解析, 条件操作依赖 bit0==1")
}

func TestParseRejections(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  error // 期望的错误类型（用 errors.As 匹配）
	}{
		{"未知操作", "qubits 1\nbits 1\nFOOBAR 0\n", ErrUnknownOp{}},
		{"量子比特越界", "qubits 1\nbits 1\nH 3\n", ErrOutOfRange{}},
		{"经典比特越界", "qubits 1\nbits 1\nMEASURE 0 -> 5\n", ErrOutOfRange{}},
		{"条件经典比特越界", "qubits 1\nbits 1\nIF 9==1 X 0\n", ErrOutOfRange{}},
		{"重复声明qubits", "qubits 1\nqubits 2\nbits 1\n", ErrDuplicateDecl{}},
		{"重复声明bits", "qubits 1\nbits 1\nbits 2\n", ErrDuplicateDecl{}},
		{"缺少qubits声明", "bits 1\nH 0\n", ErrIncomplete{}},
		{"缺少bits声明", "qubits 1\n", ErrIncomplete{}},
		{"声明前出现门", "H 0\nqubits 1\nbits 1\n", ErrIncomplete{}},
		{"MEASURE语法不完整", "qubits 1\nbits 1\nMEASURE 0\n", ErrIncomplete{}},
		{"门操作数不足", "qubits 2\nbits 1\nCNOT 0\n", ErrIncomplete{}},
		{"旋转门缺参数", "qubits 1\nbits 1\nRX 0\n", ErrIncomplete{}},
		{"非法条件取值", "qubits 1\nbits 1\nIF 0==2 X 0\n", ErrIncomplete{}},
		{"双比特门操作数重复", "qubits 2\nbits 1\nCNOT 0 0\n", ErrDuplicateOperand{}},
		{"SWAP操作数重复", "qubits 2\nbits 1\nSWAP 1 1\n", ErrDuplicateOperand{}},
		{"条件门操作数重复", "qubits 2\nbits 1\nIF 0==1 CZ 1 1\n", ErrDuplicateOperand{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("输入电路:\n%s", tc.input)
			_, err := Parse(tc.input)
			if err == nil {
				t.Fatalf("期望拒绝但解析成功")
			}
			t.Logf("判定依据: 得到错误 %q (类型 %T)", err, err)
			// errors.As 需要目标指针
			switch tc.want.(type) {
			case ErrUnknownOp:
				var e ErrUnknownOp
				if !errors.As(err, &e) {
					t.Fatalf("错误类型不符: got %T want ErrUnknownOp", err)
				}
			case ErrOutOfRange:
				var e ErrOutOfRange
				if !errors.As(err, &e) {
					t.Fatalf("错误类型不符: got %T want ErrOutOfRange", err)
				}
			case ErrDuplicateDecl:
				var e ErrDuplicateDecl
				if !errors.As(err, &e) {
					t.Fatalf("错误类型不符: got %T want ErrDuplicateDecl", err)
				}
			case ErrIncomplete:
				var e ErrIncomplete
				if !errors.As(err, &e) {
					t.Fatalf("错误类型不符: got %T, want ErrIncomplete", err)
				}
			case ErrDuplicateOperand:
				var e ErrDuplicateOperand
				if !errors.As(err, &e) {
					t.Fatalf("错误类型不符: got %T, want ErrDuplicateOperand", err)
				}
			}
		})
	}
}

func TestValidateManualCircuit(t *testing.T) {
	// 绕过 Parse 手工构造的线路，同样要被 Validate 拒绝重复操作数。
	c := &Circuit{
		NumQubits: 2,
		NumBits:   1,
		Ops: []Operation{
			{Kind: OpGate, Gate: Gate{Name: "CNOT", Qubits: []int{1, 1}}},
		},
	}
	err := Validate(c)
	t.Logf("判定依据: 手工构造的 CNOT 1 1 属非法线路, Validate 得到: %v", err)
	var e ErrDuplicateOperand
	if !errors.As(err, &e) {
		t.Fatalf("期望 ErrDuplicateOperand, 得到 %T: %v", err, err)
	}

	// 合法线路应通过。
	ok := &Circuit{
		NumQubits: 2,
		NumBits:   1,
		Ops: []Operation{
			{Kind: OpGate, Gate: Gate{Name: "CNOT", Qubits: []int{0, 1}}},
			{Kind: OpMeasure, Qubit: 0, Bit: 0},
		},
	}
	if err := Validate(ok); err != nil {
		t.Fatalf("合法线路未通过 Validate: %v", err)
	}
	t.Logf("对照组: CNOT 0 1 + MEASURE 通过 Validate")
}
