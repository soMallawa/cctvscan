package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerPrivateConfiguration(t *testing.T) {
	root := t.TempDir()
	key := filepath.Join(root, "key")
	dsn := filepath.Join(root, "database")
	os.WriteFile(key, make([]byte, 32), 0600)
	os.WriteFile(dsn, []byte("postgres://fixture@127.0.0.1/fixture"), 0600)
	binary := filepath.Join(root, "scanner")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CCTVSCAN_SECRET_KEY_FILE": key, "CCTVSCAN_DATABASE_URL_FILE": dsn, "CCTVSCAN_SCANNER_BIN": binary, "CCTVSCAN_WORKER_ROOT": filepath.Join(root, "jobs")}
	get := func(k string) string { return env[k] }
	c, e := loadConfig(get)
	if e != nil || len(c.key) != 32 || c.binary != binary {
		t.Fatal("valid private configuration rejected", e)
	}
	os.Chmod(key, 0644)
	if _, e = loadConfig(get); e == nil {
		t.Fatal("world-readable encryption key accepted")
	}
	os.Chmod(key, 0600)
	env["CCTVSCAN_DATABASE_URL"] = "must-not-appear-in-errors"
	if _, e = loadConfig(get); e == nil {
		t.Fatal("ambiguous database secrets accepted")
	}
	delete(env, "CCTVSCAN_DATABASE_URL")
	env["CCTVSCAN_WORKER_ROOT"] = "/"
	if _, e = loadConfig(get); e == nil {
		t.Fatal("unsafe filesystem root accepted")
	}
}
