package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// HandleTransaction ensures that a transaction is committed or rolled back properly.
func HandleTransaction(ctx context.Context, tx pgx.Tx, err *error) {
	if p := recover(); p != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			slog.Error("Failed to rollback transaction", slog.Any("error", rollbackErr))
		}
		panic(p) // Re-panic after rollback
	} else if *err != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			slog.Error("Failed to rollback transaction", slog.Any("error", rollbackErr))
		}
	} else {
		commitErr := tx.Commit(ctx)
		if commitErr != nil {
			slog.Error("Failed to commit transaction", slog.Any("error", commitErr))
			*err = fmt.Errorf("commit failed: %w", commitErr)
		}
	}
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func IsPKCollision(constraintName string) func(error) bool {
	return func(err error) bool {
		var pgErr *pgconn.PgError
		return errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == constraintName
	}
}

// NewClassifier returns a classify function that maps a unique-violation on a
// specific constraint to a collision kind. Anything else is NotCollision so it
// propagates out of the retry loop untouched.
func NewClassifier(pkConstraint string, mrnConstraint string) func(error) enums.CollisionKind {
	return func(err error) enums.CollisionKind {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" { // 23505 = unique_violation
			return enums.NotCollision
		}
		switch pgErr.ConstraintName {
		case pkConstraint:
			return enums.UUIDCollision
		case mrnConstraint:
			return enums.MRNCollision
		default:
			return enums.NotCollision // some *other* unique index — a real error, don't retry
		}
	}
}
