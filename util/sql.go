package util

import (
	"fmt"
	"strconv"
)

type SQLBinOp func(lhs, rhs string) string

var (
	SQLBinOpEQ          = simpleBinOp("=")
	SQLBinOpNEQ         = simpleBinOp("!=")
	SQLBinOpGT          = simpleBinOp(">")
	SQLBinOpGTE         = simpleBinOp(">=")
	SQLBinOpLT          = simpleBinOp("<")
	SQLBinOpLTE         = simpleBinOp("<=")
	SQLBinOpContains    = simpleBinOp("&&")
	SQLBinOpNotContains = SQLBinOpContains.Neg()
)

func (op SQLBinOp) Neg() SQLBinOp {
	return func(lhs, rhs string) string {
		return fmt.Sprintf("NOT (%s)", op(lhs, rhs))
	}
}

func simpleBinOp(op string) SQLBinOp {
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
	b.expr = append(b.expr, expr(b.offset+len(b.params)))
	b.params = append(b.params, v)
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
