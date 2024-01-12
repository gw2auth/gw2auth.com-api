package service

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"reflect"
	"testing"
)

func TestSerde(t *testing.T) {
	kOrig, err := NewKeyAndIv()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	b := kOrig.ToBytes()
	kResolved, err := NewKeyAndIvFromBytes(b)

	if err != nil {
		t.Fatalf("FromBytes failed: %v", err)
	}

	if !reflect.DeepEqual(kOrig, kResolved) {
		t.Fatalf("kOrig and kResolved are not equal")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	for _, v := range []int{0, 2, 3, 5, 7, 11, 13, 17, 19, 23, 37, 928, 1271, 8383} {
		t.Run(fmt.Sprintf("input size %d", v), func(t *testing.T) {
			testEncryptDecryptSize(t, v)
		})
	}
}

func testEncryptDecryptSize(t *testing.T, size int) {
	k, err := NewKeyAndIv()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	src := make([]byte, size)
	if _, err = rand.Read(src); err != nil {
		t.Fatalf("failed to create random input: %v", err)
	}

	encr, err := k.Encrypt(src)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decr, err := k.Decrypt(encr)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decr, src) {
		t.Fatalf("decr and src are not equal")
	}
}
