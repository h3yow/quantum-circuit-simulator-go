package sim

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

// midMeasureCircuit 含中途测量与条件操作，用于检验检查点对随机序列的精确恢复。
const midMeasureCircuit = `
qubits 2
bits 2
H 0
MEASURE 0 -> 0
IF 0==1 X 1
H 1
MEASURE 1 -> 1
`

func TestSessionStepByStepMatchesRun(t *testing.T) {
	c := mustParse(t, midMeasureCircuit)
	s := New(20)
	want, err := s.Run(c, 42)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.NewSession(c, 42)
	if err != nil {
		t.Fatal(err)
	}
	var last StepResult
	for {
		last, err = sess.Step()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("步进: index=%d done=%v bits=%v determined=%v", last.Index, last.Done, last.Bits, last.Determined)
		if last.Done {
			break
		}
	}
	t.Logf("判定依据: 逐步推进最终结果 %v 应与一次性 Run 结果 %v 完全一致", last.Bits, want)
	if fmt.Sprintf("%v", last.Bits) != fmt.Sprintf("%v", want) {
		t.Fatalf("逐步推进结果 %v != 一次性运行结果 %v", last.Bits, want)
	}
	// 结束后再 Step 应为幂等空操作
	again, err := sess.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !again.Done || again.Index != -1 {
		t.Fatalf("结束后 Step 应为幂等 Done, 得到 %+v", again)
	}
}

func TestSessionIntermediateState(t *testing.T) {
	input := `
qubits 2
bits 2
X 0
H 1
MEASURE 0 -> 0
MEASURE 1 -> 1
`
	c := mustParse(t, input)
	s := New(20)
	sess, err := s.NewSession(c, 1)
	if err != nil {
		t.Fatal(err)
	}

	done, total := sess.Progress()
	t.Logf("初始进度: %d/%d", done, total)
	if done != 0 || total != 4 {
		t.Fatalf("初始进度应为 0/4, 得到 %d/%d", done, total)
	}

	// 第 1 步 X 0 后 q0 必为 1, q1 必为 0
	if _, err := sess.Step(); err != nil {
		t.Fatal(err)
	}
	p0, _ := sess.QubitProbability(0)
	p1, _ := sess.QubitProbability(1)
	t.Logf("X 0 后: P(q0=1)=%.4f P(q1=1)=%.4f (判定依据: 应为 1 与 0)", p0, p1)
	if math.Abs(p0-1) > 1e-9 || math.Abs(p1) > 1e-9 {
		t.Fatalf("X 0 后概率错误: P(q0=1)=%v P(q1=1)=%v", p0, p1)
	}

	// 第 2 步 H 1 后 q1 概率 0.5
	if _, err := sess.Step(); err != nil {
		t.Fatal(err)
	}
	p1, _ = sess.QubitProbability(1)
	t.Logf("H 1 后: P(q1=1)=%.4f (判定依据: 应为 0.5)", p1)
	if math.Abs(p1-0.5) > 1e-9 {
		t.Fatalf("H 后概率应为 0.5, 得到 %v", p1)
	}

	// 测量前经典比特均未确定
	_, determined := sess.Bits()
	if determined[0] || determined[1] {
		t.Fatalf("测量前经典比特不应已确定: %v", determined)
	}

	// 第 3 步测量 q0: 结果必为 1 且 bit0 已确定
	r, err := sess.Step()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("测量 q0 后: bits=%v determined=%v (判定依据: bit0 必为 1 且已确定)", r.Bits, r.Determined)
	if r.Bits[0] != 1 || !r.Determined[0] || r.Determined[1] {
		t.Fatalf("测量后经典比特状态错误: bits=%v determined=%v", r.Bits, r.Determined)
	}

	// 越界查询应返回可区分错误
	_, err = sess.QubitProbability(5)
	t.Logf("越界概率查询得到错误: %v (判定依据: 应为 circuit.ErrOutOfRange)", err)
	var oor = circuit.ErrOutOfRange{}
	if !errors.As(err, &oor) {
		t.Fatalf("越界量子比特查询应返回 ErrOutOfRange, 得到 %T: %v", err, err)
	}
}

func TestSessionReproducibility(t *testing.T) {
	c := mustParse(t, midMeasureCircuit)
	s := New(20)
	s1, _ := s.NewSession(c, 7)
	s2, _ := s.NewSession(c, 7)
	step := 0
	for {
		r1, err := s1.Step()
		if err != nil {
			t.Fatal(err)
		}
		r2, err := s2.Step()
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%+v", r1) != fmt.Sprintf("%+v", r2) {
			t.Fatalf("第 %d 步两个同种子会话不一致: %+v vs %+v", step, r1, r2)
		}
		t.Logf("第 %d 步两会话快照一致: bits=%v determined=%v", step, r1.Bits, r1.Determined)
		if r1.Done {
			break
		}
		step++
	}
	t.Logf("判定依据: 同 (线路, 种子, 推进过程) 下两个会话每步快照完全一致")
}

func TestSessionConcurrent(t *testing.T) {
	c := mustParse(t, midMeasureCircuit)
	s := New(20)
	want, err := s.Run(c, 123)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 16 个 goroutine 各自推进独立会话, 最终结果须与串行基准 %v 一致", want)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			sess, err := s.NewSession(c, 123)
			if err != nil {
				errs <- err
				return
			}
			bits, err := sess.RunToEnd()
			if err != nil {
				errs <- err
				return
			}
			if fmt.Sprintf("%v", bits) != fmt.Sprintf("%v", want) {
				errs <- fmt.Errorf("goroutine %d 结果被污染: got %v want %v", g, bits, want)
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestCheckpointResumeMatchesOneShot(t *testing.T) {
	c := mustParse(t, midMeasureCircuit)
	s := New(20)
	const seed = 99
	want, err := s.Run(c, seed)
	if err != nil {
		t.Fatal(err)
	}
	// 在线路的每个中间时刻存检查点并续跑, 结果都必须与一次性跑完一致
	for cut := 0; cut <= len(c.Ops); cut++ {
		sess, err := s.NewSession(c, seed)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < cut; i++ {
			if _, err := sess.Step(); err != nil {
				t.Fatal(err)
			}
		}
		cp := sess.Checkpoint()
		done, _ := sess.Progress()
		resumed, err := s.Restore(cp)
		if err != nil {
			t.Fatal(err)
		}
		got, err := resumed.RunToEnd()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("在 %d/%d 处存档续跑: got %v, 一次性基准 %v", done, len(c.Ops), got, want)
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			t.Fatalf("在 %d 处存档续跑结果 %v != 一次性结果 %v", cut, got, want)
		}
	}
	t.Logf("判定依据: 任意中间时刻(含测量后, 随机序列已消耗)存档续跑, 经典比特结果都与一次性跑完一致")
}

func TestCheckpointConcurrentRestore(t *testing.T) {
	c := mustParse(t, midMeasureCircuit)
	s := New(20)
	const seed = 5
	// 在第一次测量之后存档, 确保随机序列状态也被检验
	sess, err := s.NewSession(c, seed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Step(); err != nil { // H 0
		t.Fatal(err)
	}
	if _, err := sess.Step(); err != nil { // MEASURE 0 -> 0
		t.Fatal(err)
	}
	cp := sess.Checkpoint()
	// 基准: 串行恢复一次并跑完
	base, err := s.Restore(cp)
	if err != nil {
		t.Fatal(err)
	}
	want, err := base.RunToEnd()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 8 个 goroutine 并发恢复同一检查点, 结果须全部等于串行基准 %v 且互不污染", want)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			rs, err := s.Restore(cp)
			if err != nil {
				errs <- err
				return
			}
			// 先各自多走一步, 验证彼此状态独立
			if _, err := rs.Step(); err != nil {
				errs <- err
				return
			}
			got, err := rs.RunToEnd()
			if err != nil {
				errs <- err
				return
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
				errs <- fmt.Errorf("goroutine %d 恢复结果被污染: got %v want %v", g, got, want)
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	// 检查点本身不被恢复过程修改: 再次恢复仍得同一基准
	again, err := s.Restore(cp)
	if err != nil {
		t.Fatal(err)
	}
	got, err := again.RunToEnd()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("并发恢复后再次从检查点恢复: got %v, 基准 %v", got, want)
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("检查点被恢复过程污染: got %v want %v", got, want)
	}
}

func TestSessionRespectsLimits(t *testing.T) {
	c := mustParse(t, `
qubits 25
bits 1
H 0
MEASURE 0 -> 0
`)
	s := New(20)
	_, err := s.NewSession(c, 1)
	t.Logf("判定依据: 25 量子比特超出上限 20, NewSession 得到错误: %v", err)
	var e ErrTooManyQubits
	if !errors.As(err, &e) {
		t.Fatalf("NewSession 期望 ErrTooManyQubits, 得到 %T: %v", err, err)
	}

	// 用大上限模拟器建的检查点, 恢复到小上限模拟器同样要被拒绝
	big := New(30)
	sess, err := big.NewSession(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	cp := sess.Checkpoint()
	_, err = s.Restore(cp)
	t.Logf("判定依据: 超限线路的检查点恢复到小上限模拟器, 得到错误: %v", err)
	if !errors.As(err, &e) {
		t.Fatalf("Restore 期望 ErrTooManyQubits, 得到 %T: %v", err, err)
	}
}
