package safecast

import (
	"math"
)

// This files is highly inspired from https://pkg.go.dev/golang.org/x/exp/constraints
// I didn't import it as it would have added an unnecessary dependency

// Signed is a constraint for all signed integers: int, int8, int16, int32, and int64 types.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned is a constraint for all unsigned integers: uint, uint8, uint16, uint32, and uint64 types.
// TODO: support uintpr
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Integer is a constraint for the all unsigned and signed integers
type Integer interface {
	Signed | Unsigned
}

// Float is a constraint for the float32 and float64 types.
type Float interface {
	~float32 | ~float64
}

// Number is a constraint for all integers and floats
// TODO: consider using complex, but not sure there is a need
type Number interface {
	Integer | Float
}

// Input is a constraint for all types that can be used as input for [Convert], and [MustConvert]
type Input interface {
	Number | ~string | ~bool
}

func isNegative[T Number](t T) bool {
	return t < 0
}

func sameSign[T1, T2 Number](a T1, b T2) bool {
	return isNegative(a) == isNegative(b)
}

func isUnsigned[T Number]() bool {
	v := -1
	return T(v) >= 0
}
func isFloat[T Number]() bool {
	v := 0.1
	return T(v) != 0
}

func isFloat32[T Number]() bool {
	if !isFloat[T]() {
		return false
	}
	v := math.SmallestNonzeroFloat64
	return T(v*0.9) == 0
}

func isFloat64[T Number]() bool {
	return isFloat[T]() && !isFloat32[T]()
}

func sizeOf[T Number]() uint64 {
	if isFloat32[T]() {
		return 4
	}

	x := uint16(1 << 8)
	y := uint32(2 << 16)
	z := uint64(4 << 32)

	return 1 + uint64(T(x))>>8 + uint64(T(y))>>16 + uint64(T(z))>>32
}

func minOf[T Number]() any {
	switch {
	case isFloat64[T]():
		return float64(-math.MaxFloat64)
	case isFloat32[T]():
		return float32(-math.MaxFloat32)
	case isUnsigned[T]():
		return T(0)
	}
	v := int64(1) << (8*sizeOf[T]() - 1)
	return T(v)
}

func maxOf[T Number]() any {
	switch {
	case isFloat64[T]():
		return float64(math.MaxFloat64)
	// there is no case for float64, as nothing can overflow it
	case isFloat32[T]():
		return float32(math.MaxFloat32)
	}
	v := uint64(1)<<(8*sizeOf[T]()-1) - 1
	if isUnsigned[T]() {
		v = v*2 + 1
	}

	return T(v)
}
