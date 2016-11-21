package vrp

import (
	"fmt"
	"go/token"
	"go/types"
	"math/big"

	"honnef.co/go/ssa"
)

type Zs []Z

func (zs Zs) Len() int {
	return len(zs)
}

func (zs Zs) Less(i int, j int) bool {
	return zs[i].Cmp(zs[j]) == -1
}

func (zs Zs) Swap(i int, j int) {
	zs[i], zs[j] = zs[j], zs[i]
}

type Z struct {
	infinity int8
	integer  *big.Int
}

func NewZ(n *big.Int) Z {
	return Z{integer: n}
}

func (z1 Z) Infinite() bool {
	return z1.infinity != 0
}

func (z1 Z) Add(z2 Z) Z {
	if z2.Sign() == -1 {
		return z1.Sub(z2.Negate())
	}
	if z1 == NInfinity {
		return NInfinity
	}
	if z1 == PInfinity {
		return PInfinity
	}
	if z2 == PInfinity {
		return PInfinity
	}

	if !z1.Infinite() && !z2.Infinite() {
		n := &big.Int{}
		n.Add(z1.integer, z2.integer)
		return NewZ(n)
	}

	panic(fmt.Sprintf("%s + %s is not defined", z1, z2))
}

func (z1 Z) Sub(z2 Z) Z {
	if z2.Sign() == -1 {
		return z1.Add(z2.Negate())
	}
	if !z1.Infinite() && !z2.Infinite() {
		n := &big.Int{}
		n.Sub(z1.integer, z2.integer)
		return NewZ(n)
	}

	if z1 != PInfinity && z2 == PInfinity {
		return NInfinity
	}
	if z1.Infinite() && !z2.Infinite() {
		return Z{infinity: z1.infinity}
	}
	panic(fmt.Sprintf("%s - %s is not defined", z1, z2))
}

func (z1 Z) Mul(z2 Z) Z {
	if (z1.integer != nil && z1.integer.Sign() == 0) ||
		(z2.integer != nil && z2.integer.Sign() == 0) {
		return NewZ(&big.Int{})
	}

	if z1.infinity != 0 || z2.infinity != 0 {
		return Z{infinity: int8(z1.Sign() * z2.Sign())}
	}

	n := &big.Int{}
	n.Mul(z1.integer, z2.integer)
	return NewZ(n)
}

func (z1 Z) Negate() Z {
	if z1.infinity == 1 {
		return NInfinity
	}
	if z1.infinity == -1 {
		return PInfinity
	}
	n := &big.Int{}
	n.Neg(z1.integer)
	return NewZ(n)
}

func (z1 Z) Sign() int {
	if z1.infinity != 0 {
		return int(z1.infinity)
	}
	return z1.integer.Sign()
}

func (z1 Z) String() string {
	if z1 == NInfinity {
		return "-∞"
	}
	if z1 == PInfinity {
		return "∞"
	}
	return fmt.Sprintf("%d", z1.integer)
}

func (z1 Z) Cmp(z2 Z) int {
	if z1.infinity == z2.infinity && z1.infinity != 0 {
		return 0
	}
	if z1 == PInfinity {
		return 1
	}
	if z1 == NInfinity {
		return -1
	}
	if z2 == NInfinity {
		return 1
	}
	if z2 == PInfinity {
		return -1
	}
	return z1.integer.Cmp(z2.integer)
}

func Max(zs ...Z) Z {
	if len(zs) == 0 {
		panic("Max called with no arguments")
	}
	if len(zs) == 1 {
		return zs[0]
	}
	ret := zs[0]
	for _, z := range zs[1:] {
		if z.Cmp(ret) == 1 {
			ret = z
		}
	}
	return ret
}

func Min(zs ...Z) Z {
	if len(zs) == 0 {
		panic("Min called with no arguments")
	}
	if len(zs) == 1 {
		return zs[0]
	}
	ret := zs[0]
	for _, z := range zs[1:] {
		if z.Cmp(ret) == -1 {
			ret = z
		}
	}
	return ret
}

var NInfinity = Z{infinity: -1}
var PInfinity = Z{infinity: 1}
var EmptyI = Interval{true, PInfinity, NInfinity}

func InfinityFor(v ssa.Value) Interval {
	if b, ok := v.Type().Underlying().(*types.Basic); ok {
		if (b.Info() & types.IsUnsigned) != 0 {
			return NewInterval(NewZ(&big.Int{}), PInfinity)
		}
	}
	return NewInterval(NInfinity, PInfinity)
}

type Interval struct {
	known bool
	lower Z
	upper Z
}

func NewInterval(l, u Z) Interval {
	if u.Cmp(l) == -1 {
		return EmptyI
	}
	return Interval{known: true, lower: l, upper: u}
}

func (i Interval) IsKnown() bool {
	return i.known
}

func (i Interval) Empty() bool {
	return i.lower == PInfinity && i.upper == NInfinity
}

func (i Interval) IsMaxRange() bool {
	return i.lower == NInfinity && i.upper == PInfinity
}

func (i1 Interval) Intersection(i2 Interval) Interval {
	if !i1.IsKnown() {
		return i2
	}
	if !i2.IsKnown() {
		return i1
	}
	if i1.Empty() || i2.Empty() {
		return EmptyI
	}
	i3 := NewInterval(Max(i1.lower, i2.lower), Min(i1.upper, i2.upper))
	if i3.lower.Cmp(i3.upper) == 1 {
		return EmptyI
	}
	return i3
}

func (i1 Interval) Union(other Range) Range {
	i2, ok := other.(Interval)
	if !ok {
		i2 = EmptyI
	}
	if i1.Empty() || !i1.IsKnown() {
		return i2
	}
	if i2.Empty() || !i2.IsKnown() {
		return i1
	}
	return NewInterval(Min(i1.lower, i2.lower), Max(i1.upper, i2.upper))
}

func (i1 Interval) Add(i2 Interval) Interval {
	if i1.Empty() || i2.Empty() {
		return EmptyI
	}
	l1, u1, l2, u2 := i1.lower, i1.upper, i2.lower, i2.upper
	return NewInterval(l1.Add(l2), u1.Add(u2))
}

func (i1 Interval) Sub(i2 Interval) Interval {
	if i1.Empty() || i2.Empty() {
		return EmptyI
	}
	l1, u1, l2, u2 := i1.lower, i1.upper, i2.lower, i2.upper
	return NewInterval(l1.Sub(u2), u1.Sub(l2))
}

func (i1 Interval) Mul(i2 Interval) Interval {
	if i1.Empty() || i2.Empty() {
		return EmptyI
	}
	x1, x2 := i1.lower, i1.upper
	y1, y2 := i2.lower, i2.upper
	return NewInterval(
		Min(x1.Mul(y1), x1.Mul(y2), x2.Mul(y1), x2.Mul(y2)),
		Max(x1.Mul(y1), x1.Mul(y2), x2.Mul(y1), x2.Mul(y2)),
	)
}

func (i1 Interval) String() string {
	if !i1.IsKnown() {
		return "[⊥, ⊥]"
	}
	if i1.Empty() {
		return "{}"
	}
	return fmt.Sprintf("[%s, %s]", i1.lower, i1.upper)
}

type ArithmeticConstraint struct {
	aConstraint
	A  ssa.Value
	B  ssa.Value
	Op token.Token
	Fn func(Interval, Interval) Interval
}

func NewArithmeticConstraint(a, b, y ssa.Value, op token.Token, fn func(Interval, Interval) Interval) *ArithmeticConstraint {
	return &ArithmeticConstraint{
		aConstraint: aConstraint{
			y: y,
		},
		A:  a,
		B:  b,
		Op: op,
		Fn: fn,
	}
}

func (c *ArithmeticConstraint) Eval(g *Graph) (ret Range) {
	i1, i2 := g.Range(c.A).(Interval), g.Range(c.B).(Interval)
	if !i1.IsKnown() || !i2.IsKnown() {
		return Interval{}
	}
	return c.Fn(i1, i2)
}

func (c *ArithmeticConstraint) String() string {
	return fmt.Sprintf("%s = %s %s %s", c.Y().Name(), c.A.Name(), c.Op, c.B.Name())
}

func (c *ArithmeticConstraint) Operands() []ssa.Value {
	return []ssa.Value{c.A, c.B}
}

type AddConstraint struct{ *ArithmeticConstraint }
type SubConstraint struct{ *ArithmeticConstraint }
type MulConstraint struct{ *ArithmeticConstraint }

func NewAddConstraint(a, b, y ssa.Value) Constraint {
	return &AddConstraint{NewArithmeticConstraint(a, b, y, token.ADD, Interval.Add)}
}
func NewSubConstraint(a, b, y ssa.Value) Constraint {
	return &SubConstraint{NewArithmeticConstraint(a, b, y, token.SUB, Interval.Sub)}
}
func NewMulConstraint(a, b, y ssa.Value) Constraint {
	return &MulConstraint{NewArithmeticConstraint(a, b, y, token.MUL, Interval.Mul)}
}

type IntConversionConstraint struct {
	aConstraint
	X ssa.Value
}

func (c *IntConversionConstraint) Operands() []ssa.Value {
	return []ssa.Value{c.X}
}

func (c *IntConversionConstraint) Eval(g *Graph) Range {
	s := &types.StdSizes{
		// XXX is it okay to assume the largest word size, or do we
		// need to be platform specific?
		WordSize: 8,
		MaxAlign: 1,
	}
	fromI := g.Range(c.X).(Interval)
	toI := g.Range(c.Y()).(Interval)
	fromT := c.X.Type().Underlying().(*types.Basic)
	toT := c.Y().Type().Underlying().(*types.Basic)
	fromB := s.Sizeof(c.X.Type())
	toB := s.Sizeof(c.Y().Type())

	if !fromI.IsKnown() {
		return toI
	}
	if !toI.IsKnown() {
		return fromI
	}

	// uint<N> -> sint/uint<M>, M > N: [max(0, l1), min(2**N-1, u2)]
	if (fromT.Info()&types.IsUnsigned != 0) &&
		toB > fromB {

		n := big.NewInt(1)
		n.Lsh(n, uint(fromB*8))
		n.Sub(n, big.NewInt(1))
		return NewInterval(
			Max(NewZ(&big.Int{}), fromI.lower),
			Min(NewZ(n), toI.upper),
		)
	}

	// sint<N> -> sint<M>, M > N; [max(-∞, l1), min(2**N-1, u2)]
	if (fromT.Info()&types.IsUnsigned == 0) &&
		(toT.Info()&types.IsUnsigned == 0) &&
		toB > fromB {

		n := big.NewInt(1)
		n.Lsh(n, uint(fromB*8))
		n.Sub(n, big.NewInt(1))
		return NewInterval(
			Max(NInfinity, fromI.lower),
			Min(NewZ(n), toI.upper),
		)
	}

	return fromI
}

func (c *IntConversionConstraint) String() string {
	return fmt.Sprintf("%s = %s(%s)", c.Y().Name(), c.Y().Type(), c.X.Name())
}

type FutureIntersectionConstraint struct {
	aConstraint
	ranges      map[ssa.Value]Range
	X           ssa.Value
	lower       ssa.Value
	lowerOffset Z
	upper       ssa.Value
	upperOffset Z
	I           Interval
	resolved    bool
}

func (c *FutureIntersectionConstraint) Futures() []ssa.Value {
	var s []ssa.Value
	if c.lower != nil {
		s = append(s, c.lower)
	}
	if c.upper != nil {
		s = append(s, c.upper)
	}
	return s
}

func (c *FutureIntersectionConstraint) Operands() []ssa.Value {
	return []ssa.Value{c.X}
}

func (c *FutureIntersectionConstraint) Eval(g *Graph) Range {
	xi := g.Range(c.X).(Interval)
	return xi.Intersection(c.I)
}

func (c *FutureIntersectionConstraint) Resolve() {
	l := NInfinity
	u := PInfinity
	if c.lower != nil {
		li, ok := c.ranges[c.lower]
		if !ok {
			li = InfinityFor(c.lower)
		}
		l = li.(Interval).lower
		l = l.Add(c.lowerOffset)
	}
	if c.upper != nil {
		ui, ok := c.ranges[c.upper]
		if !ok {
			ui = InfinityFor(c.upper)
		}
		u = ui.(Interval).upper
		u = u.Add(c.upperOffset)
	}
	c.I = NewInterval(l, u)
}

func (c *FutureIntersectionConstraint) String() string {
	var lname, uname string
	if c.lower != nil {
		lname = c.lower.Name()
	}
	if c.upper != nil {
		uname = c.upper.Name()
	}
	return fmt.Sprintf("%s = %s.%t ⊓ [%s+%s, %s+%s] %s",
		c.Y().Name(), c.X.Name(), c.Y().(*ssa.Sigma).Branch, lname, c.lowerOffset, uname, c.upperOffset, c.I)
}

type IntersectionConstraint struct {
	aConstraint
	X ssa.Value
	I Interval
}

func (c *IntersectionConstraint) Operands() []ssa.Value {
	return []ssa.Value{c.X}
}

func (c *IntersectionConstraint) Eval(g *Graph) Range {
	xi := g.Range(c.X).(Interval)
	if !xi.IsKnown() {
		return c.I
	}
	return xi.Intersection(c.I)
}

func (c *IntersectionConstraint) String() string {
	return fmt.Sprintf("%s = %s.%t ⊓ %s", c.Y().Name(), c.X.Name(), c.Y().(*ssa.Sigma).Branch, c.I)
}

type IntervalConstraint struct {
	aConstraint
	I Interval
}

func (s *IntervalConstraint) Operands() []ssa.Value {
	return nil
}

func (c *IntervalConstraint) Eval(*Graph) Range {
	return c.I
}

func (c *IntervalConstraint) String() string {
	return fmt.Sprintf("%s = %s", c.Y().Name(), c.I)

}
