package gw2

import (
	"slices"
	"testing"
)

func TestBitSet(t *testing.T) {
	src := []Permission{
		PermissionAccount,
		PermissionWallet,
		PermissionTradingpost,
	}
	bitSet := PermissionsToBitSet(src)

	if bitSet != 641 {
		t.Fatalf("expected 641, got %v", bitSet)
	}

	dst := PermissionsFromBitSet(bitSet)
	if len(src) != len(dst) {
		t.Fatalf("expected %v, got %v", src, dst)
	}

	for _, v := range src {
		if !slices.Contains(dst, v) {
			t.Fatalf("expected %v, got %v", src, dst)
		}
	}
}
