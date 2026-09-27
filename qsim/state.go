package qsim

import (
	"fmt"
	"math"
)

// stateVec 是一个纯态的态向量，幅度按计算基序排列：
// 下标 i 的第 q 位对应量子比特 q（q=0 为最低位）。
type stateVec struct {
	n   int
	amp []complex128
}

// newStateVec 构造 |0...0> 初态；规模超限时返回 KindResourceExhausted。
//
// n 个量子比特需要 2^n 个 complex128（每个 16 字节），
// 同时以 MaxQubits 与单次分配字节数两道防线限制内存。
func newStateVec(n int, maxQubits int) (*stateVec, error) {
	if n < 0 {
		return nil, kindError(KindInvalidArg, "量子比特数不能为负：%d", n)
	}
	if n > maxQubits {
		return nil, kindError(KindResourceExhausted,
			"需要 %d 个量子比特（态向量 %d 个幅度，约 %s），超过允许上限 %d",
			n, uint64(1)<<uint(n), humanBytes(stateBytes(n)), maxQubits)
	}
	size := 1 << uint(n)
	// 兜底：即便 maxQubits 被调用方调得过大，也拒绝明显不合理的单次分配。
	const hardMaxAmp = 1 << 30 // 约 16GiB（10.7 亿个 complex128）
	if uint64(size) > hardMaxAmp {
		return nil, kindError(KindResourceExhausted,
			"态向量分配规模 %d 超过硬性上限 %d", size, hardMaxAmp)
	}
	amp := make([]complex128, size)
	amp[0] = 1
	return &stateVec{n: n, amp: amp}, nil
}

// stateBytes 返回 n 量子比特态向量占用的字节数（每幅度 16B）。
func stateBytes(n int) uint64 {
	return uint64(1) << uint(n) * 16
}

// humanBytes 返回便于阅读的字节量级。
func humanBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%d GiB", b>>30)
	case b >= 1<<20:
		return fmt.Sprintf("%d MiB", b>>20)
	case b >= 1<<10:
		return fmt.Sprintf("%d KiB", b>>10)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// applyGate 按给定门矩阵作用到若干量子比特上。
//
// matrix 为 2^k × 2^k 的稠密复矩阵（行主序）；qbits[0] 对应复合下标
// 高位，qbits[k-1] 对应低位。通过枚举“自由比特”分组，原地更新幅度。
func (s *stateVec) applyGate(matrix []complex128, qbits []int) {
	k := len(qbits)
	dim := 1 << uint(k)

	// 目标比特掩码，以及“压缩位 -> 原位”映射。
	mask := 0
	for _, q := range qbits {
		mask |= 1 << uint(q)
	}
	freeMask := (1 << uint(s.n)) - 1 - mask

	// 枚举自由比特组合；free 本身就是按原比特位置散开的下标，
	// 故可以直接作为该分组的基址。
	for free := freeMask; ; free = (free - 1) & freeMask {
		// 收集该分组的 2^k 个幅度：in[a] 对应复合下标 a。
		in := make([]complex128, dim)
		for a := 0; a < dim; a++ {
			in[a] = s.amp[free|expand(a, qbits)]
		}
		for row := 0; row < dim; row++ {
			var sum complex128
			mrow := matrix[row*dim : (row+1)*dim]
			for col := 0; col < dim; col++ {
				sum += mrow[col] * in[col]
			}
			s.amp[free|expand(row, qbits)] = sum
		}

		if free == 0 {
			break
		}
	}
}

// expand 把 k 位复合下标 a 按 qbits 映射回完整 n 位下标：
// a 的第 (k-1-i) 位放到 qbits[i] 上（qbits[0] 为高位）。
func expand(a int, qbits []int) int {
	out := 0
	k := len(qbits)
	for i, q := range qbits {
		bit := (a >> uint(k-1-i)) & 1
		out |= bit << uint(q)
	}
	return out
}

// measure 对量子比特 q 执行一次投影测量。
//
// 以该比特为 1 的全部基矢概率之和作为 P(1)，由 rng 抽取 [0,1)
// 均匀随机数决定结果；随后把未观测到的分支幅度清零并整体归一化，
// 从而保证后续门与测量只在坍缩后的态上演化。
func (s *stateVec) measure(q int, rng rngSource) int {
	bit := uint(q)
	var p1 float64
	for i, a := range s.amp {
		if (i>>bit)&1 == 1 {
			p1 += real(a)*real(a) + imag(a)*imag(a)
		}
	}
	result := 0
	if rng.Float64() < p1 {
		result = 1
	}
	normSq := 0.0
	for i := range s.amp {
		isOne := ((i >> bit) & 1) == 1
		if isOne != (result == 1) {
			s.amp[i] = 0
		} else {
			a := s.amp[i]
			normSq += real(a)*real(a) + imag(a)*imag(a)
		}
	}
	if normSq > 0 {
		scale := complex(1/math.Sqrt(normSq), 0)
		for i := range s.amp {
			s.amp[i] *= scale
		}
	}
	return result
}

// rngSource 抽象随机源，便于注入确定性实现。
type rngSource interface {
	Float64() float64
}
