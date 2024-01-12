package util

import (
	"slices"
	"testing"
)

func TestUnique(t *testing.T) {
	r := Unique([]string{"a", "b", "a"})
	if !slices.Equal(r, []string{"a", "b"}) {
		t.FailNow()
	}
}
