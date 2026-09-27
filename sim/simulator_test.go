package sim

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

const bellCircuit = `
qubits 2
bits 2
H 0
CNOT 0 1
MEASURE 0 -> 0
MEASURE 1 -> 1
`

func mustParse(t *testing.T, input string) *circuit.Circuit {
	t.Helper()
	t.Logf("输入电路:\n%s", input)
	c, err := circuit.Parse(input)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	return c
}

func TestBellStateDistribution(t *testing.T) {
	c := mustParse(t, bellCircuit)
	s := New(20)
	res, err := s.Sample(c, 4000, 42)
	if err != nil {
		t.Fatalf("采样失败: %v", err)
	}
	t.Logf("判定依据: Bell 态只应出现 00 与 11, 各约 50%%, 实测计数: %v", res.Counts)
	if len(res.Counts) != 2 {
		t.Fatalf("Bell 态应只有 2 种结果, 得到 %v", res.Counts)
	}
	for k, v := range res.Counts {
		if k != "00" && k != "11" {
			t.Fatalf("出现不可能的结果 %q", k)
		}
		if v < 1800 || v > 2200 {
			t.Fatalf("结果 %q 计数 %d 偏离 50%% 过多", k, v)
		}
	}
}

func TestSeedReproducibility(t *testing.T) {
	c := mustParse(t, bellCircuit)
	s := New(20)
	r1, err := s.Sample(c, 500, 7)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Sample(c, 500, 7)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 同一种子两次采样计数应完全一致: %v vs %v", r1.Counts, r2.Counts)
	if fmt.Sprintf("%v", r1.Counts) != fmt.Sprintf("%v", r2.Counts) {
		t.Fatalf("同一种子结果不一致: %v != %v", r1.Counts, r2.Counts)
	}
	r3, err := s.Sample(c, 500, 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("不同种子计数: seed=7 %v, seed=8 %v (允许不同, 分布仍须只有 00/11)", r1.Counts, r3.Counts)
	for k := range r3.Counts {
		if k != "00" && k != "11" {
			t.Fatalf("不同种子下出现不可能结果 %q", k)
		}
	}
}

func TestMidCircuitMeasurementAndConditional(t *testing.T) {
	// 测量 q0 写入 c0；若 c0==1 则对 q1 施加 X。
	// q0 经 H 后测量为 1 时 q1 被翻转 -> c1 == c0 恒成立。
	input := `
qubits 2
bits 2
H 0
MEASURE 0 -> 0
IF 0==1 X 1
MEASURE 1 -> 1
`
	c := mustParse(t, input)
	s := New(20)
	res, err := s.Sample(c, 2000, 99)
	if err != nil {
		t.Fatalf("采样失败: %v", err)
	}
	t.Logf("判定依据: 条件 X 使 c1 跟随 c0, 只应出现 00 与 11, 实测: %v", res.Counts)
	for k := range res.Counts {
		if k != "00" && k != "11" {
			t.Fatalf("条件分支未生效, 出现结果 %q", k)
		}
	}

	// 对照组: 条件为 c0==0 时翻转, 结果应只出现 01 与 10。
	input2 := `
qubits 2
bits 2
H 0
MEASURE 0 -> 0
IF 0==0 X 1
MEASURE 1 -> 1
`
	c2 := mustParse(t, input2)
	res2, err := s.Sample(c2, 2000, 99)
	if err != nil {
		t.Fatalf("采样失败: %v", err)
	}
	t.Logf("对照组判定依据: 条件 c0==0 翻转, 只应出现 01 与 10, 实测: %v", res2.Counts)
	for k := range res2.Counts {
		if k != "01" && k != "10" {
			t.Fatalf("对照组条件分支未生效, 出现结果 %q", k)
		}
	}
}

func TestDeterministicCircuit(t *testing.T) {
	// 无随机性: X 0 后测量必为 1。
	input := `
qubits 1
bits 1
X 0
MEASURE 0 -> 0
`
	c := mustParse(t, input)
	s := New(20)
	res, err := s.Sample(c, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: X|0>=|1>, 测量必为 1, 实测: %v", res.Counts)
	if res.Counts["1"] != 100 {
		t.Fatalf("确定性线路结果错误: %v", res.Counts)
	}
}

func TestTooManyQubits(t *testing.T) {
	input := `
qubits 25
bits 1
H 0
MEASURE 0 -> 0
`
	c := mustParse(t, input)
	s := New(20) // 上限 20 个量子比特
	_, err := s.Run(c, 1)
	t.Logf("判定依据: 25 量子比特超出上限 20, 得到错误: %v", err)
	var e ErrTooManyQubits
	if !errors.As(err, &e) {
		t.Fatalf("期望 ErrTooManyQubits, 得到 %T: %v", err, err)
	}
	if _, err := s.Sample(c, 10, 1); !errors.As(err, &e) {
		t.Fatalf("Sample 同样应拒绝: %v", err)
	}
}

func TestConcurrentReuse(t *testing.T) {
	c := mustParse(t, bellCircuit)
	s := New(20)
	// 先算出基准结果
	want, err := s.Sample(c, 200, 123)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("判定依据: 16 个 goroutine 并发复用同一 Simulator, 结果须与串行基准 %v 完全一致", want.Counts)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				got, err := s.Sample(c, 200, 123)
				if err != nil {
					errs <- err
					return
				}
				if fmt.Sprintf("%v", got.Counts) != fmt.Sprintf("%v", want.Counts) {
					errs <- fmt.Errorf("并发结果被污染: got %v want %v", got.Counts, want.Counts)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
