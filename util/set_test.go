package util

import (
	"maps"
	"testing"
)

func TestNewSet(t *testing.T) {
	s := NewSet("a", "b", "a")
	if len(s) != 2 {
		t.FailNow()
	}

	if !maps.Equal(s, map[string]struct{}{"a": {}, "b": {}}) {
		t.FailNow()
	}
}

func TestSet_Add(t *testing.T) {
	s := make(Set[string])
	if !s.Add("a") {
		t.FailNow()
	}

	if s.Add("a") {
		t.FailNow()
	}
}

func TestSet_Remove(t *testing.T) {
	s := NewSet("a")
	if !s.Remove("a") {
		t.FailNow()
	}

	if s.Remove("a") {
		t.FailNow()
	}
}

func TestSet_Contains(t *testing.T) {
	s := NewSet("a")
	if !s.Contains("a") {
		t.FailNow()
	}
}

func TestSet_ContainsAll(t *testing.T) {
	s1 := NewSet("a", "b")
	s2 := NewSet("a")

	if !s1.ContainsAll(s2) {
		t.FailNow()
	}

	if s2.ContainsAll(s1) {
		t.FailNow()
	}
}

func TestSet_Intersect(t *testing.T) {
	s1 := NewSet("a", "b")
	s2 := NewSet("a")

	if !maps.Equal(s1.Intersect(s2), s2) {
		t.FailNow()
	}

	if !maps.Equal(s2.Intersect(s1), s2) {
		t.FailNow()
	}
}
