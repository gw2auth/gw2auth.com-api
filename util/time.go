package util

import "time"

var unixZero = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

func UnixZero() time.Time {
	return unixZero
}
