package qsim

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// evalAngle 对门参数做受限表达式求值：
// 支持十进制数字、pi（含 pi/2、2*pi、-pi/3 等）、一元负号、
// 以及 + - * / 与括号；不允许任何空白之外的标识符。
func evalAngle(expr string) (float64, error) {
	t := &angleTok{src: expr}
	v, err := t.parseExpr()
	if err != nil {
		return 0, err
	}
	t.skipSpaces()
	if !t.eof() {
		return 0, fmt.Errorf("表达式尾部存在无法识别的字符 %q", t.peek())
	}
	return v, nil
}

type angleTok struct {
	src string
	pos int
}

func (t *angleTok) skipSpaces() {
	for t.pos < len(t.src) && (t.src[t.pos] == ' ' || t.src[t.pos] == '\t') {
		t.pos++
	}
}

func (t *angleTok) eof() bool  { return t.pos >= len(t.src) }
func (t *angleTok) peek() byte { return t.src[t.pos] }
func (t *angleTok) eat() byte  { ch := t.src[t.pos]; t.pos++; return ch }

func (t *angleTok) parseExpr() (float64, error) {
	v, err := t.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		t.skipSpaces()
		if t.eof() || (t.peek() != '+' && t.peek() != '-') {
			return v, nil
		}
		op := t.eat()
		rhs, err := t.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += rhs
		} else {
			v -= rhs
		}
	}
}

func (t *angleTok) parseTerm() (float64, error) {
	v, err := t.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		t.skipSpaces()
		if t.eof() || (t.peek() != '*' && t.peek() != '/') {
			return v, nil
		}
		op := t.eat()
		rhs, err := t.parseUnary()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, fmt.Errorf("角度表达式中出现除以零")
			}
			v /= rhs
		}
	}
}

func (t *angleTok) parseUnary() (float64, error) {
	t.skipSpaces()
	if !t.eof() && t.peek() == '-' {
		t.eat()
		v, err := t.parseUnary()
		return -v, err
	}
	if !t.eof() && t.peek() == '+' {
		t.eat()
		return t.parseUnary()
	}
	return t.parsePrimary()
}

func (t *angleTok) parsePrimary() (float64, error) {
	t.skipSpaces()
	if t.eof() {
		return 0, fmt.Errorf("表达式不完整")
	}
	if t.peek() == '(' {
		t.eat()
		v, err := t.parseExpr()
		if err != nil {
			return 0, err
		}
		t.skipSpaces()
		if t.eof() || t.peek() != ')' {
			return 0, fmt.Errorf("缺少右括号")
		}
		t.eat()
		return v, nil
	}
	if strings.HasPrefix(t.src[t.pos:], "pi") {
		t.pos += 2
		return math.Pi, nil
	}
	start := t.pos
	for t.pos < len(t.src) {
		ch := t.src[t.pos]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			t.pos++
			continue
		}
		break
	}
	if start == t.pos {
		return 0, fmt.Errorf("无法识别的符号 %q", t.peek())
	}
	v, err := strconv.ParseFloat(t.src[start:t.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("数字格式非法 %q", t.src[start:t.pos])
	}
	return v, nil
}
