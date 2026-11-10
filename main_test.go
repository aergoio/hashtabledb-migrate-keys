package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aergoio/hashtabledb"
)

const testKey = "dpos.LibStatus"
const testValue = "legacy-external-value"

func createLegacyDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := hashtabledb.Open(dbPath, hashtabledb.Options{"HashTableSize": 2 * 1024})
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if err := db.SetOption("AddMutableKey", []byte(testKey)); err != nil {
		t.Fatalf("add mutable key: %v", err)
	}
	if err := db.Set([]byte(testKey), []byte(testValue)); err != nil {
		t.Fatalf("set external key: %v", err)
	}
	if err := db.Sync(); err != nil {
		t.Fatalf("sync external key: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	files, err := filepath.Glob(dbPath + "-vk-*")
	if err != nil || len(files) == 0 {
		t.Fatalf("legacy db has no vk file: files=%v err=%v", files, err)
	}
	return dbPath
}

func TestRunMigratesExternalKeys(t *testing.T) {
	dbPath := createLegacyDB(t)

	if err := run(config{dbPath: dbPath, backup: true}); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	files, _ := filepath.Glob(dbPath + "-vk-*")
	if len(files) != 0 {
		t.Fatalf("vk files remain after migration: %v", files)
	}
	backups, _ := filepath.Glob(dbPath + ".vk-backup-*")
	if len(backups) != 1 {
		t.Fatalf("backup dirs = %v, want one", backups)
	}

	db, err := hashtabledb.Open(dbPath, hashtabledb.Options{"HashTableSize": 2 * 1024})
	if err != nil {
		t.Fatalf("reopen migrated db: %v", err)
	}
	defer db.Close()
	got, err := db.Get([]byte(testKey))
	if err != nil || !bytes.Equal(got, []byte(testValue)) {
		t.Fatalf("Get after migration = %q, err=%v, want %q", got, err, testValue)
	}
	if infos := db.ExternalKeyInfos(); len(infos) != 0 {
		t.Fatalf("external keys remain: %v", infos)
	}
}

func TestRunDryRunKeepsExternalKeys(t *testing.T) {
	dbPath := createLegacyDB(t)

	if err := run(config{dbPath: dbPath, dryRun: true}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	files, _ := filepath.Glob(dbPath + "-vk-*")
	if len(files) == 0 {
		t.Fatalf("dry run removed vk files")
	}

	db, err := hashtabledb.Open(dbPath, hashtabledb.Options{"HashTableSize": 2 * 1024})
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()
	if infos := db.ExternalKeyInfos(); len(infos) != 1 {
		t.Fatalf("external keys = %d, want 1", len(infos))
	}
}

func TestResolveDBPathFromDataDir(t *testing.T) {
	dataDir := t.TempDir()
	chainDir := filepath.Join(dataDir, "chain")
	if err := os.MkdirAll(chainDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(chainDir, "data.db")
	if err := os.WriteFile(dbPath, []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveDBPath(config{dataDir: dataDir})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != dbPath {
		t.Fatalf("resolved %q, want %q", got, dbPath)
	}
}
