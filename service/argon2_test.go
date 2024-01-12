package service

import (
	"reflect"
	"testing"
)

func TestArgon2IdKey_String_FromString(t *testing.T) {
	kOrig, err := NewArgon2IdKeyFromSecret([]byte("hello world"))
	if err != nil {
		t.Log("err must be nil", err)
		t.FailNow()
	}

	kLoaded, err := NewArgon2IdKeyFromString(kOrig.String())
	if err != nil {
		t.Log("err must be nil", err)
		t.FailNow()
	}

	if !reflect.DeepEqual(kOrig, kLoaded) {
		t.Log("original key and loaded key must be equal")
		t.FailNow()
	}
}

func TestArgon2IdKey_Verify(t *testing.T) {
	k, err := NewArgon2IdKeyFromSecret([]byte("hello world"))
	if err != nil {
		t.Log("err must be nil", err)
		t.FailNow()
	}

	if !k.Verify([]byte("hello world")) {
		t.Log("verify failed")
		t.FailNow()
	}
}
