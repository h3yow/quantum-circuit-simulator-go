package qsim_test

import (
	"fmt"

	qsim "github.com/h3yow/quantum-circuit-simulator-go/qsim"
)

// Example 展示：解析电路 -> 同种子两次运行结果一致 -> 错误可按类别区分。
func Example() {
	src := `QUBITS 2
CBITS 2
H q[0]
CX q[0], q[1]
MEASURE q[0] -> c[0]
MEASURE q[1] -> c[1]
`
	sim := qsim.New(0)
	r1, err := sim.Run(src, qsim.Options{Shots: 10, Seed: 7})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	r2, _ := sim.Run(src, qsim.Options{Shots: 10, Seed: 7})
	fmt.Printf("reproducible=%v\n", sameCounts(r1.Counts, r2.Counts))

	// 非法电路返回带类别的错误。
	_, err = sim.Run("QUBITS 1\nNOPE q[0]\n", qsim.Options{Shots: 1})
	if ce, ok := qsim.AsCircuitError(err); ok {
		fmt.Println("rejected:", ce.Kind)
	}
	// Output:
	// reproducible=true
	// rejected: UNKNOWN_OP
}

func sameCounts(a, b map[int]int) bool {
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
