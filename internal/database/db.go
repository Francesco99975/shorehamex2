package database

import (
	"context"
	"database/sql"
	"log"

	"github.com/golang-migrate/migrate/v4"
	migpgx "github.com/golang-migrate/migrate/v4/database/pgx/v5" // aliased: clashes with jackc/pgx
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

type actorKey struct{}

// WithActor returns a ctx carrying the acting user's ID. Any query run with
// this ctx (or one derived from it) is attributed to that user in the DB.
func WithActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

func Setup(dsn string) {
	var err error
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Unable to parse database config: %v", err)
	}

	cfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		actor, _ := ctx.Value(actorKey{}).(string) // "" when absent
		// Always overwrite, even with "", so a recycled connection never
		// carries a previous request's user.
		_, err := conn.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", actor)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		log.Fatalf("Unable to ping database: %v", err)
	}

	err = runMigrations(dsn)
	if err != nil {
		log.Fatalf("Unable to run migrations: %v", err)
	}

	seedDeveloperAccount(ctx)

	log.Println("Connected to database")
}

func runMigrations(dsn string) error {
	// Open a database/sql connection for migrations
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalln(err)
		}
	}()
	// Initialize the migration driver
	driver, err := migpgx.WithInstance(db, &migpgx.Config{})
	if err != nil {
		return err
	}

	// Initialize migrate with file source and database driver
	m, err := migrate.NewWithDatabaseInstance(
		"file://sql/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		return err
	}

	// Apply all up migrations
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

// Pool returns the pgxpool.Pool instance for use with sqlc-generated code.
func Pool() *pgxpool.Pool {
	return pool
}

// Close closes the database connection pool.
func Close() {
	if pool != nil {
		pool.Close()
	}
}
