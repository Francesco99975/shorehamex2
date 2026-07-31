package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/helpers"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func seedDeveloperAccount(ctx context.Context) {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatalf("Unable to generate UUID: %v", err)
	}
	username := boot.Environment.PostgresUser
	email := boot.Environment.DevEmail
	password, err := helpers.GenerateSecurePassword(16)
	if err != nil {
		log.Fatalf("Unable to generate password: %v", err)
	}

	tx, err := Pool().BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		log.Fatalf("Unable to start transaction: %v", err)
	}
	defer HandleTransaction(ctx, tx, &err)
	repo := repository.New(tx)

	exists, err := repo.ExistDeveloperAccount(ctx)
	if err != nil {
		log.Fatalf("Unable to check if developer account exists: %v", err)
	}

	if !exists {
		hashedPassword, err := helpers.HashPassword(password)
		if err != nil {
			log.Fatalf("Unable to hash password: %v", err)
		}

		_, err = repo.CreateUser(ctx, repository.CreateUserParams{
			ID:           id,
			Username:     username,
			FullName:     "Developer",
			Role:         enums.Roles.DEVELOPER.String(),
			Email:        email,
			PasswordHash: hashedPassword,
		})

		if err != nil {
			log.Fatalf("Unable to create developer account: %v", err)
		}

		err = writeInitialCredentials(username, email, password)
		if err != nil {
			log.Fatalf("Unable to write initial credentials: %v", err)
		}
	}
}

const (
	dirPerm  = 0750
	filePerm = 0600
)

func dataDir() string {
	switch boot.Environment.GoEnv {
	case enums.Environments.DEVELOPMENT:
		return "./data"
	default: // production, staging, etc.
		return "/data"
	}
}

func credentialFilePath() string {
	return filepath.Join(dataDir(), "initial_credentials")
}

func writeInitialCredentials(username string, email string, password string) error {
	// Ensure /data exists with correct permissions
	if err := helpers.EnsureDir(dataDir(), dirPerm); err != nil {
		return fmt.Errorf("failed to prepare credentials directory: %w", err)
	}

	content := fmt.Sprintf(`=====================================
  DEVELOPER SEED ACCOUNT CREDENTIALS
  Generated: %s
=====================================

  Username : %s
  Email    : %s
  Password : %s

-------------------------------------
  ACTION REQUIRED
-------------------------------------

  1. Log in with the credentials above.
  2. You will be prompted to set a new password immediately.
  3. Once changed, this file will be deleted automatically.

  Until you do this, the auth service is NOT production-safe.

=====================================
`, time.Now().UTC().Format(time.RFC1123), username, email, password)

	if err := os.WriteFile(credentialFilePath(), []byte(content), filePerm); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	// Correct permissions if the file already existed with wrong perms
	if err := os.Chmod(credentialFilePath(), filePerm); err != nil {
		return fmt.Errorf("failed to set credentials file permissions: %w", err)
	}

	return nil
}

func DeleteCredentialsFile() error {
	if credentialsFileExists() {
		if err := os.Remove(credentialFilePath()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete credentials file: %w", err)
		}
	}
	return nil
}

func credentialsFileExists() bool {
	_, err := os.Stat(credentialFilePath())
	return !os.IsNotExist(err)
}
