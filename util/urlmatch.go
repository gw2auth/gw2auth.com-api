package util

import (
	"net/url"
	"strings"
)

type section uint8

const (
	scheme = section(iota)
	host
	path
	query
)

type urlMatcher struct {
	url     string
	offsets [4]int
	cursor  int
}

func newURLMatcher(u *url.URL) *urlMatcher {
	um := urlMatcher{}
	um.url = u.Scheme + "://"
	um.offsets[host] = len(um.url)

	um.url += u.Host
	um.offsets[path] = len(um.url)

	um.url += u.EscapedPath()
	um.offsets[query] = len(um.url)

	if u.RawQuery != "" {
		um.url += "?" + u.RawQuery
	}

	return &um
}

func (um *urlMatcher) find(s string) int {
	idx := strings.Index(um.url[um.cursor:], s)
	if idx == -1 {
		return -1
	}

	return um.cursor + idx
}

func (um *urlMatcher) advance(idx int) map[section][2]string {
	skipped := make(map[section][2]string)

	for _, section := range []section{scheme, host, path, query} {
		if (section >= query || um.cursor < um.offsets[section+1]) && idx > um.offsets[section] {
			var sectionEnd int
			if section >= query {
				sectionEnd = len(um.url)
			} else {
				sectionEnd = um.offsets[section+1]
			}

			start := max(um.cursor, um.offsets[section])
			end := min(sectionEnd, idx)

			skipped[section] = [2]string{
				um.url[start:end],
				um.url[end:sectionEnd],
			}
		}
	}

	um.cursor = idx

	return skipped
}

func (um *urlMatcher) advanceEnd() map[section][2]string {
	return um.advance(len(um.url))
}

func (um *urlMatcher) exhausted() bool {
	return um.cursor >= len(um.url)
}

func URLMatch(pattern string, u *url.URL) bool {
	if !strings.ContainsRune(pattern, '*') {
		return u.String() == pattern
	}

	um := newURLMatcher(u)
	if um.url != u.String() {
		return false
	}

	wildcard := false
	for _, part := range buildParts(pattern) {
		switch part {
		case "*":
			wildcard = true

		default:
			idx := um.find(part)
			if idx == -1 {
				return false
			}

			skipped := um.advance(idx)
			if wildcard {
				if !validWildcardSkip(skipped) {
					return false
				}

				wildcard = false
			} else if len(skipped) > 0 {
				return false
			}

			um.advance(idx + len(part))
		}
	}

	if wildcard {
		skipped := um.advanceEnd()
		if !validWildcardSkip(skipped) {
			return false
		}
	}

	return um.exhausted()
}

func validWildcardSkip(skipped map[section][2]string) bool {
	if len(skipped) != 1 {
		// exactly one section might be (partially) skipped
		return false
	}

	for section, skip := range skipped {
		switch section {
		case host:
			if !validHostSkip(skip) {
				return false
			}

		case path:
			if !validPathSkip(skip) {
				return false
			}

		default:
			// only host and path skips supported
			return false
		}
	}

	return true
}

func validHostSkip(skip [2]string) bool {
	if strings.ContainsRune(skip[0], '.') {
		// not allowed to skip more than one element
		return false
	}

	// remaining part must have at least 2 defined host parts ("*.gw2auth.com" is valid but "*.com" is not)
	return strings.HasPrefix(skip[1], ".") && strings.Count(skip[1], ".") >= 2
}

func validPathSkip(skip [2]string) bool {
	// not allowed to skip more than one element
	return !strings.ContainsRune(skip[0], '/')
}

func buildParts(pattern string) []string {
	parts := make([]string, 0, len(pattern)/2)

	literal := ""
	for _, r := range []rune(pattern) {
		switch r {
		case '*':
			if literal != "" {
				parts = append(parts, literal)
				literal = ""
			}

			parts = append(parts, "*")

		default:
			literal += string(r)
		}
	}

	if literal != "" {
		parts = append(parts, literal)
	}

	return parts
}
