// Package circuit 定义量子线路的描述结构与解析结果类型。
package circuit

// OpKind 表示线路中一条操作的类别。
type OpKind int

const (
	OpGate      OpKind = iota // 幺正门操作
	OpMeasure                 // 测量操作
	OpConditional             // 依赖经典比特的条件操作
)

// Gate 描述一个作用在若干量子比特上的门。
type Gate struct {
	Name   string  // 门名称，如 H、X、CNOT
	Qubits []int   // 作用的量子比特下标
	Param  float64 // 可选参数（如旋转门角度）
}

// Operation 是线路中的一步操作。
type Operation struct {
	Kind      OpKind
	Gate      Gate   // Kind == OpGate 或 OpConditional 时有效
	Qubit     int    // Kind == OpMeasure 时被测量的量子比特
	Bit       int    // Kind == OpMeasure 时写入的经典比特
	CondBit   int    // Kind == OpConditional 时依赖的经典比特
	CondValue int    // 条件成立所需的经典比特取值（0 或 1）
}

// Circuit 是一份完整的量子线路描述。
type Circuit struct {
	NumQubits int
	NumBits   int
	Ops       []Operation
}
