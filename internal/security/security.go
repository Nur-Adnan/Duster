package security

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nur-Adnan/duster/lib/fs"
)

// IsPathSafe checks if the destination path is resolved and fully authorized for cleanups.
// Explicitly protects: C:\Windows, System32, Program Files, Boot, Recovery, EFI, System Volume Info, and root drives.
func IsPathSafe(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	// Canonicalize and resolve path variables
	resolved := strings.ToLower(filepath.Clean(fs.ResolveEnvPath(path)))
	resolved = strings.ReplaceAll(resolved, "/", "\\")

	// Verify standard security checks
	if fs.IsSystemProtectedPath(resolved) {
		return false
	}

	// Additional structural protections
	forbiddenPrefixes := []string{
		`c:\boot`,
		`c:\recovery`,
		`c:\efi`,
		`c:\system volume information`,
		`c:\$winreagent`,
		`c:\windows\installer`,
	}

	for _, prefix := range forbiddenPrefixes {
		if resolved == prefix || strings.HasPrefix(resolved, prefix+`\`) {
			return false
		}
	}

	return true
}

// DusterPublicKeyPEM represents the embedded public key used to verify self-update release packages.
//
// ╔══════════════════════════════════════════════════════════════════════╗
// ║  ⚠️  TODO: REPLACE WITH PRODUCTION RSA-2048 KEY BEFORE SHIPPING   ║
// ║                                                                    ║
// ║  Generate a real keypair:                                          ║
// ║    openssl genrsa -out duster_private.pem 2048                     ║
// ║    openssl rsa -in duster_private.pem -pubout -out duster_pub.pem  ║
// ║                                                                    ║
// ║  The key below is a PLACEHOLDER and will fail any real             ║
// ║  signature verification. It must NOT ship in production.           ║
// ╚══════════════════════════════════════════════════════════════════════╝
const DusterPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzqWfI3R3PzQp7Qy6Kz1m
9n/c/9mYmP5NfG3G+vH/8Xw2Bf8zFq4J2kPj7R7cKjT1h1ZqjT1h1ZqjT1h1ZqjT
1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1
h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h
1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1
ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1ZqjT1h1Z
wIDAQAB
-----END PUBLIC KEY-----`

// VerifyPayloadSignature checks if the binary matches the cryptographic RSA-2048 public key signature.
func VerifyPayloadSignature(payload []byte, signature []byte) error {
	block, _ := pem.Decode([]byte(DusterPublicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("not an RSA public key")
	}
	hashed := sha256.Sum256(payload)
	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], signature)
}
