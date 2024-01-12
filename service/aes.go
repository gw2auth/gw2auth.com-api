package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
)

const keySize = 256 / 8
const ivSize = 16

type KeyAndIv struct {
	key []byte
	iv  []byte
}

func (k KeyAndIv) Encrypt(b []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, err
	}

	ecb := cipher.NewCBCEncrypter(block, k.iv)
	b = pkcs5Padding(b, block.BlockSize())
	encrypted := make([]byte, len(b))
	ecb.CryptBlocks(encrypted, b)

	return encrypted, nil
}

func (k KeyAndIv) Decrypt(b []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, err
	}

	ecb := cipher.NewCBCDecrypter(block, k.iv)
	decrypted := make([]byte, len(b))
	ecb.CryptBlocks(decrypted, b)

	return pkcs5Trimming(decrypted), nil
}

func (k KeyAndIv) ToBytes() []byte {
	b := make([]byte, 0, len(k.key)+len(k.iv)+8)
	b = binary.BigEndian.AppendUint32(b, uint32(len(k.key)))
	b = append(b, k.key...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(k.iv)))
	b = append(b, k.iv...)

	return b
}

func NewKeyAndIv() (KeyAndIv, error) {
	key := make([]byte, keySize)
	iv := make([]byte, ivSize)

	if _, err := rand.Read(key); err != nil {
		return KeyAndIv{}, err
	} else if _, err = rand.Read(iv); err != nil {
		return KeyAndIv{}, err
	}

	return KeyAndIv{key, iv}, nil
}

func NewKeyAndIvFromBytes(b []byte) (KeyAndIv, error) {
	buf := bytes.NewBuffer(b)

	keyLen := int(binary.BigEndian.Uint32(buf.Next(4)))
	key := buf.Next(keyLen)
	if len(key) != keyLen {
		return KeyAndIv{}, errors.New("unexpected key length")
	}

	ivLen := int(binary.BigEndian.Uint32(buf.Next(4)))
	iv := buf.Next(ivLen)
	if len(key) != keyLen {
		return KeyAndIv{}, errors.New("unexpected iv length")
	}

	return KeyAndIv{key, iv}, nil
}

func pkcs5Padding(b []byte, blockSize int) []byte {
	padding := blockSize - len(b)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(b, padtext...)
}

func pkcs5Trimming(b []byte) []byte {
	padding := b[len(b)-1]
	return b[:len(b)-int(padding)]
}
