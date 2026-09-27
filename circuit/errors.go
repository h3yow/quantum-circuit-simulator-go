package circuit

import "fmt"

// ErrUnknownOp 表示线路中出现了无法识别的操作。
type ErrUnknownOp struct{ Name string }

func (e ErrUnknownOp) Error() string { return fmt.Sprintf("unknown operation %q", e.Name) }

// ErrOutOfRange 表示引用了越界的量子比特或经典比特。
type ErrOutOfRange struct {
	What  string // "qubit" 或 "bit"
	Index int
	Limit int
}

func (e ErrOutOfRange) Error() string {
	return fmt.Sprintf("%s index %d out of range [0,%d)", e.What, e.Index, e.Limit)
}

// ErrDuplicateDecl 表示量子比特或经典比特被重复声明。
type ErrDuplicateDecl struct{ What string }

func (e ErrDuplicateDecl) Error() string { return fmt.Sprintf("duplicate declaration: %s", e.What) }

// ErrIncomplete 表示线路结构不完整（缺少声明、字段缺失等）。
type ErrIncomplete struct{ What string }

func (e ErrIncomplete) Error() string { return fmt.Sprintf("incomplete circuit: %s", e.What) }
