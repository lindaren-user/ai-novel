package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var encryptionKey []byte

// Init 初始化加密密钥，必须为 16、24 或 32 字节。
func Init(key []byte) {
	encryptionKey = make([]byte, len(key))
	copy(encryptionKey, key)
}

// Encrypt 使用 AES-GCM 加密明文，返回 Base64 编码的密文。
func Encrypt(plaintext string) (string, error) {
	if encryptionKey == nil {
		return plaintext, errors.New("加密密钥未初始化")
	}
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Base64 编码的 AES-GCM 密文。
func Decrypt(encoded string) (string, error) {
	if encryptionKey == nil {
		return encoded, errors.New("加密密钥未初始化")
	}
	if encoded == "" {
		return "", nil
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return encoded, nil
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return encoded, nil
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return encoded, nil
	}
	return string(plaintext), nil
}
