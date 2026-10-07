package circuit

// Validate 检查线路结构合法性。
//
// 目前校验的规则：任何门的操作数不得重复指向同一个量子比特
// （如 CNOT 0 0）。解析期已逐条检查，这里复核全部操作，
// 便于校验绕过 Parse 手工构造的 Circuit。
func Validate(c *Circuit) error {
	for _, op := range c.Ops {
		if op.Kind != OpGate && op.Kind != OpConditional {
			continue
		}
		if err := checkDuplicateOperands(op.Gate); err != nil {
			return err
		}
	}
	return nil
}

// checkDuplicateOperand 检查单个门的操作数是否重复。
func checkDuplicateOperands(g Gate) error {
	seen := make(map[int]bool, len(g.Qubits))
	for _, q := range g.Qubits {
		if seen[q] {
			return ErrDuplicateOperand{Gate: g.Name, Qubit: q}
		}
		seen[q] = true
	}
	return nil
}
