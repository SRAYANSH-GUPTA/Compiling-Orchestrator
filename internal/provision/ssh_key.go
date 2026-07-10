package provision

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

func GetOrCreateSystemSSHKey(keyPath string) (string, string, error) {
	privPath := keyPath
	pubPath := keyPath + ".pub"

	privBytes, err := os.ReadFile(privPath)
	if err == nil {
		pubBytes, err := os.ReadFile(pubPath)
		if err == nil {
			return string(privBytes), string(pubBytes), nil
		}
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	}
	privPEM := pem.EncodeToMemory(privBlock)

	pubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubAuthFormat := ssh.MarshalAuthorizedKey(pubKey)

	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(pubPath, pubAuthFormat, 0644); err != nil {
		return "", "", err
	}

	return string(privPEM), string(pubAuthFormat), nil
}
