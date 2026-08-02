// genkey generates an RSA-2048 keypair, encrypts the private key with the
// master key from AUTH_MASTER_KEY env var, and prints a ready-to-run SQL
// INSERT for the signing_keys table.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/disillusioned-labs/identity/internal/platform/crypto"
	"github.com/google/uuid"
)

func main() {
	masterKeyHex := os.Getenv("AUTH_MASTER_KEY")
	if len(masterKeyHex) != 64 {
		fmt.Fprintln(os.Stderr, "AUTH_MASTER_KEY must be a 64-char hex string (32 bytes)")
		os.Exit(1)
	}
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode AUTH_MASTER_KEY: %v\n", err)
		os.Exit(1)
	}

	privPEM, pubPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate keypair: %v\n", err)
		os.Exit(1)
	}

	encrypted, err := crypto.EncryptPrivateKey(privPEM, masterKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encrypt private key: %v\n", err)
		os.Exit(1)
	}

	kid := uuid.New().String()
	encHex := hex.EncodeToString(encrypted)

	// SHA-256 of the public key as a quick fingerprint for the comment
	sum := sha256.Sum256(pubPEM)
	fp := hex.EncodeToString(sum[:8])

	fmt.Printf("-- kid: %s  pub fingerprint: %s\n", kid, fp)
	fmt.Printf("INSERT INTO signing_keys (kid, private_key_encrypted, public_key, algorithm, is_active)\n")
	fmt.Printf("VALUES (\n")
	fmt.Printf("  '%s',\n", kid)
	fmt.Printf("  decode('%s', 'hex'),\n", encHex)
	fmt.Printf("  '%s',\n", string(pubPEM))
	fmt.Printf("  'RS256',\n")
	fmt.Printf("  true\n")
	fmt.Printf(");\n")
}
