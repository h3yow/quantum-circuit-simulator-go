package circuit

import (
	"fmt"
	"strconv"
	"strings"
)

// 支持的门及其作用的量子比特数与是否带参数。
type gateSpec struct {
	arity    int
	hasParam bool
}

var gateTable = map[string]gateSpec{
	"H":    {1, false},
	"X":    {1, false},
	"Y":    {1, false},
	"Z":    {1, false},
	"S":    {1, false},
	"T":    {1, false},
	"RX":   {1, true},
	"RY":   {1, true},
	"RZ":   {1, true},
	"CNOT": {2, false},
	"CZ":   {2, false},
	"SWAP": {2, false},
}

// Parse 从文本描述解析量子线路。
//
// 语法（每行一条，# 之后为注释，空行忽略）：
//
//	qubits N          声明量子比特数（必须且只能出现一次）
//	bits M            声明经典比特数（必须且只能出现一次）
//	H 0               单比特门
//	CNOT 0 1          双比特门
//	RX 0 1.5708       带参数门
//	MEASURE 0 -> 0    测量量子比特 0 写入经典比特 0
//	IF 0==1 X 1       经典比特 0 为 1 时执行门 X 1
func Parse(input string) (*Circuit, error) {
	c := &Circuit{}
	declaredQ, declaredB := false, false

	lines := strings.Split(input, "\n")
	for ln, raw := range lines {
		line := strings.TrimSpace(raw)
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		head := strings.ToUpper(fields[0])

		switch head {
		case "QUBITS":
			if declaredQ {
				return nil, ErrDuplicateDecl{What: "qubits"}
			}
			n, err := parseCount(fields, "qubits")
			if err != nil {
				return nil, err
			}
			c.NumQubits, declaredQ = n, true
		case "BITS":
			if declaredB {
				return nil, ErrDuplicateDecl{What: "bits"}
			}
			n, err := parseCount(fields, "bits")
			if err != nil {
				return nil, err
			}
			c.NumBits, declaredB = n, true
		case "MEASURE":
			if !declaredQ || !declaredB {
				return nil, ErrIncomplete{What: fmt.Sprintf("line %d: MEASURE before declarations", ln+1)}
			}
			op, err := parseMeasure(fields, c)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", ln+1, err)
			}
			c.Ops = append(c.Ops, op)
		case "IF":
			if !declaredQ || !declaredB {
				return nil, ErrIncomplete{What: fmt.Sprintf("line %d: IF before declarations", ln+1)}
			}
			op, err := parseConditional(fields, c)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", ln+1, err)
			}
			c.Ops = append(c.Ops, op)
		default:
			if !declaredQ || !declaredB {
				return nil, ErrIncomplete{What: fmt.Sprintf("line %d: gate before declarations", ln+1)}
			}
			op, err := parseGate(fields, c)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", ln+1, err)
			}
			c.Ops = append(c.Ops, op)
		}
	}

	if !declaredQ {
		return nil, ErrIncomplete{What: "missing qubits declaration"}
	}
	if !declaredB {
		return nil, ErrIncomplete{What: "missing bits declaration"}
	}
	return c, nil
}

func parseCount(fields []string, what string) (int, error) {
	if len(fields) != 2 {
		return 0, ErrIncomplete{What: what + " declaration needs exactly one number"}
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n <= 0 {
		return 0, ErrIncomplete{What: fmt.Sprintf("invalid %s count %q", what, fields[1])}
	}
	return n, nil
}

func parseIndex(s string, what string, limit int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, ErrIncomplete{What: fmt.Sprintf("invalid %s index %q", what, s)}
	}
	if n < 0 || n >= limit {
		return 0, ErrOutOfRange{What: what, Index: n, Limit: limit}
	}
	return n, nil
}

// parseGate 解析一条门操作，fields[0] 为门名。
func parseGate(fields []string, c *Circuit) (Operation, error) {
	name := strings.ToUpper(fields[0])
	spec, ok := gateTable[name]
	if !ok {
		return Operation{}, ErrUnknownOp{Name: name}
	}
	want := 1 + spec.arity
	if spec.hasParam {
		want++
	}
	if len(fields) != want {
		return Operation{}, ErrIncomplete{What: fmt.Sprintf("gate %s expects %d operands, got %d", name, want-1, len(fields)-1)}
	}
	g := Gate{Name: name, Qubits: make([]int, spec.arity)}
	for i := 0; i < spec.arity; i++ {
		q, err := parseIndex(fields[1+i], "qubit", c.NumQubits)
		if err != nil {
			return Operation{}, err
		}
		for j := 0; j < i; j++ {
			if g.Qubits[j] == q {
				return Operation{}, ErrDuplicateQubit{Gate: name, Qubit: q}
			}
		}
		g.Qubits[i] = q
	}
	if spec.hasParam {
		p, err := strconv.ParseFloat(fields[1+spec.arity], 64)
		if err != nil {
			return Operation{}, ErrIncomplete{What: fmt.Sprintf("gate %s invalid param %q", name, fields[1+spec.arity])}
		}
		g.Param = p
	}
	return Operation{Kind: OpGate, Gate: g}, nil
}

// parseMeasure 解析 MEASURE q -> c。
func parseMeasure(fields []string, c *Circuit) (Operation, error) {
	if len(fields) != 4 || fields[2] != "->" {
		return Operation{}, ErrIncomplete{What: "MEASURE syntax: MEASURE q -> c"}
	}
	q, err := parseIndex(fields[1], "qubit", c.NumQubits)
	if err != nil {
		return Operation{}, err
	}
	b, err := parseIndex(fields[3], "bit", c.NumBits)
	if err != nil {
		return Operation{}, err
	}
	return Operation{Kind: OpMeasure, Qubit: q, Bit: b}, nil
}

// parseConditional 解析 IF c==v GATE ...。
func parseConditional(fields []string, c *Circuit) (Operation, error) {
	if len(fields) < 4 {
		return Operation{}, ErrIncomplete{What: "IF syntax: IF c==v GATE ..."}
	}
	cond := fields[1]
	var condBit, condVal int
	if _, err := fmt.Sscanf(cond, "%d==%d", &condBit, &condVal); err != nil {
		return Operation{}, ErrIncomplete{What: fmt.Sprintf("invalid condition %q (want c==v)", cond)}
	}
	if condBit < 0 || condBit >= c.NumBits {
		return Operation{}, ErrOutOfRange{What: "bit", Index: condBit, Limit: c.NumBits}
	}
	if condVal != 0 && condVal != 1 {
		return Operation{}, ErrIncomplete{What: fmt.Sprintf("condition value must be 0 or 1, got %d", condVal)}
	}
	gateOp, err := parseGate(fields[2:], c)
	if err != nil {
		return Operation{}, err
	}
	return Operation{
		Kind:      OpConditional,
		Gate:      gateOp.Gate,
		CondBit:   condBit,
		CondValue: condVal,
	}, nil
}
