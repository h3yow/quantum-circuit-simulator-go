package sim

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

// 含中途测量与条件操作的线路，便于观察逐步推进时的中间状态。
const sessionCircuit = `
qubits 2
bits 2
H 0
CNOT 0 1
MEASURE 0 -> 0
IF 0==1 X 1
MEASURE 1 -> 1
`

func TestSessionStepByStep(t *testing.T) {
	c := mustParse(t, sessionCircuit)
	s := New(20)
	sess, err := s.NewSession(c, 42)
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	// 第 1 步: H 0 之后 q0 测 1 的概率应为 0.5, 经典比特均不确定。
	r, err := sess.Step(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("第 1 步后: %+v", r)
	if r.Step != 1 || r.Total != 5 || r.Done {
		t.Fatalf("进度不符: %+v", r)
	}
	if r.Bits[0] != -1 || r.Bits[1] != -1 {
		t.Fatalf("尚未测量, 经典比特应全为 -1: %v", r.Bits)
	}
	if math.Abs(r.Prob1-0.5) > 1e-9 {
		t.Fatalf("H 之后 q0 测 1 概率应为 0.5, 得到 %v", r.Prob1)
	}

	// 第 2 步: CNOT 后处于 Bell 态, q1 测 1 概率也是 0.5。
	r, err = sess.Step(1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("第 2 步后: %+v", r)
	if math.Abs(r.Prob1-0.5) > 1e-9 {
		t.Fatalf("Bell 态下 q1 测 1 概率应为 0.5, 得到 %v", r.Prob1)
	}

	// 第 3 步: MEASURE 0 -> 0, c0 确定, c1 仍不确定。
	r, err = sess.Step(-1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("第 3 步后: %+v (不查询概率时 Prob1=-1)", r)
	if r.Prob1 != -1 {
		t.Fatalf("未查询时 Prob1 应为 -1, 得到 %v", r.Prob1)
	}
	if r.Bits[0] != 0 && r.Bits[0] != 1 {
		t.Fatalf("c0 应已确定为 0/1: %v", r.Bits)
	}
	if r.Bits[1] != -1 {
		t.Fatalf("c1 应仍未确定: %v", r.Bits)
	}

	// 跑完剩余操作, 与一次性 Run 对比。
	sessBits, err := sess.RunRemaining()
	if err != nil {
		t.Fatal(err)
	}
	runBits, err := s.Run(c, 42)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 同线路同种子, 逐步推进结果 %v 应等于一次性 Run 结果 %v", sessBits, runBits)
	if fmt.Sprintf("%v", sessBits) != fmt.Sprintf("%v", runBits) {
		t.Fatalf("逐步推进与一次性运行不一致: %v != %v", sessBits, runBits)
	}

	// 结束后再 Step 只返回 Done 快照。
	r, err = sess.Step(-1)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Done || r.Step != r.Total {
		t.Fatalf("结束后应返回 Done 快照: %+v", r)
	}
}

func TestSessionReproducibility(t *testing.T) {
	c := mustParse(t, sessionCircuit)
	s := New(20)
	collect := func() string {
		sess, err := s.NewSession(c, 7)
		if err != nil {
			t.Fatal(err)
		}
		out := ""
		for {
			r, err := sess.Step(0)
			if err != nil {
				t.Fatal(err)
			}
			out += fmt.Sprintf("%v/%.4f;", r.Bits, r.Prob1)
			if r.Done {
				break
			}
		}
		return out
	}
	a, b := collect(), collect()
	t.Logf("判定依据: 同线路同种子同样推进过程, 两次中间结果序列应完全一致\n  第 1 次: %s\n  第 2 次: %s", a, b)
	if a != b {
		t.Fatalf("会话结果不可复现:\n%s\n!=\n%s", a, b)
	}
}

func TestCheckpointResumeMatchesOneShot(t *testing.T) {
	c := mustParse(t, sessionCircuit)
	s := New(20)
	want, err := s.Run(c, 123)
	if err != nil {
		t.Fatal(err)
	}

	// 在每条操作之后都做一次检查点并续跑, 最终结果都必须等于一次性运行。
	for cut := 0; cut <= len(c.Ops); cut++ {
		sess, err := s.NewSession(c, 123)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < cut; i++ {
			if _, err := sess.Step(-1); err != nil {
				t.Fatal(err)
			}
		}
		cp := sess.Checkpoint()
		resumed, err := s.Resume(cp)
		if err != nil {
			t.Fatal(err)
		}
		got, err := resumed.RunRemaining()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("在第 %d/%d 步存档续跑: 得到 %v, 一次性运行 %v", cut, len(c.Ops), got, want)
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			t.Fatalf("第 %d 步检查点续跑结果 %v != 一次性运行 %v", cut, got, want)
		}
	}
}

func TestCheckpointDoesNotAffectOriginalSession(t *testing.T) {
	c := mustParse(t, sessionCircuit)
	s := New(20)
	sess, err := s.NewSession(c, 55)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Step(-1); err != nil { // H 0
		t.Fatal(err)
	}
	cp := sess.Checkpoint()
	// 从检查点恢复并跑完。
	resumed, err := s.Resume(cp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.RunRemaining(); err != nil {
		t.Fatal(err)
	}
	// 原会话继续跑, 结果仍须等于一次性运行。
	got, err := sess.RunRemaining()
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.Run(c, 55)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 检查点恢复不影响原会话, 原会话续跑 %v 应等于一次性运行 %v", got, want)
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("原会话被检查点污染: %v != %v", got, want)
	}
}

func TestConcurrentSessionsAndResume(t *testing.T) {
	c := mustParse(t, sessionCircuit)
	s := New(20)
	want, err := s.Run(c, 999)
	if err != nil {
		t.Fatal(err)
	}
	// 准备一个中间检查点, 供多个 goroutine 并发恢复。
	base, err := s.NewSession(c, 999)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := base.Step(-1); err != nil {
			t.Fatal(err)
		}
	}
	cp := base.Checkpoint()

	t.Logf("判定依据: 16 个 goroutine 并发推进各自会话并从同一检查点恢复, 结果都须等于一次性运行 %v", want)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			// 独立会话逐步跑完。
			sess, err := s.NewSession(c, 999)
			if err != nil {
				errs <- err
				return
			}
			for {
				r, err := sess.Step(g % 2) // 各自查询不同比特的概率
				if err != nil {
					errs <- err
					return
				}
				if r.Done {
					break
				}
			}
			// 从共享检查点恢复。
			resumed, err := s.Resume(cp)
			if err != nil {
				errs <- err
				return
			}
			got, err := resumed.RunRemaining()
			if err != nil {
				errs <- err
				return
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
				errs <- fmt.Errorf("goroutine %d 结果被污染: got %v want %v", g, got, want)
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestSessionRejectsInvalidAndOversized(t *testing.T) {
	s := New(20)

	// 重复操作数的非法线路: 解析期即拒绝。
	bad := "qubits 2\nbits 1\nCNOT 0 0\nMEASURE 0 -> 0\n"
	t.Logf("输入电路:\n%s", bad)
	if _, err := circuit.Parse(bad); err == nil {
		t.Fatal("重复操作数线路应在解析期被拒绝")
	} else {
		var e circuit.ErrDuplicateOperand
		t.Logf("判定依据: 解析错误 %v, errors.As(ErrDuplicateOperand)=%v", err, errors.As(err, &e))
		if !errors.As(err, &e) {
			t.Fatalf("期望 ErrDuplicateOperand, 得到 %T", err)
		}
	}

	// 手工构造的非法线路: 会话与一次性运行都要拒绝。
	manual := &circuit.Circuit{
		NumQubits: 2, NumBits: 1,
		Ops: []circuit.Operation{{Kind: circuit.OpGate, Gate: circuit.Gate{Name: "CNOT", Qubits: []int{1, 1}}}},
	}
	if _, err := s.NewSession(manual, 1); err == nil {
		t.Fatal("手工构造的重复操作数线路应被 NewSession 拒绝")
	} else {
		t.Logf("NewSession 拒绝手工非法线路: %v", err)
	}
	if _, err := s.Run(manual, 1); err == nil {
		t.Fatal("手工构造的重复操作数线路应被 Run 拒绝")
	}

	// 超出上限的线路: 会话同样被拒绝。
	big := mustParse(t, "qubits 25\nbits 1\nH 0\nMEASURE 0 -> 0\n")
	if _, err := s.NewSession(big, 1); err == nil {
		t.Fatal("超上限线路应被 NewSession 拒绝")
	} else {
		var e ErrTooManyQubits
		t.Logf("判定依据: 25 量子比特超出上限 20, NewSession 得到: %v", err)
		if !errors.As(err, &e) {
			t.Fatalf("期望 ErrTooManyQubits, 得到 %T", err)
		}
	}
}

func TestLimitNormalization(t *testing.T) {
	c := mustParse(t, bellCircuit)

	// 缺省保护: 未显式给出上限 (<=0) 时使用缺省值, 正常线路不受影响。
	for _, arg := range []int{0, -3} {
		s := New(arg)
		if s.MaxQubits != DefaultMaxQubits {
			t.Fatalf("New(%d) 应归一化为缺省上限 %d, 得到 %d", arg, DefaultMaxQubits, s.MaxQubits)
		}
		if _, err := s.Sample(c, 100, 1); err != nil {
			t.Fatalf("New(%d) 不应拒绝正常线路: %v", arg, err)
		}
		t.Logf("New(%d) 归一化为缺省上限 %d, Bell 线路采样正常", arg, DefaultMaxQubits)
	}

	// 全局硬上限: 调用方上限超过硬上限时被收敛, 而不是报错或放行。
	s := New(HardMaxQubits + 100)
	if s.MaxQubits != HardMaxQubits {
		t.Fatalf("超限上限应被收敛到硬上限 %d, 得到 %d", HardMaxQubits, s.MaxQubits)
	}
	t.Logf("New(%d) 被收敛到全局硬上限 %d", HardMaxQubits+100, HardMaxQubits)

	// 任何调用都无法突破硬上限: 构造超出硬上限的线路必须被拒绝。
	tooBig := &circuit.Circuit{
		NumQubits: HardMaxQubits + 1, NumBits: 1,
		Ops: []circuit.Operation{{Kind: circuit.OpMeasure, Qubit: 0, Bit: 0}},
	}
	_, err := s.Run(tooBig, 1)
	var e ErrTooManyQubits
	t.Logf("判定依据: %d 量子比特超出硬上限 %d, 得到: %v", HardMaxQubits+1, HardMaxQubits, err)
	if !errors.As(err, &e) || e.Limit != HardMaxQubits {
		t.Fatalf("期望 ErrTooManyQubits(Limit=%d), 得到 %v", HardMaxQubits, err)
	}

	// 缺省上限同样拦截: New(0) 下 21 量子比特线路被拒绝。
	over21 := &circuit.Circuit{
		NumQubits: DefaultMaxQubits + 1, NumBits: 1,
		Ops: []circuit.Operation{{Kind: circuit.OpMeasure, Qubit: 0, Bit: 0}},
	}
	if _, err := New(0).Run(over21, 1); !errors.As(err, &e) {
		t.Fatalf("缺省上限应拒绝 %d 量子比特线路, 得到 %v", DefaultMaxQubits+1, err)
	}
	t.Logf("缺省上限 %d 拦截 %d 量子比特线路", DefaultMaxQubits, DefaultMaxQubits+1)
}

func TestResumeHonorsLimit(t *testing.T) {
	c := mustParse(t, bellCircuit)
	big := New(20)
	sess, err := big.NewSession(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	cp := sess.Checkpoint()
	// 用量子比特上限更小的模拟器恢复同一检查点, 应被拒绝。
	small := New(1)
	if _, err := small.Resume(cp); err == nil {
		t.Fatal("上限不足的模拟器应拒绝恢复该检查点")
	} else {
		var e ErrTooManyQubits
		t.Logf("判定依据: 检查点线路需 2 量子比特, 上限 1 的模拟器恢复得到: %v", err)
		if !errors.As(err, &e) {
			t.Fatalf("期望 ErrTooManyQubits, 得到 %T", err)
		}
	}
}
