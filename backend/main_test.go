package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCommandUsage(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"fetch-geoip", "--nope"}, {"fetch-geoip", "--force", "extra"}} {
		if code := runCommand(args); code != 2 {
			t.Errorf("runCommand(%v) = %d, want 2", args, code)
		}
	}
}

func TestFetchGeoIPKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"c.mmdb", "a.mmdb"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GEOIP_CITY_DB", filepath.Join(dir, "c.mmdb"))
	t.Setenv("GEOIP_ASN_DB", filepath.Join(dir, "a.mmdb"))
	t.Setenv("FORCE", "")
	// Present and no --force: it must succeed without touching the network or the files.
	if code := runCommand([]string{"fetch-geoip"}); code != 0 {
		t.Errorf("exit code %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "c.mmdb")); string(b) != "x" {
		t.Error("existing file was replaced")
	}
}
