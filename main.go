package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aergoio/hashtabledb"
)

type config struct {
	dbPath   string
	dataDir  string
	backup   bool
	dryRun   bool
	noVerify bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.dbPath, "db", "", "path to the chain database file (data.db)")
	flag.StringVar(&cfg.dataDir, "datadir", "", "aergo data directory (uses <datadir>/chain/data.db)")
	flag.BoolVar(&cfg.backup, "backup", true, "copy vk files to a timestamped backup directory before deleting them")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "list external keys without changing the database")
	flag.BoolVar(&cfg.noVerify, "no-verify", false, "skip reopening the database and verifying migrated values")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
}

func resolveDBPath(cfg config) (string, error) {
	if cfg.dbPath != "" {
		return cfg.dbPath, nil
	}
	if cfg.dataDir == "" {
		return "", fmt.Errorf("either -db or -datadir is required")
	}

	candidates := []string{
		filepath.Join(cfg.dataDir, "chain", "data.db"),
		filepath.Join(cfg.dataDir, "data.db"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no chain database found under %s", cfg.dataDir)
}

func run(cfg config) error {
	dbPath, err := resolveDBPath(cfg)
	if err != nil {
		return err
	}
	dbPath, err = filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("chain database is not accessible: %w", err)
	}

	fmt.Printf("Opening %s\n", dbPath)
	db, err := hashtabledb.Open(dbPath, hashtabledb.Options{
		"HashTableSize": 2 * 1024,
	})
	if err != nil {
		return fmt.Errorf("open database (is aergosvr stopped?): %w", err)
	}

	infos := db.ExternalKeyInfos()
	if len(infos) == 0 {
		fmt.Println("No external keys found; nothing to migrate.")
		return db.Close()
	}

	fmt.Printf("Found %d external key(s):\n", len(infos))
	for _, info := range infos {
		fmt.Printf("  key=%x value_size=%d files=%v\n", info.Key, info.ValueSize, info.Files)
	}

	if cfg.dryRun {
		fmt.Println("Dry run: no changes made.")
		return db.Close()
	}

	values := make(map[string][]byte, len(infos))
	for _, info := range infos {
		value, err := db.Get(info.Key)
		if err != nil && !errors.Is(err, hashtabledb.ErrKeyNotFound) {
			db.Close()
			return fmt.Errorf("read external key %x: %w", info.Key, err)
		}
		values[string(info.Key)] = append([]byte(nil), value...)
	}

	if cfg.backup {
		backupDir := fmt.Sprintf("%s.vk-backup-%s", dbPath, time.Now().Format("20060102-150405"))
		if err := backupFiles(backupDir, infos); err != nil {
			db.Close()
			return fmt.Errorf("backup external files: %w", err)
		}
		fmt.Printf("Backed up vk files to %s\n", backupDir)
	}

	migrated, err := db.MigrateExternalKeys()
	if err != nil {
		db.Close()
		return fmt.Errorf("migrate external keys: %w", err)
	}
	fmt.Printf("Migrated %d external key(s) to the main append-only file.\n", migrated)

	if err := db.Close(); err != nil {
		return fmt.Errorf("close migrated database: %w", err)
	}

	files, err := filepath.Glob(dbPath + "-vk-*")
	if err != nil {
		return fmt.Errorf("check remaining vk files: %w", err)
	}
	if len(files) != 0 {
		return fmt.Errorf("external value files still present: %v", files)
	}

	if cfg.noVerify {
		return nil
	}
	return verify(dbPath, values)
}

func backupFiles(backupDir string, infos []hashtabledb.ExternalKeyInfo) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	for _, info := range infos {
		for _, file := range info.Files {
			dst := filepath.Join(backupDir, filepath.Base(file))
			if err := copyFile(file, dst); err != nil {
				return err
			}
		}
	}
	dir, err := os.Open(backupDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func verify(dbPath string, values map[string][]byte) error {
	db, err := hashtabledb.Open(dbPath, hashtabledb.Options{
		"HashTableSize": 2 * 1024,
	})
	if err != nil {
		return fmt.Errorf("reopen migrated database: %w", err)
	}
	defer db.Close()

	if infos := db.ExternalKeyInfos(); len(infos) != 0 {
		return fmt.Errorf("external keys remain after migration: %d", len(infos))
	}
	for key, want := range values {
		got, err := db.Get([]byte(key))
		if err != nil && !errors.Is(err, hashtabledb.ErrKeyNotFound) {
			return fmt.Errorf("verify key %x: %w", []byte(key), err)
		}
		if len(want) == 0 && errors.Is(err, hashtabledb.ErrKeyNotFound) {
			continue
		}
		if sha256.Sum256(got) != sha256.Sum256(want) {
			return fmt.Errorf("verify key %x: value mismatch", []byte(key))
		}
	}
	fmt.Printf("Verified %d migrated key(s).\n", len(values))
	return nil
}
