package jwks

import (
	jwksservice "github.com/disillusioned-labs/identity/internal/service/jwks"
)

type JwksResponse struct {
	Keys []JwksKeyResponse `json:"keys"`
}

type JwksKeyResponse struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func toJwksResponse(jwksKeyOutput []jwksservice.JwksKeyOutput) JwksResponse {
	var keys []JwksKeyResponse

	for _, keyOutput := range jwksKeyOutput {
		jwksKeyResponse := toJwksKeyResponse(keyOutput)
		keys = append(keys, jwksKeyResponse)
	}

	return JwksResponse{Keys: keys}
}

func toJwksKeyResponse(key jwksservice.JwksKeyOutput) JwksKeyResponse {
	return JwksKeyResponse{
		Kid: key.Kid,
		Kty: key.Kty,
		Alg: key.Alg,
		Use: key.Use,
		N:   key.N,
		E:   key.E,
	}
}
