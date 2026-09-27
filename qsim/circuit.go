package qsim

// Circuit 是一份经过解析与校验的电路描述。
type Circuit struct {
	NQubits    int
	NClbits    int
	Operations []*Operation
}

// OpType 标识一条电路操作的种类。
type OpType int

const (
	OpGate OpType = iota
	OpMeasure
	OpCondition
)

// String 返回操作种类的稳定名称。
func (t OpType) String() string {
	switch t {
	case OpGate:
		return "GATE"
	case OpMeasure:
		return "MEASURE"
	case OpCondition:
		return "CONDITION"
	default:
		return "UNKNOWN"
	}
}

// Operation 描述一条按电路顺序执行的操作。
//
// 对于条件操作，IfClbit 给出被判断的经典比特，期望值固定为 1，
// Body 为条件成立时执行的子操作（门）。
type Operation struct {
	Type    OpType
	Name    string    // 门名（大写），测量为 "MEASURE"
	Qubits  []int     // 门作用的量子比特 / 测量的量子比特
	Params  []float64 // 旋转门角度参数（弧度）
	Clbit   int       // 测量写入的经典比特
	IfClbit int       // 条件操作读取的经典比特
	Body    *Operation
	line    int // 源文件行号，用于报错
}

// Line 返回该操作在源文本中的 1 基行号。
func (o *Operation) Line() int { return o.line }

// Parse 解析电路描述文本并完成全部静态校验。
//
// 失败时返回 *CircuitError，可通过 ErrorKind 区分：
// 语法不完整、未知操作、重复声明、缺失声明、越界、参数非法等。
func Parse(src string) (*Circuit, error) {
	p := &parser{}
	return p.parse(src)
}
