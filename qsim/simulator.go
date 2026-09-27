package qsim

import (
	"math/rand/v2"
)

// 默认资源上限：24 量子比特 => 2^24 * 16B = 256MiB 态向量。
const defaultMaxQubits = 24

// Result 是批量采样的聚合结果。
type Result struct {
	// Shots 实际执行的运行次数。
	Shots int
	// Seed 本次采样使用的随机种子（回显，便于复现）。
	Seed uint64
	// Counts 统计每次运行结束时经典寄存器的取值出现次数：
	// 键为经典寄存器的二进制编码（c[0] 为最低位）。
	Counts map[int]int
	// Measurements 记录第 1 次运行（shot 0）中的全部测量事件，
	// 按发生顺序排列，用于解释中途测量与条件分支的行为。
	Measurements []Measurement
}

// Measurement 记录一次中途/结尾测量事件。
type Measurement struct {
	Qubit int
	Clbit int
	Value int
}

// Options 控制一次批量采样。
type Options struct {
	// Shots 运行次数，必须 >= 1。
	Shots int
	// Seed 随机种子；同电路、同种子、同次数的结果完全一致。
	Seed uint64
	// MaxQubits 允许模拟的最大量子比特数；<= 0 表示使用模拟器默认值。
	MaxQubits int
}

// Simulator 可被多个 goroutine 并发复用；它本身不可变，
// 每次运行都在独立的态向量与随机源上进行，互不污染。
type Simulator struct {
	maxQubits int
}

// New 创建模拟器；maxQubits <= 0 时使用默认上限 24。
func New(maxQubits int) *Simulator {
	if maxQubits <= 0 {
		maxQubits = defaultMaxQubits
	}
	return &Simulator{maxQubits: maxQubits}
}

// Run 解析电路文本并执行 opts.Shots 次独立采样。
func (s *Simulator) Run(src string, opts Options) (*Result, error) {
	c, err := Parse(src)
	if err != nil {
		return nil, err
	}
	return s.RunCircuit(c, opts)
}

// RunCircuit 对已解析电路执行采样。
func (s *Simulator) RunCircuit(c *Circuit, opts Options) (*Result, error) {
	if c == nil {
		return nil, kindError(KindInvalidArg, "电路为空")
	}
	if opts.Shots < 1 {
		return nil, kindError(KindInvalidArg, "Shots 必须 >= 1，得到 %d", opts.Shots)
	}
	maxQ := s.maxQubits
	if opts.MaxQubits > 0 {
		maxQ = opts.MaxQubits
	}
	// 防御性校验：调用方可能手工构造 Circuit 而未经过 Parse。
	if err := validate(c); err != nil {
		return nil, err
	}

	res := &Result{
		Shots:  opts.Shots,
		Seed:   opts.Seed,
		Counts: make(map[int]int),
	}
	for shot := 0; shot < opts.Shots; shot++ {
		events, reg, err := s.runShot(c, maxQ, opts.Seed, uint64(shot))
		if err != nil {
			return nil, err
		}
		if shot == 0 {
			res.Measurements = events
		}
		v := 0
		for i, b := range reg {
			if b == 1 {
				v |= 1 << uint(i)
			}
		}
		res.Counts[v]++
	}
	return res, nil
}

// runShot 执行单次运行：独立态向量 + 由 (seed, shot) 派生的随机源。
//
// 随机源采用 PCG(seed, shot)：同一种子下每个 shot 的随机流固定且互不
// 相同，因此整批采样结果完全可复现，且与并发调度无关。
func (s *Simulator) runShot(c *Circuit, maxQ int, seed, shot uint64) ([]Measurement, []int, error) {
	sv, err := newStateVec(c.NQubits, maxQ)
	if err != nil {
		return nil, nil, err
	}
	rng := rand.New(rand.NewPCG(seed, shot))
	reg := make([]int, c.NClbits)
	var events []Measurement

	for _, op := range c.Operations {
		switch op.Type {
		case OpGate:
			if err := applyGateOp(sv, op); err != nil {
				return nil, nil, err
			}
		case OpMeasure:
			v := sv.measure(op.Qubits[0], rng)
			reg[op.Clbit] = v
			events = append(events, Measurement{Qubit: op.Qubits[0], Clbit: op.Clbit, Value: v})
		case OpCondition:
			if reg[op.IfClbit] == 1 {
				if err := applyGateOp(sv, op.Body); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return events, reg, nil
}

// applyGateOp 查表并应用一个门操作。
func applyGateOp(sv *stateVec, op *Operation) error {
	spec, ok := gateRegistry()[op.Name]
	if !ok {
		return kindError(KindUnknownOp, "未知的门操作 %q", op.Name)
	}
	if len(op.Params) != spec.nParams {
		return kindError(KindInvalidArg, "门 %s 需要 %d 个参数，实际给出 %d 个",
			op.Name, spec.nParams, len(op.Params))
	}
	sv.applyGate(spec.matrix(op.Params), op.Qubits)
	return nil
}
