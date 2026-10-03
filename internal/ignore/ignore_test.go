package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

var square = Entry{Source: "SQUARE ONE", Library: "/lib/Square Two"}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "ignore.json")

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Has(Upgrades, square) {
		t.Fatal("empty store must not match")
	}
	s.Set(Upgrades, square, true)
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}

	s, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(Upgrades, square) {
		t.Error("saved entry not found after reload")
	}
	if s.Has(Upgrades, Entry{Source: "SQUARE ONE", Library: "/lib/Other"}) {
		t.Error("entry must match on both fields")
	}
}

func TestStoreFeaturesAreSeparate(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "ignore.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Set(Upgrades, square, true)
	if s.Has("import", square) {
		t.Error("an upgrades ignore must not apply to import")
	}
}

func TestStoreUnignore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Set(Upgrades, square, true)
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}
	s.Set(Upgrades, square, false)
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}

	s, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Has(Upgrades, square) {
		t.Error("unignored entry still present after reload")
	}
}

func TestSaveSkipsWriteWhenUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Set(Upgrades, square, false)
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file must not be written without changes, stat err: %v", err)
	}
}

func TestStoreNormalizesUnicode(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "ignore.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Set(Upgrades, Entry{Source: "Béyoncé"}, true)
	if !s.Has(Upgrades, Entry{Source: "Béyoncé"}) {
		t.Error("NFD and NFC forms must match")
	}
}

func TestLoadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected parse error")
	}
}
