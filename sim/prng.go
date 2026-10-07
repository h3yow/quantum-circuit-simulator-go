package sim

import "math/rand"

// snapSource 是一个可快照的确定性伪随机源（SplitMix64）。
//
// 标准库的 rand.Source 无法导出内部状态，而检查点需要保存/恢复
// 随机序列的精确位置，因此 Run/Sample 与执行会话统一使用该源，
// 保证同一线路、同一种子下所有用法共享同一条随机序列。
type snapSource struct {
	state uint64 // 全部状态，可直接快照
}

var _ rand.Source64 = (*snapSource)(nil)

func newSnapSource(seed int64) *snapSource { return &snapSource{state: uint64(seed)} }

// Seed 实现 rand.Source。
func (s *snapSource) Seed(seed int64) { s.state = uint64(seed) }

// Uint64 实现 rand.Source64（SplitMix64 递推）。
func (s *snapSource) Uint64() uint64 {
	s.state += 0x9E3779B97F4A7C15
	z := s.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Int63 实现 rand.Source。
func (s *snapSource) Int63() int64 { return int64(s.Uint64() & 0x7FFFFFFFFFFFFFFF) }

// snapshot 返回当前状态，可用于之后精确恢复到本条随机序列位置。
func (s *snapSource) snapshot() uint64 { return s.state }

// restore 将源恢复到 snapshot 保存的状态。
func (s *snapSource) restore(state uint64) { s.state = state }

// newRNG 以给定种子创建一次运行/会话使用的随机源。
func newRNG(seed int64) (*rand.Rand, *snapSource) {
	src := newSnapSource(seed)
	return rand.New(src), src
}
