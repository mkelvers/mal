package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type config struct {
	email         string
	password      string
	passwordStdin bool
	dbFile        string
}

func trimTrailingNewline(v string) string {
	v = strings.TrimSuffix(v, "\n")
	v = strings.TrimSuffix(v, "\r")
	return v
}

func parseConfig() (config, error) {
	var cfg config

	flag.StringVar(&cfg.email, "email", "", "User email/username")
	flag.StringVar(&cfg.password, "password", "", "User password")
	flag.BoolVar(&cfg.passwordStdin, "password-stdin", false, "Read password from stdin")
	flag.StringVar(&cfg.dbFile, "db", "", "SQLite database file path")
	flag.Parse()

	args := flag.Args()
	if len(args) > 2 {
		return cfg, fmt.Errorf("too many arguments")
	}

	if cfg.email == "" && len(args) >= 1 {
		cfg.email = args[0]
	}

	if cfg.password == "" && len(args) == 2 {
		cfg.password = args[1]
	}

	if cfg.password != "" && cfg.passwordStdin {
		return cfg, fmt.Errorf("use either --password or --password-stdin")
	}

	cfg.email = strings.TrimSpace(cfg.email)
	if cfg.email == "" {
		return cfg, fmt.Errorf("email is required")
	}

	if cfg.passwordStdin {
		passwordBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return cfg, fmt.Errorf("read password from stdin: %w", err)
		}
		cfg.password = trimTrailingNewline(string(passwordBytes))
	}

	if cfg.password == "" {
		return cfg, fmt.Errorf("password is required")
	}

	if cfg.dbFile == "" {
		cfg.dbFile = os.Getenv("DATABASE_FILE")
	}

	if cfg.dbFile == "" {
		cfg.dbFile = "mal.db"
	}

	if _, err := os.Stat(cfg.dbFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, fmt.Errorf("database file not found: %s", cfg.dbFile)
		}
		return cfg, fmt.Errorf("inspect database file: %w", err)
	}

	return cfg, nil
}

func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", cfg.dbFile))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	_, err = db.Exec(
		"INSERT INTO user (id, username, password_hash) VALUES (?, ?, ?)",
		uuid.NewString(),
		cfg.email,
		string(hash),
	)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return fmt.Errorf("user already exists: %s", cfg.email)
		}
		return fmt.Errorf("insert user: %w", err)
	}

	fmt.Printf("created user: %s\n", cfg.email)
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [--db path] [--email email] [--password value | --password-stdin] [email] [password]\n", os.Args[0])
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintf(os.Stderr, "  %s --db /app/data/mal.db --email admin@example.com --password 'strong-pass'\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  printf 'strong-pass' | %s --email admin@example.com --password-stdin\n", os.Args[0])
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Defaults:")
		fmt.Fprintln(os.Stderr, "  --db uses DATABASE_FILE, then mal.db")
		flag.PrintDefaults()
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
}
