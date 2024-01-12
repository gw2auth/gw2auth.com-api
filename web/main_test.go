package web

import (
	"github.com/gw2auth/gw2auth.com-api/internal/test"
	"os"
	"testing"
)

var dbScope *test.Scope

func TestMain(t *testing.M) {
	var code int
	test.WithScope(func(scope *test.Scope) {
		dbScope = scope
		code = t.Run()
		dbScope = nil
	})

	os.Exit(code)
}
