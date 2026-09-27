package qsim

import "fmt"

// ErrorKind 以可程序化判断的方式区分电路解析/校验/运行失败的原因。
type ErrorKind int

const (
	// KindUnknown 未分类错误（正常流程不应出现）。
	KindUnknown ErrorKind = iota
	// KindSyntax 电路结构不完整或词法/语法非法。
	KindSyntax
	// KindUnknownOp 出现了模拟器不支持的操作。
	KindUnknownOp
	// KindDuplicateDecl 量子/经典比特重复声明。
	KindDuplicateDecl
	// KindMissingDecl 使用了从未声明的比特。
	KindMissingDecl
	// KindOutOfRange 比特下标越界或参数非法。
	KindOutOfRange
	// KindInvalidArg 操作参数个数/取值不合法。
	KindInvalidArg
	// KindResourceExhausted 态向量规模超过允许的资源上限。
	KindResourceExhausted
)

// String 返回错误类别的稳定标识，便于日志解释与断言。
func (k ErrorKind) String() string {
	switch k {
	case KindSyntax:
		return "SYNTAX"
	case KindUnknownOp:
		return "UNKNOWN_OP"
	case KindDuplicateDecl:
		return "DUPLICATE_DECL"
	case KindMissingDecl:
		return "MISSING_DECL"
	case KindOutOfRange:
		return "OUT_OF_RANGE"
	case KindInvalidArg:
		return "INVALID_ARG"
	case KindResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	default:
		return "UNKNOWN"
	}
}

// CircuitError 携带失败类别、行号与人类可读信息，便于调用方区分失败原因。
type CircuitError struct {
	Kind    ErrorKind
	Line    int // 1 基行号；非解析阶段错误为 0
	Message string
}

func (e *CircuitError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s (line %d): %s", e.Kind, e.Line, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// kindError 快速构造一个带类别的错误（无行号）。
func kindError(kind ErrorKind, format string, args ...any) error {
	return &CircuitError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// lineError 快速构造一个带类别与行号的错误。
func lineError(kind ErrorKind, line int, format string, args ...any) error {
	return &CircuitError{Kind: kind, Line: line, Message: fmt.Sprintf(format, args...)}
}

// AsCircuitError 尝试把 error 还原为 *CircuitError。
func AsCircuitError(err error) (*CircuitError, bool) {
	if ce, ok := err.(*CircuitError); ok {
		return ce, true
	}
	return nil, false
}
