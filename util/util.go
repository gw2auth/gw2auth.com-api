package util

import "strconv"

func Unique[T comparable](s []T) []T {
	r := make([]T, 0, len(s))
	unq := make(map[T]bool)

	for _, v := range s {
		if present, _ := unq[v]; !present {
			unq[v] = true
			r = append(r, v)
		}
	}

	return r
}

func ParseUint32(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 10, 32)
	return uint32(v), err
}

func ParseUint8(s string) (uint8, error) {
	v, err := strconv.ParseUint(s, 10, 8)
	return uint8(v), err
}
