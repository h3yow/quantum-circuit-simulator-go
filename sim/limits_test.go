package sim

import (
	"errors"
	"testing"
)

const smallCircuit = `
qubits 2
bits 1
H 0
MEASURE 0 -> 0
`

func TestLimitClampingRules(t *testing.T) {
	cases := []struct {
		name string
		arg  int
		want int
	}{
		{"零值用缺省", 0, DefaultMaxQubits},
		{"负值用缺省", -5, DefaultMaxQubits},
		{"正常值原样生效", 12, 12},
		{"等于硬上限", HardMaxQubits, HardMaxQubits},
		{"超出硬上限被钳制", HardMaxQubits + 100, HardMaxQubits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.arg)
			t.Logf("判定依据: New(%d) 生效上限应为 %d, 实际 %d", tc.arg, tc.want, s.MaxQubits)
			if s.MaxQubits != tc.want {
				t.Fatalf("New(%d).MaxQubits = %d, 期望 %d", tc.arg, s.MaxQubits, tc.want)
			}
		})
	}
}

func TestUnreasonableLimitDoesNotRejectNormalCircuit(t *testing.T) {
	c := mustParse(t, smallCircuit)
	// 调用方未给上限(0/负值)或给出离谱上限时, 正常线路都必须照常可跑
	for _, arg := range []int{0, -100, HardMaxQubits + 1000} {
		s := New(arg)
		bits, err := s.Run(c, 1)
		t.Logf("判定依据: New(%d) 钳制后上限 %d, 2 量子比特正常线路应照常运行, err=%v bits=%v",
			arg, s.MaxQubits, err, bits)
		if err != nil {
			t.Fatalf("New(%d) 下正常线路被拒绝: %v", arg, err)
		}
		if _, err := s.Sample(c, 10, 1); err != nil {
			t.Fatalf("New(%d) 下正常线路采样被拒绝: %v", arg, err)
		}
		sess, err := s.NewSession(c, 1)
		if err != nil {
			t.Fatalf("New(%d) 下正常线路会话被拒绝: %v", arg, err)
		}
		if _, err := sess.RunToEnd(); err != nil {
			t.Fatalf("New(%d) 下正常线路会话推进失败: %v", arg, err)
		}
	}
}

func TestDefaultLimitProtectsWhenUnset(t *testing.T) {
	// 未显式给上限时, 超出缺省上限的线路必须被拒绝而不是尝试分配巨额内存
	input := `
qubits 21
bits 1
H 0
MEASURE 0 -> 0
`
	c := mustParse(t, input)
	s := New(0) // 未指定 -> 缺省 20
	_, err := s.Run(c, 1)
	t.Logf("判定依据: 缺省上限 %d, 21 量子比特线路应得到 ErrTooManyQubits, 实际: %v", DefaultMaxQubits, err)
	var e ErrTooManyQubits
	if !errors.As(err, &e) {
		t.Fatalf("期望 ErrTooManyQubits, 得到 %T: %v", err, err)
	}
}

func TestHardCapCannotBeExceeded(t *testing.T) {
	// 调用方即使显式要求更高, 超过硬上限的线路也必须被拒绝
	s := New(HardMaxQubits + 50)
	if s.MaxQubits != HardMaxQubits {
		t.Fatalf("上限未被钳制到硬上限: %d", s.MaxQubits)
	}
	input := `
qubits 31
bits 1
H 0
MEASURE 0 -> 0
`
	c := mustParse(t, input)
	_, err := s.Run(c, 1)
	t.Logf("判定依据: 硬上限 %d 不可突破, 31 量子比特线路应得到 ErrTooManyQubits, 实际: %v", HardMaxQubits, err)
	var e ErrTooManyQubits
	if !errors.As(err, &e) {
		t.Fatalf("期望 ErrTooManyQubits, 得到 %T: %v", err, err)
	}
}
