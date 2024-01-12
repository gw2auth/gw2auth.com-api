package util

type Set[T comparable] map[T]struct{}

func NewSet[T comparable](values ...T) Set[T] {
	s := make(Set[T])
	for _, v := range values {
		s[v] = struct{}{}
	}

	return s
}

func (s Set[T]) Add(v T) bool {
	_, present := s[v]
	s[v] = struct{}{}
	return !present
}

func (s Set[T]) Remove(v T) bool {
	_, present := s[v]
	delete(s, v)
	return present
}

func (s Set[T]) Contains(v T) bool {
	_, present := s[v]
	return present
}

func (s Set[T]) ContainsAll(other Set[T]) bool {
	for v := range other {
		if !s.Contains(v) {
			return false
		}
	}

	return true
}

func (s Set[T]) Intersect(other Set[T]) Set[T] {
	r := make(Set[T])
	for v := range s {
		if other.Contains(v) {
			r.Add(v)
		}
	}

	return r
}
