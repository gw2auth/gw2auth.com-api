package util

import (
	"github.com/stretchr/testify/assert"
	"net/url"
	"testing"
)

type testCase struct {
	pattern string
	url     string
	expect  bool
}

func TestAll(t *testing.T) {
	tests := map[string]testCase{

		"equal strings without wildcard should match": {
			pattern: "https://gw2auth.com",
			url:     "https://gw2auth.com",
			expect:  true,
		},
		"simple host wildcard should match": {
			pattern: "https://*.gw2auth.com",
			url:     "https://test.gw2auth.com",
			expect:  true,
		},
		"simple host and path wildcard should match": {
			pattern: "https://*.gw2auth.com/*",
			url:     "https://test.gw2auth.com/test",
			expect:  true,
		},
		"double host wildcard should match": {
			pattern: "https://*.*.gw2auth.com",
			url:     "https://test.test.gw2auth.com",
			expect:  true,
		},
		"path wildcard with suffix should match": {
			pattern: "https://gw2auth.com/*/a",
			url:     "https://gw2auth.com/test/a",
			expect:  true,
		},
		"path wildcard with prefix should match": {
			pattern: "https://gw2auth.com/a/*",
			url:     "https://gw2auth.com/a/test",
			expect:  true,
		},
		"path wildcard with query should match": {
			pattern: "https://gw2auth.com/*?q=a",
			url:     "https://gw2auth.com/test?q=a",
			expect:  true,
		},
		"pattern without wildcard should only match on equal strings": {
			pattern: "https://gw2auth.com",
			url:     "https://test.gw2auth.com",
			expect:  false,
		},
		"wildcard should only match one element": {
			pattern: "https://*.gw2auth.com",
			url:     "https://test.test.gw2auth.com",
			expect:  false,
		},
		"host wildcard must be followed by at least 2 more known elements (1)": {
			pattern: "https://*.com",
			url:     "https://gw2auth.com",
			expect:  false,
		},
		"host wildcard must be followed by at least 2 more known elements (2)": {
			pattern: "https://*.*.com",
			url:     "https://test.gw2auth.com",
			expect:  false,
		},
		"host wildcard must be followed by at least 2 more known elements (3)": {
			pattern: "https://*.*.*",
			url:     "https://test.gw2auth.com",
			expect:  false,
		},
		"host wildcard must be in place of exactly one element": {
			pattern: "https://*gw2auth.com",
			url:     "https://testgw2auth.com",
			expect:  false,
		},
		"path wildcard should only match one element": {
			pattern: "https://gw2auth.com/*",
			url:     "https://gw2auth.com/a/b",
			expect:  false,
		},
		"query should match": {
			pattern: "https://gw2auth.com/*?q=test",
			url:     "https://gw2auth.com/a?q=somethingelse",
			expect:  false,
		},
		"query wildcards not supported": {
			pattern: "https://gw2auth.com/?q=*",
			url:     "https://gw2auth.com/?q=test",
			expect:  false,
		},
		"scheme wildcards not supported": {
			pattern: "*://gw2auth.com",
			url:     "https://gw2auth.com",
			expect:  false,
		},
		"port wildcards not supported": {
			pattern: "https://gw2auth.com:*",
			url:     "https://gw2auth.com:443",
			expect:  false,
		},
		"wildcards not supported for URIs with userinfo": {
			pattern: "https://user:password@*.gw2auth.com",
			url:     "https://user:password@test.gw2auth.com",
			expect:  false,
		},
		"wildcards not supported for URIs with fragment": {
			pattern: "https://*.gw2auth.com/#fragment",
			url:     "https://test.gw2auth.com/#fragment",
			expect:  false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if assert.NoError(t, err) {
				result := URLMatch(test.pattern, u)
				assert.Equal(t, test.expect, result)
			}
		})
	}
}
