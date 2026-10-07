package sim

import (
	"fmt"
	"math/rand"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

// StepResult 是一次 Session.Step 推进后的快照。
type StepResult struct {
	Index      int    // 本步执行的操作下标（0 起）；已结束时为 -1
	Total      int    // 线路操作总数
	Done       bool   // 是否已执行完所有操作
	Bits       []int  // 当前经典比特数组（拷贝）
	Determined []bool // 各经典比特是否已被测量写入
}

// Session 是一次可逐步推进的执行会话。
// 会话独占自己的态矢量、经典比特与随机序列，互不共享，
// 因此多个会话可并发推进而互不污染。
type Session struct {
	c          *circuit.Circuit
	state      []complex128
	bits       []int
	determined []bool
	rng        *rand.Rand
	seed       int64
	draws      int // 已消耗的随机抽样次数（每次测量消耗一次）
	pc         int // 下一条待执行操作的下标
}

// Checkpoint 是某个中间时刻的执行状态快照，仅在进程内有效。
// 快照为深拷贝，可被多个会话并发恢复而互不影响。
type Checkpoint struct {
	c          *circuit.Circuit
	state      []complex128
	bits       []int
	determined []bool
	seed       int64
	draws      int
	pc         int
}

// NewSession 为线路 c 创建一个执行会话，随机序列由 seed 决定。
// 同一 (circuit, seed) 与同样的推进过程下，中间与最终结果完全可复现。
// 线路超出模拟器量子比特上限时返回 ErrTooManyQubits。
func (s *Simulator) NewSession(c *circuit.Circuit, seed int64) (*Session, error) {
	if err := s.checkSize(c); err != nil {
		return nil, err
	}
	size := 1 << uint(c.NumQubits)
	state := make([]complex128, size)
	state[0] = 1
	return &Session{
		c:          c,
		state:      state,
		bits:       make([]int, c.NumBits),
		determined: make([]bool, c.NumBits),
		rng:        rand.New(rand.NewSource(seed)),
		seed:       seed,
	}, nil
}

// Step 执行下一条操作并返回推进后的快照。
// 所有操作已执行完时不再推进，返回 Done 为 true 的快照（Index 为 -1）。
func (sess *Session) Step() (StepResult, error) {
	if sess.pc >= len(sess.c.Ops) {
		return sess.snapshot(-1), nil
	}
	op := sess.c.Ops[sess.pc]
	switch op.Kind {
	case circuit.OpGate:
		applyGate(sess.state, sess.c.NumQubits, op.Gate)
	case circuit.OpMeasure:
		sess.bits[op.Bit] = measure(sess.state, sess.c.NumQubits, op.Qubit, sess.rng)
		sess.draws++
		sess.determined[op.Bit] = true
	case circuit.OpConditional:
		if sess.bits[op.CondBit] == op.CondValue {
			applyGate(sess.state, sess.c.NumQubits, op.Gate)
		}
	}
	idx := sess.pc
	sess.pc++
	return sess.snapshot(idx), nil
}

// Progress 返回已执行操作数与总操作数。
func (sess *Session) Progress() (done, total int) {
	return sess.pc, len(sess.c.Ops)
}

// Bits 返回当前经典比特数组与各比特是否已被测量写入（均为拷贝）。
func (sess *Session) Bits() (bits []int, determined []bool) {
	bits = make([]int, len(sess.bits))
	copy(bits, sess.bits)
	determined = make([]bool, len(sess.determined))
	copy(determined, sess.determined)
	return bits, determined
}

// QubitProbability 返回当前状态下量子比特 q 测得 1 的概率。
func (sess *Session) QubitProbability(q int) (float64, error) {
	if q < 0 || q >= sess.c.NumQubits {
		return 0, circuit.ErrOutOfRange{What: "qubit", Index: q, Limit: sess.c.NumQubits}
	}
	mask := 1 << uint(q)
	var p1 float64
	for i := 0; i < len(sess.state); i++ {
		if i&mask != 0 {
			p1 += real(sess.state[i])*real(sess.state[i]) + imag(sess.state[i])*imag(sess.state[i])
		}
	}
	return p1, nil
}

// Checkpoint 把当前执行状态存为检查点（深拷贝）。
func (sess *Session) Checkpoint() *Checkpoint {
	cp := &Checkpoint{
		c:          sess.c,
		state:      make([]complex128, len(sess.state)),
		bits:       make([]int, len(sess.bits)),
		determined: make([]bool, len(sess.determined)),
		seed:       sess.seed,
		draws:      sess.draws,
		pc:         sess.pc,
	}
	copy(cp.state, sess.state)
	copy(cp.bits, sess.bits)
	copy(cp.determined, sess.determined)
	return cp
}

// Restore 从检查点恢复出一个新会话，可继续推进剩余操作。
// 恢复出的会话持有独立的状态拷贝与随机序列，与检查点及其他会话完全独立；
// 从同一检查点恢复并推进到结束，结果与一次性从头跑完一致。
// 检查点所属线路超出模拟器上限时返回 ErrTooManyQubits。
func (s *Simulator) Restore(cp *Checkpoint) (*Session, error) {
	if cp == nil {
		return nil, fmt.Errorf("nil checkpoint")
	}
	if err := s.checkSize(cp.c); err != nil {
		return nil, err
	}
	sess := &Session{
		c:          cp.c,
		state:      make([]complex128, len(cp.state)),
		bits:       make([]int, len(cp.bits)),
		determined: make([]bool, len(cp.determined)),
		rng:        rand.New(rand.NewSource(cp.seed)),
		seed:       cp.seed,
		draws:      cp.draws,
		pc:         cp.pc,
	}
	copy(sess.state, cp.state)
	copy(sess.bits, cp.bits)
	copy(sess.determined, cp.determined)
	// 随机源按种子重建后快进到检查点时刻已消耗的位置。
	for i := 0; i < cp.draws; i++ {
		sess.rng.Float64()
	}
	return sess, nil
}

// RunToEnd 从当前位置推进到线路结束，返回最终经典比特数组（拷贝）。
func (sess *Session) RunToEnd() ([]int, error) {
	for {
		r, err := sess.Step()
		if err != nil {
			return nil, err
		}
		if r.Done {
			bits, _ := sess.Bits()
			return bits, nil
		}
	}
}

// snapshot 生成当前状态的 StepResult 拷贝。index 为本步执行的操作下标，未执行时为 -1。
func (sess *Session) snapshot(index int) StepResult {
	bits, determined := sess.Bits()
	return StepResult{
		Index:      index,
		Total:      len(sess.c.Ops),
		Done:       sess.pc >= len(sess.c.Ops),
		Bits:       bits,
		Determined: determined,
	}
}
