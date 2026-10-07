package sim

// 资源上限规则（对 Run/Sample/会话/检查点恢复统一生效）：
//   - 调用方未显式给出上限（<=0）时使用 DefaultMaxQubits；
//   - 调用方上限超过 HardMaxQubits 时被收敛到硬上限，而不是报错；
//   - 任何调用都无法突破 HardMaxQubits（全局硬上限）。
//
// 归一化只影响上限取值，不会因此拒绝本可运行的线路。
const (
	DefaultMaxQubits = 20 // 缺省保护上限
	HardMaxQubits    = 26 // 全局硬上限（2^26 个 complex128 约 1GiB）
)

// normalizeLimit 将调用方给定的上限归一化为实际生效值。
func normalizeLimit(maxQubits int) int {
	if maxQubits <= 0 {
		return DefaultMaxQubits
	}
	if maxQubits > HardMaxQubits {
		return HardMaxQubits
	}
	return maxQubits
}
