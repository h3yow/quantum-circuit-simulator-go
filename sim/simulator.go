// Package sim 提供量子线路的态矢量模拟与采样。
//
// 语义约定：
//   - 量子比特 q 对应态矢量下标的第 q 位（小端）。
//   - 经典比特串按声明顺序输出，bit 0 在最左。
//   - 每次运行使用独立的伪随机源，Simulator 本身不保存运行态，
//     因此可被多个 goroutine 并发复用且互不污染。
package sim

import (
	"fmt"
	"math"
	"math/cmplx"
	"math/rand"
	"strings"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

// 资源上限规则：
//   - 调用方未显式给出上限（maxQubits <= 0）时使用 DefaultMaxQubits；
//   - 任何调用都无法突破 HardMaxQubits，超出时钳制到该值；
//   - 不合理的上限按上述规则统一钳制，不会因此拒绝合法线路。
const (
	// DefaultMaxQubits 是未显式指定上限时的缺省量子比特数上限。
	DefaultMaxQubits = 20
	// HardMaxQubits 是全局硬上限，任何调用都无法突破
	// （2^30 个 complex128 已达 16 GiB，超出无实际意义）。
	HardMaxQubits = 30
)

// ErrTooManyQubits 表示态矢量所需内存超出允许范围。
type ErrTooManyQubits struct {
	Qubits int
	Limit  int
}

func (e ErrTooManyQubits) Error() string {
	return fmt.Sprintf("circuit needs %d qubits, exceeds limit %d (state vector would need 2^%d complex128 entries)",
		e.Qubits, e.Limit, e.Qubits)
}

// Result 是采样运行的输出。
type Result struct {
	Shots  int
	Counts map[string]int // 各经典比特串出现次数
}

// Simulator 可并发复用；内部不保存任何运行态。
type Simulator struct {
	MaxQubits int // 生效的量子比特数上限（已经过钳制），超出返回 ErrTooManyQubits
}

// New 创建一个模拟器。
//
// maxQubits 为期望的量子比特数上限，按统一规则钳制：
//   - maxQubits <= 0：视为未指定，使用 DefaultMaxQubits；
//   - maxQubits > HardMaxQubits：钳制到 HardMaxQubits。
//
// 钳制后的生效值保存在返回实例的 MaxQubits 字段中，调用方可直接读取确认。
func New(maxQubits int) *Simulator {
	if maxQubits <= 0 {
		maxQubits = DefaultMaxQubits
	}
	if maxQubits > HardMaxQubits {
		maxQubits = HardMaxQubits
	}
	return &Simulator{MaxQubits: maxQubits}
}

// Run 以给定随机种子运行线路一次，返回最终经典比特数组。
// 同一 (circuit, seed) 的结果完全可复现。
func (s *Simulator) Run(c *circuit.Circuit, seed int64) ([]int, error) {
	rng := rand.New(rand.NewSource(seed))
	return s.runOnce(c, rng)
}

// Sample 以给定种子采样 shots 次，返回各比特串计数。
// 同一种子下计数序列完全可复现。
func (s *Simulator) Sample(c *circuit.Circuit, shots int, seed int64) (*Result, error) {
	if shots <= 0 {
		return nil, fmt.Errorf("shots must be positive, got %d", shots)
	}
	if err := s.checkSize(c); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewSource(seed))
	res := &Result{Shots: shots, Counts: map[string]int{}}
	for i := 0; i < shots; i++ {
		bits, err := s.runOnce(c, rng)
		if err != nil {
			return nil, err
		}
		res.Counts[bitString(bits)]++
	}
	return res, nil
}

func (s *Simulator) checkSize(c *circuit.Circuit) error {
	if c.NumQubits > s.MaxQubits {
		return ErrTooManyQubits{Qubits: c.NumQubits, Limit: s.MaxQubits}
	}
	return nil
}

// runOnce 演化一次完整线路。rng 由调用方持有，保证 Sample 内多次运行共享同一随机序列。
func (s *Simulator) runOnce(c *circuit.Circuit, rng *rand.Rand) ([]int, error) {
	if err := s.checkSize(c); err != nil {
		return nil, err
	}
	n := c.NumQubits
	size := 1 << uint(n)
	state := make([]complex128, size)
	state[0] = 1
	bits := make([]int, c.NumBits)

	for _, op := range c.Ops {
		switch op.Kind {
		case circuit.OpGate:
			applyGate(state, n, op.Gate)
		case circuit.OpMeasure:
			bits[op.Bit] = measure(state, n, op.Qubit, rng)
		case circuit.OpConditional:
			if bits[op.CondBit] == op.CondValue {
				applyGate(state, n, op.Gate)
			}
		}
	}
	return bits, nil
}

func bitString(bits []int) string {
	var sb strings.Builder
	for _, b := range bits {
		sb.WriteByte(byte('0' + b))
	}
	return sb.String()
}

// gateMatrix 返回门的 1 或 2 比特矩阵（行主序）。
func gateMatrix(g circuit.Gate) []complex128 {
	i := complex(0, 1)
	switch g.Name {
	case "H":
		s := complex(1/math.Sqrt2, 0)
		return []complex128{s, s, s, -s}
	case "X":
		return []complex128{0, 1, 1, 0}
	case "Y":
		return []complex128{0, -i, i, 0}
	case "Z":
		return []complex128{1, 0, 0, -1}
	case "S":
		return []complex128{1, 0, 0, i}
	case "T":
		return []complex128{1, 0, 0, cmplx.Exp(i * math.Pi / 4)}
	case "RX":
		c := complex(math.Cos(g.Param/2), 0)
		s := complex(0, -math.Sin(g.Param/2))
		return []complex128{c, s, s, c}
	case "RY":
		c := complex(math.Cos(g.Param/2), 0)
		s := complex(math.Sin(g.Param/2), 0)
		return []complex128{c, -s, s, c}
	case "RZ":
		return []complex128{cmplx.Exp(-i * complex(g.Param/2, 0)), 0, 0, cmplx.Exp(i * complex(g.Param/2, 0))}
	}
	return nil
}

// applyGate 将门作用到态矢量上（原地修改）。
func applyGate(state []complex128, n int, g circuit.Gate) {
	switch g.Name {
	case "CNOT":
		applyCNOT(state, n, g.Qubits[0], g.Qubits[1])
	case "CZ":
		applyCZ(state, n, g.Qubits[0], g.Qubits[1])
	case "SWAP":
		applySWAP(state, n, g.Qubits[0], g.Qubits[1])
	default:
		applySingle(state, n, g.Qubits[0], gateMatrix(g))
	}
}

// applySingle 对量子比特 q 作用 2x2 矩阵 m。
func applySingle(state []complex128, n, q int, m []complex128) {
	step := 1 << uint(q)
	for base := 0; base < len(state); base += 2 * step {
		for off := 0; off < step; off++ {
			i0 := base + off
			i1 := i0 + step
			a0, a1 := state[i0], state[i1]
			state[i0] = m[0]*a0 + m[1]*a1
			state[i1] = m[2]*a0 + m[3]*a1
		}
	}
}

// applyCNOT 以 ctrl 为控制位、tgt 为目标位应用 CNOT。
func applyCNOT(state []complex128, n, ctrl, tgt int) {
	cmask := 1 << uint(ctrl)
	tmask := 1 << uint(tgt)
	for i := 0; i < len(state); i++ {
		if i&cmask != 0 && i&tmask == 0 {
			state[i], state[i|tmask] = state[i|tmask], state[i]
		}
	}
}

// applyCZ 应用受控 Z。
func applyCZ(state []complex128, n, ctrl, tgt int) {
	mask := 1<<uint(ctrl) | 1<<uint(tgt)
	for i := 0; i < len(state); i++ {
		if i&mask == mask {
			state[i] = -state[i]
		}
	}
}

// applySWAP 交换两个量子比特。
func applySWAP(state []complex128, n, a, b int) {
	if a == b {
		return
	}
	amask := 1 << uint(a)
	bmask := 1 << uint(b)
	for i := 0; i < len(state); i++ {
		if i&amask != 0 && i&bmask == 0 {
			j := (i &^ amask) | bmask
			state[i], state[j] = state[j], state[i]
		}
	}
}

// measure 测量量子比特 q：按当前态的概率分布抽样，坍缩并归一化，返回 0 或 1。
func measure(state []complex128, n, q int, rng *rand.Rand) int {
	mask := 1 << uint(q)
	var p1 float64
	for i := 0; i < len(state); i++ {
		if i&mask != 0 {
			p1 += real(state[i])*real(state[i]) + imag(state[i])*imag(state[i])
		}
	}
	outcome := 0
	if rng.Float64() < p1 {
		outcome = 1
	}
	// 坍缩：清零另一分支，归一化保留分支。
	var norm float64
	if outcome == 1 {
		norm = math.Sqrt(p1)
	} else {
		norm = math.Sqrt(1 - p1)
	}
	if norm == 0 {
		return outcome // 数值上概率为 0 的分支不会被抽到，防御性返回
	}
	for i := 0; i < len(state); i++ {
		bit := 0
		if i&mask != 0 {
			bit = 1
		}
		if bit != outcome {
			state[i] = 0
		} else {
			state[i] /= complex(norm, 0)
		}
	}
	return outcome
}
