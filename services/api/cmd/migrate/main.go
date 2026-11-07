// Command migrate applies or rolls back the database migrations.
//
// Usage:
//
//	migrate up    apply every pending migration
//	migrate down  roll back the most recent migration
//
// The database is read from DATABASE_URL.
package main

import (
	"errors"
	"fmt"
	"os"

	"readvault/api/internal/adapter/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if len(args) != 1 {
		return errors.New("usage: migrate up|down")
	}

	switch args[0] {
	case "up":
		if err := postgres.MigrateUp(databaseURL); err != nil {
			return err
		}
		fmt.Println("migrations applied")
	case "down":
		if err := postgres.MigrateDown(databaseURL); err != nil {
			return err
		}
		fmt.Println("last migration rolled back")
	default:
		return fmt.Errorf("unknown command %q (want up or down)", args[0])
	}
	return nil
}
