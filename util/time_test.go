package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUnixZero(t *testing.T) {
	assert.Equal(t, UnixZero().Unix(), int64(0))
}
