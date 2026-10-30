// sign signs a file as a release's SHA256SUMS is signed: Ed25519, the key in PEM (PKCS #8), the 64 raw bytes of
// the signature in <file>.sig. The Windows update test uses it with test-release.key.pem, OpenSSL 3 not being at
// hand there.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: sign <key.pem> <file>")
		os.Exit(2)
	}
	if err := sign(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sign(keyPath, path string) error {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("%s: no PEM block", keyPath)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("%s: not an Ed25519 key", keyPath)
	}
	msg, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".sig", ed25519.Sign(priv, msg), 0o644)
}
