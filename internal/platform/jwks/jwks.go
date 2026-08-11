package jwks

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func PublicKeyToJWKS(pubPEM string, kid string) (JWK, error) {
	// PEM decode
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return JWK{}, errors.New("failed to decode PEM")
	}

	// Parse public key (PKIX)
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return JWK{}, errors.New(fmt.Sprintf("failed to parse public key: %w", err))
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return JWK{}, errors.New("public key is not RSA")
	}

	// RSA modulus (n)
	n := base64.RawURLEncoding.EncodeToString(rsaPub.N.Bytes())

	// RSA exponent (e)
	e := base64.RawURLEncoding.EncodeToString(
		[]byte{
			byte(rsaPub.E >> 16),
			byte(rsaPub.E >> 8),
			byte(rsaPub.E),
		},
	)

	jwks := JWK{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   n,
		E:   e,
	}

	return jwks, nil
}
