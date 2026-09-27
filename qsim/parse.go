package qsim

import (
	"regexp"
	"strconv"
	"strings"
)

// parser 逐行解析电路文本。
type parser struct {
	lines []string
}

var (
	reQubitRef = regexp.MustCompile(`^q\[(\d+)\]$`)
	reClbitRef = regexp.MustCompile(`^c\[(\d+)\]$`)
	reGateCall = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)(?:\((.*)\))?$`)
)

// parse 执行解析入口。
func (p *parser) parse(src string) (*Circuit, error) {
	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	c := &Circuit{}
	// 同一比特被多次 QUBITS/CBITS 区间声明时用于查重。
	declaredQ := map[int]bool{}
	declaredC := map[int]bool{}

	for idx, raw := range rawLines {
		lineNo := idx + 1
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		tokens := tokenize(line)
		if len(tokens) == 0 {
			continue
		}

		switch strings.ToUpper(tokens[0]) {
		case "QUBITS":
			if len(tokens) != 2 {
				return nil, lineError(KindSyntax, lineNo, "QUBITS 声明格式应为 QUBITS <n>")
			}
			n, err := strconv.Atoi(tokens[1])
			if err != nil || n < 0 {
				return nil, lineError(KindInvalidArg, lineNo, "QUBITS 数量必须是非负整数，得到 %q", tokens[1])
			}
			for i := 0; i < n; i++ {
				if declaredQ[i] {
					return nil, lineError(KindDuplicateDecl, lineNo, "量子比特 q[%d] 被重复声明", i)
				}
				declaredQ[i] = true
			}
			c.NQubits += n
		case "CBITS":
			if len(tokens) != 2 {
				return nil, lineError(KindSyntax, lineNo, "CBITS 声明格式应为 CBITS <n>")
			}
			n, err := strconv.Atoi(tokens[1])
			if err != nil || n < 0 {
				return nil, lineError(KindInvalidArg, lineNo, "CBITS 数量必须是非负整数，得到 %q", tokens[1])
			}
			for i := 0; i < n; i++ {
				if declaredC[i] {
					return nil, lineError(KindDuplicateDecl, lineNo, "经典比特 c[%d] 被重复声明", i)
				}
				declaredC[i] = true
			}
			c.NClbits += n
		case "MEASURE":
			op, err := parseMeasure(tokens, lineNo)
			if err != nil {
				return nil, err
			}
			c.Operations = append(c.Operations, op)
		case "IF":
			op, err := parseCondition(tokens, lineNo)
			if err != nil {
				return nil, err
			}
			c.Operations = append(c.Operations, op)
		default:
			op, err := parseGate(tokens, lineNo)
			if err != nil {
				return nil, err
			}
			c.Operations = append(c.Operations, op)
		}
	}

	if err := validate(c); err != nil {
		return nil, err
	}
	return c, nil
}

// stripComment 去掉 // 行注释。
func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

// tokenize 按空白切分；括号外的逗号也作为分隔符（用于多个量子比特），
// 门参数括号内的逗号保持原样，例如 RX(pi/2, pi) 仍是一个 token。
func tokenize(line string) []string {
	var b strings.Builder
	depth := 0
	for _, ch := range line {
		switch {
		case ch == '(':
			depth++
			b.WriteRune(ch)
		case ch == ')':
			depth--
			b.WriteRune(ch)
		case ch == ',' && depth == 0:
			b.WriteRune(' ')
		default:
			b.WriteRune(ch)
		}
	}
	return strings.Fields(b.String())
}

// parseQubitRef 解析 q[k]。
func parseQubitRef(tok string, lineNo int) (int, error) {
	m := reQubitRef.FindStringSubmatch(tok)
	if m == nil {
		return 0, lineError(KindSyntax, lineNo, "期望量子比特引用 q[k]，得到 %q", tok)
	}
	n, _ := strconv.Atoi(m[1])
	return n, nil
}

// parseClbitRef 解析 c[k]。
func parseClbitRef(tok string, lineNo int) (int, error) {
	m := reClbitRef.FindStringSubmatch(tok)
	if m == nil {
		return 0, lineError(KindSyntax, lineNo, "期望经典比特引用 c[k]，得到 %q", tok)
	}
	n, _ := strconv.Atoi(m[1])
	return n, nil
}

// parseQubitList 解析逗号分隔的量子比特列表，并检查重复。
func parseQubitList(toks []string, lineNo int) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0, len(toks))
	for _, t := range toks {
		q, err := parseQubitRef(t, lineNo)
		if err != nil {
			return nil, err
		}
		if seen[q] {
			return nil, lineError(KindInvalidArg, lineNo, "量子比特 q[%d] 在同一操作中重复出现", q)
		}
		seen[q] = true
		out = append(out, q)
	}
	return out, nil
}

// parseGate 解析门调用，例如 H q[0] / RX(pi/2) q[0] / CX q[0],q[1]。
func parseGate(tokens []string, lineNo int) (*Operation, error) {
	m := reGateCall.FindStringSubmatch(tokens[0])
	if m == nil {
		return nil, lineError(KindSyntax, lineNo, "无法解析的操作 %q", tokens[0])
	}
	name := strings.ToUpper(m[1])
	op := &Operation{Type: OpGate, Name: name, line: lineNo}

	spec, known := gateRegistry()[name]
	if !known {
		return nil, lineError(KindUnknownOp, lineNo, "未知的门操作 %q", name)
	}
	if len(tokens) < 2 {
		return nil, lineError(KindSyntax, lineNo, "门 %s 缺少量子比特操作数", name)
	}
	if m[2] != "" {
		params, err := parseParams(m[2], lineNo)
		if err != nil {
			return nil, err
		}
		op.Params = params
	}
	if len(op.Params) != spec.nParams {
		return nil, lineError(KindInvalidArg, lineNo,
			"门 %s 需要 %d 个参数，实际给出 %d 个", name, spec.nParams, len(op.Params))
	}
	qbits, err := parseQubitList(tokens[1:], lineNo)
	if err != nil {
		return nil, err
	}
	if len(qbits) != qubitArity(name, qbits) {
		return nil, lineError(KindInvalidArg, lineNo,
			"门 %s 作用于 %d 个量子比特，实际给出 %d 个", name, qubitArity(name, qbits), len(qbits))
	}
	op.Qubits = qbits
	return op, nil
}

// qubitArity 返回门的期望比特数；传入 qbits 仅用于无法识别时兜底。
func qubitArity(name string, qbits []int) int {
	switch name {
	case "I", "X", "Y", "Z", "H", "S", "SDG", "T", "TDG", "RX", "RY", "RZ", "P":
		return 1
	case "CX", "CNOT", "CZ", "SWAP":
		return 2
	case "CCX", "TOFFOLI":
		return 3
	}
	return len(qbits)
}

// parseParams 解析门参数列表，支持 pi 常量与四则表达式（无空格）。
func parseParams(raw string, lineNo int) ([]float64, error) {
	parts := strings.Split(raw, ",")
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		v, err := evalAngle(strings.TrimSpace(part))
		if err != nil {
			return nil, lineError(KindInvalidArg, lineNo, "无法解析门参数 %q: %v", part, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseMeasure 解析 MEASURE q[k] -> c[j]。
func parseMeasure(tokens []string, lineNo int) (*Operation, error) {
	if len(tokens) != 4 || strings.TrimSpace(tokens[2]) != "->" {
		return nil, lineError(KindSyntax, lineNo, `测量格式应为 MEASURE q[k] -> c[j]`)
	}
	q, err := parseQubitRef(tokens[1], lineNo)
	if err != nil {
		return nil, err
	}
	c, err := parseClbitRef(tokens[3], lineNo)
	if err != nil {
		return nil, err
	}
	return &Operation{Type: OpMeasure, Name: "MEASURE", Qubits: []int{q}, Clbit: c, line: lineNo}, nil
}

// parseCondition 解析 IF (c[k]) <gate...>。
//
// 词法切分后允许两种常见写法："( c[0] )" 或 "(c[0])"，
// 因此解析时同时接受 "(" / "c[0])" 这样的 token 组合。
func parseCondition(tokens []string, lineNo int) (*Operation, error) {
	if len(tokens) < 3 {
		return nil, lineError(KindSyntax, lineNo, `条件操作格式应为 IF (c[k]) <门> q[...]`)
	}
	// 同时支持 IF (c[0]) X ... 与 IF ( c[0] ) X ... 两种写法。
	var clbitTok string
	var rest []string
	switch {
	case tokens[1] == "(":
		clbitTok = tokens[2]
		closeIdx := 3
		if strings.HasSuffix(clbitTok, ")") {
			clbitTok = strings.TrimSuffix(clbitTok, ")")
		} else {
			if len(tokens) <= 3 || tokens[3] != ")" {
				return nil, lineError(KindSyntax, lineNo, `条件判断缺少右括号 ")"`)
			}
			closeIdx = 4
		}
		rest = tokens[closeIdx:]
	case strings.HasPrefix(tokens[1], "("):
		inner := strings.TrimPrefix(tokens[1], "(")
		if !strings.HasSuffix(inner, ")") {
			return nil, lineError(KindSyntax, lineNo, `条件判断缺少右括号 ")"`)
		}
		clbitTok = strings.TrimSuffix(inner, ")")
		rest = tokens[2:]
	default:
		return nil, lineError(KindSyntax, lineNo, `条件判断必须以 (c[k]) 形式给出，得到 %q`, tokens[1])
	}
	cl, err := parseClbitRef(clbitTok, lineNo)
	if err != nil {
		return nil, err
	}
	if len(rest) == 0 {
		return nil, lineError(KindSyntax, lineNo, "条件成立时缺少要执行的门操作")
	}
	head := strings.ToUpper(rest[0])
	if head == "MEASURE" || head == "IF" {
		return nil, lineError(KindInvalidArg, lineNo, "条件体目前只支持门操作，得到 %s", head)
	}
	body, err := parseGate(rest, lineNo)
	if err != nil {
		return nil, err
	}
	return &Operation{Type: OpCondition, IfClbit: cl, Body: body, line: lineNo}, nil
}

// validate 做跨操作的静态校验：所有引用必须在声明范围内。
func validate(c *Circuit) error {
	check := func(op *Operation) error {
		for _, q := range op.Qubits {
			if q < 0 || q >= c.NQubits {
				return lineError(KindOutOfRange, op.line,
					"量子比特 q[%d] 越界：电路仅声明了 %d 个量子比特", q, c.NQubits)
			}
		}
		if op.Type == OpMeasure {
			if op.Clbit < 0 || op.Clbit >= c.NClbits {
				return lineError(KindOutOfRange, op.line,
					"经典比特 c[%d] 越界：电路仅声明了 %d 个经典比特", op.Clbit, c.NClbits)
			}
		}
		return nil
	}
	for _, op := range c.Operations {
		switch op.Type {
		case OpGate, OpMeasure:
			if err := check(op); err != nil {
				return err
			}
		case OpCondition:
			if op.IfClbit < 0 || op.IfClbit >= c.NClbits {
				return lineError(KindOutOfRange, op.line,
					"条件经典比特 c[%d] 越界：电路仅声明了 %d 个经典比特", op.IfClbit, c.NClbits)
			}
			if op.Body == nil {
				return lineError(KindSyntax, op.line, "条件操作缺少函数体")
			}
			if err := check(op.Body); err != nil {
				return err
			}
		}
	}
	return nil
}
