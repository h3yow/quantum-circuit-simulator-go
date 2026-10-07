package sim

import (
	"math/rand"

	"github.com/h3yow/quantum-circuit-simulator-go/circuit"
)

// StepResult 描述会话推进一步后的快照。
type StepResult struct {
	Step  int     // 已执行的操作数
	Total int     // 线路总操作数
	Done  bool    // 是否已执行完所有操作
	Bits  []int   // 经典比特当前值，未确定的位为 -1
	Prob1 float64 // 本次查询的量子比特测得 1 的概率；未查询时为 -1
}

// Checkpoint 是某个中间时刻的执行状态快照。
// 仅在进程内复用；通过 Resume 可恢复出多个互不影响的会话。
type Checkpoint struct {
	circ  *circuit.Circuit
	state []complex128 // 态矢量副本
	bits  []int        // 经典比特当前值副本
	set   []bool       // 各经典比特是否已确定
	rng   uint64       // 随机源状态快照
	step  int          // 已执行的操作数
}

// Session 是一次可逐步推进的执行会话。
// 会话状态完全私有，不同会话（含同一检查点恢复出的）互不共享数据；
// 单个 Session 本身不是并发安全的，不要跨 goroutine 共享同一会话。
type Session struct {
	sim   *Simulator
	circ  *circuit.Circuit
	state []complex128
	bits  []int
	set   []bool
	rng   *rand.Rand
	src   *snapSource // rng 的底层源，用于检查点快照
	step  int
}

// NewSession 以给定种子为线路创建执行会话。
// 与 Run 一样受结构校验与量子比特上限约束；
// 同一线路、同一种子、同样的推进过程，结果与 Run(c, seed) 完全可复现。
func (s *Simulator) NewSession(c *circuit.Circuit, seed int64) (*Session, error) {
	if err := circuit.Validate(c); err != nil {
		return nil, err
	}
	if err := s.checkSize(c); err != nil {
		return nil, err
	}
	n := c.NumQubits
	state := make([]complex128, 1<<uint(n))
	state[0] = 1
	rng, src := newRNG(seed)
	return &Session{
		sim:   s,
		circ:  c,
		state: state,
		bits:  make([]int, c.NumBits),
		set:   make([]bool, c.NumBits),
		rng:   rng,
		src:   src,
	}, nil
}

// Step 推进一条操作，返回当前快照。
// queryQubit >= 0 时附带该量子比特在当前态下测得 1 的概率（不坍缩态矢量）；
// queryQubit < 0 表示不查询，Prob1 为 -1。线路已执行完时继续调用只返回 Done 快照。
func (sess *Session) Step(queryQubit int) (StepResult, error) {
	if queryQubit >= sess.circ.NumQubits {
		return StepResult{}, circuit.ErrOutOfRange{What: "qubit", Index: queryQubit, Limit: sess.circ.NumQubits}
	}
	if sess.step < len(sess.circ.Ops) {
		op := sess.circ.Ops[sess.step]
		if bit := execOp(sess.state, sess.circ.NumQubits, sess.bits, op, sess.rng); bit >= 0 {
			sess.set[bit] = true
		}
		sess.step++
	}
	res := StepResult{
		Step:  sess.step,
		Total: len(sess.circ.Ops),
		Done:  sess.step == len(sess.circ.Ops),
		Bits:  make([]int, len(sess.bits)),
		Prob1: -1,
	}
	for i := range sess.bits {
		if sess.set[i] {
			res.Bits[i] = sess.bits[i]
		} else {
			res.Bits[i] = -1
		}
	}
	if queryQubit >= 0 {
		res.Prob1 = probOfOne(sess.state, queryQubit)
	}
	return res, nil
}

// Checkpoint 保存当前执行状态。快照为深拷贝，
// 之后继续推进原会话或从快照恢复新会话都互不影响。
func (sess *Session) Checkpoint() *Checkpoint {
	return &Checkpoint{
		circ:  sess.circ,
		state: append([]complex128(nil), sess.state...),
		bits:  append([]int(nil), sess.bits...),
		set:   append([]bool(nil), sess.set...),
		rng:   sess.src.snapshot(),
		step:  sess.step,
	}
}

// Resume 从检查点恢复出一个新会话，可继续推进剩余操作。
// 恢复出的会话持有独立的状态副本，同一检查点可被并发恢复多次而互不污染；
// 恢复同样受本模拟器的量子比特上限约束。
func (s *Simulator) Resume(cp *Checkpoint) (*Session, error) {
	if err := s.checkSize(cp.circ); err != nil {
		return nil, err
	}
	src := newSnapSource(0)
	src.restore(cp.rng)
	return &Session{
		sim:   s,
		circ:  cp.circ,
		state: append([]complex128(nil), cp.state...),
		bits:  append([]int(nil), cp.bits...),
		set:   append([]bool(nil), cp.set...),
		rng:   rand.New(src),
		src:   src,
		step:  cp.step,
	}, nil
}

// RunRemaining 从当前位置跑完剩余操作，返回最终经典比特数组。
// 与 Run 一致：未被测量过的经典比特取 0。
func (sess *Session) RunRemaining() ([]int, error) {
	for sess.step < len(sess.circ.Ops) {
		if _, err := sess.Step(-1); err != nil {
			return nil, err
		}
	}
	return append([]int(nil), sess.bits...), nil
}
