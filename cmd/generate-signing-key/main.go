package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/disillusioned-labs/identity/internal/config"
	"github.com/disillusioned-labs/identity/internal/platform/crypto"
	"github.com/disillusioned-labs/identity/internal/platform/postgres"
	"github.com/disillusioned-labs/identity/internal/repository"

	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	rotate := flag.Bool("rotate", false,
		"deactivate the current active key in the same transaction; without it a second key is rejected by ux_signing_keys_active")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	masterKey, err := cfg.Auth.MasterKeyBytes()
	if err != nil {
		return fmt.Errorf("decode auth master key: %w", err)
	}

	privPEM, pubPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}

	encrypted, err := crypto.EncryptPrivateKey(privPEM, masterKey)
	if err != nil {
		return fmt.Errorf("encrypt private key: %w", err)
	}

	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	kid := uuid.New().String()
	repo := repository.NewStore(pool)

	err = repo.ExecTx(ctx, func(q repository.Querier) error {
		if *rotate {
			if err := q.RotateSigningKey(ctx); err != nil {
				return fmt.Errorf("deactivate current key: %w", err)
			}
		}
		return q.InsertSigningKey(ctx, repository.InsertSigningKeyParams{
			Kid:                 kid,
			PrivateKeyEncrypted: encrypted,
			PublicKey:           string(pubPEM),
			Algorithm:           "RS256",
			IsActive:            true,
		})
	})
	if err != nil {
		return fmt.Errorf("insert signing key: %w", err)
	}

	fmt.Printf("active signing key inserted: kid=%s alg=RS256\n", kid)
	return nil
}
