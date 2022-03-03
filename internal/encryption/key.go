package encryption

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
)

func GenerateRSAKey(pin string) (string, error) {
	rnd := rand.Reader
	key, err := rsa.GenerateKey(rnd, 2048)
	if err != nil {
		return "", err
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	block, err := x509.EncryptPEMBlock(
		rnd,
		"RSA PRIVATE KEY",
		keyBytes,
		[]byte(pin),
		x509.PEMCipherAES256,
	)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(block)), nil
}
