package qsim

import (
	"math"
	"math/cmplx"
)

// gateSpec 描述一个已注册的原生门。
//
// 矩阵采用 2^k × 2^k 行主序稠密复矩阵，作用在复合希尔伯特空间上：
// qbits[0] 对应矩阵下标中的高位比特，qbits[k-1] 对应低位比特。
type gateSpec struct {
	nParams int
	matrix  func(p []float64) []complex128
}

var (
	cI = complex(1.0, 0)
	c0 = complex(0.0, 0)
	ci = complex(0.0, 1)
)

func sqrt2inv() complex128 { return complex(1/math.Sqrt2, 0) }

// fixedMatrix 把固定矩阵包装成不依赖参数的构造器。
func fixedMatrix(m []complex128) func(p []float64) []complex128 {
	return func(_ []float64) []complex128 { return m }
}

var registeredGates = buildGateRegistry()

func buildGateRegistry() map[string]gateSpec {
	minusI := complex(0, -1)
	reg := make(map[string]gateSpec)

	put := func(names []string, spec gateSpec) {
		for _, n := range names {
			reg[n] = spec
		}
	}

	// ---- 单比特无参门 ----
	put([]string{"I"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, cI,
	})})
	put([]string{"X"}, gateSpec{0, fixedMatrix([]complex128{
		c0, cI,
		cI, c0,
	})})
	put([]string{"Y"}, gateSpec{0, fixedMatrix([]complex128{
		c0, minusI,
		ci, c0,
	})})
	put([]string{"Z"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, -cI,
	})})
	put([]string{"H"}, gateSpec{0, fixedMatrix([]complex128{
		sqrt2inv(), sqrt2inv(),
		sqrt2inv(), -sqrt2inv(),
	})})
	put([]string{"S"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, ci,
	})})
	put([]string{"SDG"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, -ci,
	})})
	tPhase := cmplx.Exp(complex(0, math.Pi/4))
	put([]string{"T"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, tPhase,
	})})
	put([]string{"TDG"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0,
		c0, cmplx.Conj(tPhase),
	})})

	// ---- 单比特旋转门（各 1 个角度参数，单位：弧度）----
	put([]string{"RX"}, gateSpec{1, func(p []float64) []complex128 {
		c := complex(math.Cos(p[0]/2), 0)
		s := complex(math.Sin(p[0]/2), 0)
		return []complex128{
			c, -ci * s,
			-ci * s, c,
		}
	}})
	put([]string{"RY"}, gateSpec{1, func(p []float64) []complex128 {
		c := complex(math.Cos(p[0]/2), 0)
		s := complex(math.Sin(p[0]/2), 0)
		return []complex128{
			c, -s,
			s, c,
		}
	}})
	put([]string{"RZ"}, gateSpec{1, func(p []float64) []complex128 {
		e0 := cmplx.Exp(complex(0, -p[0]/2))
		e1 := cmplx.Exp(complex(0, p[0]/2))
		return []complex128{
			e0, c0,
			c0, e1,
		}
	}})
	put([]string{"P"}, gateSpec{1, func(p []float64) []complex128 {
		return []complex128{
			cI, c0,
			c0, cmplx.Exp(complex(0, p[0])),
		}
	}})

	// ---- 两比特门：qbits = [control..., target/低位] ----
	put([]string{"CX", "CNOT"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0, c0, c0,
		c0, cI, c0, c0,
		c0, c0, c0, cI,
		c0, c0, cI, c0,
	})})
	put([]string{"CZ"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0, c0, c0,
		c0, cI, c0, c0,
		c0, c0, cI, c0,
		c0, c0, c0, -cI,
	})})
	put([]string{"SWAP"}, gateSpec{0, fixedMatrix([]complex128{
		cI, c0, c0, c0,
		c0, c0, cI, c0,
		c0, cI, c0, c0,
		c0, c0, c0, cI,
	})})

	// ---- 三比特门 CCX：qbits = [c1, c2, target] ----
	ccx := make([]complex128, 64)
	for i := 0; i < 8; i++ {
		j := i
		if i == 6 {
			j = 7
		} else if i == 7 {
			j = 6
		}
		ccx[i*8+j] = cI
	}
	put([]string{"CCX", "TOFFOLI"}, gateSpec{0, fixedMatrix(ccx)})

	return reg
}

// gateRegistry 返回门名（大写）-> 门规格 的只读查找表。
func gateRegistry() map[string]gateSpec {
	return registeredGates
}
