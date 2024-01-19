package util

import (
	"fmt"
	"strconv"
)

type SQLOp func(lhs, rhs string) string

var (
	SQLOpAND            = simpleOp("AND")
	SQLOpOR             = simpleOp("OR")
	SQLBinOpEQ          = simpleOp("=")
	SQLBinOpNEQ         = simpleOp("!=")
	SQLBinOpGT          = simpleOp(">")
	SQLBinOpGTE         = simpleOp(">=")
	SQLBinOpLT          = simpleOp("<")
	SQLBinOpLTE         = simpleOp("<=")
	SQLBinOpContains    = simpleOp("&&")
	SQLBinOpNotContains = SQLBinOpContains.Neg()
)

func (op SQLOp) Neg() SQLOp {
	return func(lhs, rhs string) string {
		return fmt.Sprintf("NOT (%s)", op(lhs, rhs))
	}
}

func simpleOp(op string) SQLOp {
	return func(lhs, rhs string) string {
		return fmt.Sprintf("%s %s %s", lhs, op, rhs)
	}
}

type SQLBuilder struct {
	offset int
	expr   []string
	params []any
}

func NewSQLBuilder(offset int) *SQLBuilder {
	return &SQLBuilder{
		offset: offset,
		expr:   make([]string, 0),
		params: make([]any, 0),
	}
}

func (b *SQLBuilder) Add(v any, expr func(int) string) {
	idx := b.offset + len(b.params)
	b.params = append(b.params, v)
	b.expr = append(b.expr, expr(idx))
}

func (b *SQLBuilder) AddSlice(v []any, expr func([]int) string) {
	l := len(b.params)
	nums := make([]int, len(v))
	for i := 0; i < len(nums); i++ {
		nums[i] = b.offset + l + i
	}

	b.expr = append(b.expr, expr(nums))
	b.params = append(b.params, v...)
}

func (b *SQLBuilder) Get() ([]string, []any) {
	return b.expr, b.params
}

func SQLParam(num int) string {
	return "$" + strconv.Itoa(num)
}

func SQLParams(nums []int) []string {
	values := make([]string, len(nums))
	for i := 0; i < len(nums); i++ {
		values[i] = SQLParam(nums[i])
	}

	return values
}
