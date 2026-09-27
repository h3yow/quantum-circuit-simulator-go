package qsim

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

const bellCircuit = `// Bell 态：H q0 后 CNOT q0,q1，测量两个比特
QUBITS 2
CBITS 2
H q[0]
CX q[0], q[1]
MEASURE q[0] -> c[0]
MEASURE q[1] -> c[1]
`

func TestSimulatorBellDistribution(t *testing.T) {
	t.Logf("输入电路:\n%s", bellCircuit)
	sim := New(0)
	res, err := sim.Run(bellCircuit, Options{Shots: 4000, Seed: 12345})
	if err != nil {
		t.Fatalf("判定依据: 期望运行成功，实际 %v", err)
	}
	t.Logf("判定依据: 4000 次采样计数=%v（理论上只应出现 00->0 与 11->3）", res.Counts)
	if res.Counts[0]+res.Counts[3] != 4000 {
		t.Fatalf("出现了 Bell 态不可能的结果: %v", res.Counts)
	}
	if len(res.Counts) != 2 {
		t.Fatalf("4000 次采样下两个相关结果都应出现，实际 %v", res.Counts)
	}
	// 各自频率应接近 0.5，允许 6% 偏差。
	for _, k := range []int{0, 3} {
		p := float64(res.Counts[k]) / 4000
		if math.Abs(p-0.5) > 0.06 {
			t.Fatalf("结果 %d 频率 %f 偏离理论值 0.5 过多", k, p)
		}
	}
}

func TestSimulatorSeedReproducibility(t *testing.T) {
	t.Logf("输入电路:\n%s", bellCircuit)
	sim := New(0)
	runs := make([]*Result, 4)
	for i := range runs {
		r, err := sim.Run(bellCircuit, Options{Shots: 500, Seed: 42})
		if err != nil {
			t.Fatal(err)
		}
		runs[i] = r
	}
	t.Logf("判定依据: 同种子 42 的四次独立调用计数均为 %v", runs[0].Counts)
	for i := 1; i < 4; i++ {
		if !mapsEq(runs[0].Counts, runs[i].Counts) {
			t.Fatalf("同种子结果不可复现: %v vs %v", runs[0].Counts, runs[i].Counts)
		}
	}
	// 不同种子通常得到不同计数；不强制必异，但这里对比两个分布。
	other, _ := sim.Run(bellCircuit, Options{Shots: 500, Seed: 43})
	t.Logf("判定依据: 种子 42=%v，种子 43=%v（二者均独立采样）", runs[0].Counts, other.Counts)

	// shot 级随机流派生不应让相邻 shot 系统性偏向某一侧：
	// 检查单 shot 序列整体仍近似平衡。
	r, _ := sim.Run(bellCircuit, Options{Shots: 2000, Seed: 42})
	if p := float64(r.Counts[3]) / 2000; math.Abs(p-0.5) > 0.06 {
		t.Fatalf("shot 派生随机流分布失衡: p(11)=%f", p)
	}
}

func TestSimulatorMidMeasurementBranch(t *testing.T) {
	// 先把 q0 置 1 并测量到 c0；条件成立时对 q1 做 X，再测 q1。
	// 最终 c0 被覆盖为 q1 的测量值，必然是 1。
	src := `
QUBITS 2
CBITS 1
X q[0]
MEASURE q[0] -> c[0]
IF (c[0]) X q[1]
MEASURE q[1] -> c[0]
`
	t.Logf("输入电路（条件应恒成立）:\n%s", src)
	sim := New(0)
	res, err := sim.Run(src, Options{Shots: 50, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 条件恒成立 -> c0 最终必为 1，计数=%v", res.Counts)
	if len(res.Counts) != 1 || res.Counts[1] != 50 {
		t.Fatalf("条件分支行为错误: %v", res.Counts)
	}
	// shot0 的事件序列：先测得 q0=1，再测得 q1=1。
	if len(res.Measurements) != 2 ||
		res.Measurements[0].Value != 1 || res.Measurements[1].Value != 1 {
		t.Fatalf("测量事件序列不符合预期: %+v", res.Measurements)
	}

	// 对照组：不翻转 q0，条件永不成立，q1 保持 0。
	srcFalse := `
QUBITS 2
CBITS 1
MEASURE q[0] -> c[0]
IF (c[0]) X q[1]
MEASURE q[1] -> c[0]
`
	t.Logf("输入电路（条件应恒不成立）:\n%s", srcFalse)
	res2, err := sim.Run(srcFalse, Options{Shots: 50, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 条件恒不成立 -> c0 最终必为 0，计数=%v", res2.Counts)
	if len(res2.Counts) != 1 || res2.Counts[0] != 50 {
		t.Fatalf("条件分支对照组行为错误: %v", res2.Counts)
	}
}

func TestSimulatorMidMeasurementCollapse(t *testing.T) {
	// H q0 后立即测量，测量把态坍缩；后续 CNOT 产生确定相关而非 50/50 纠缠。
	// 更精确的统计：测 q0 后再 CNOT 并测 q1，两者必须相等。
	src := `
QUBITS 2
CBITS 2
H q[0]
MEASURE q[0] -> c[0]
CX q[0], q[1]
MEASURE q[1] -> c[1]
`
	t.Logf("输入电路（中途测量后再纠缠）:\n%s", src)
	sim := New(0)
	res, err := sim.Run(src, Options{Shots: 2000, Seed: 99})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 坍缩后 q0 与 q1 必相等，只允许 00->0 与 11->3，计数=%v", res.Counts)
	if res.Counts[0]+res.Counts[3] != 2000 {
		t.Fatalf("中途测量坍缩语义错误，出现不相关结果: %v", res.Counts)
	}
	if math.Abs(float64(res.Counts[0])/2000-0.5) > 0.06 {
		t.Fatalf("坍缩分支先验概率失衡: %v", res.Counts)
	}
}

func TestSimulatorResourceExhausted(t *testing.T) {
	sim := New(0)
	src := "QUBITS 30\n"
	t.Logf("输入电路:\n%s", src)
	_, err := sim.Run(src, Options{Shots: 1})
	assertErrorKind(t, src, err, KindResourceExhausted)

	src2 := "QUBITS 5\nH q[0]\n"
	t.Logf("输入电路:\n%s判定依据: Options.MaxQubits=3 应在运行期拒绝", src2)
	_, err = sim.Run(src2, Options{Shots: 1, MaxQubits: 3})
	assertErrorKind(t, src2, err, KindResourceExhausted)
}

func TestSimulatorInvalidOptions(t *testing.T) {
	sim := New(0)
	_, err := sim.Run("QUBITS 1\nH q[0]\n", Options{Shots: 0})
	var ce *CircuitError
	if !errors.As(err, &ce) || ce.Kind != KindInvalidArg {
		t.Fatalf("判定依据: Shots=0 应返回 INVALID_ARG，实际 %v", err)
	}
	t.Logf("判定依据: Shots=0 被拒绝: %v", err)
}

func TestSimulatorConcurrentRuns(t *testing.T) {
	sim := New(0)
	const workers = 32
	const shots = 200
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			seed := uint64(1000 + id%4) // 多 goroutine 复用同一种子
			res, err := sim.Run(bellCircuit, Options{Shots: shots, Seed: seed})
			if err != nil {
				errCh <- err
				return
			}
			if res.Counts[0]+res.Counts[3] != shots {
				errCh <- fmt.Errorf("worker %d got illegal outcomes %v", id, res.Counts)
			}
			// 与单线程同种子结果逐一比对
			ref, err := sim.Run(bellCircuit, Options{Shots: shots, Seed: seed})
			if err != nil {
				errCh <- err
				return
			}
			if !mapsEq(res.Counts, ref.Counts) {
				errCh <- fmt.Errorf("worker %d seed %d 并发结果与串行不一致: %v vs %v",
					id, seed, res.Counts, ref.Counts)
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("判定依据: 并发运行出现问题: %v", err)
	}
	t.Logf("判定依据: %d 个 goroutine 复用同一模拟器，结果均合法且与同种子串行调用一致", workers)
}

func mapsEq(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
