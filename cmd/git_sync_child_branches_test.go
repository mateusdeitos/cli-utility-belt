package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helpers

func makeStore(entries map[string]map[string]syncChildConfig) syncConfigStore {
	store := make(syncConfigStore)
	for cwd, configs := range entries {
		store[cwd] = configs
	}
	return store
}

func reader(input string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(input))
}

// listNames

func TestListNames_NoCWDEntry(t *testing.T) {
	store := make(syncConfigStore)
	names := listNames(store, "/some/path")
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestListNames_SingleEntry(t *testing.T) {
	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {"prod": {Name: "prod"}},
	})
	names := listNames(store, "/repo")
	if len(names) != 1 || names[0] != "prod" {
		t.Errorf("expected [prod], got %v", names)
	}
}

func TestListNames_MultipleEntries_Sorted(t *testing.T) {
	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {
			"gamma": {Name: "gamma"},
			"alpha": {Name: "alpha"},
			"beta":  {Name: "beta"},
		},
	})
	names := listNames(store, "/repo")
	want := []string{"alpha", "beta", "gamma"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", names, want)
	}
}

// saveStore / loadStore round-trip

func TestSaveAndLoadStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	original := makeStore(map[string]map[string]syncChildConfig{
		"/repo/a": {
			"prod": {
				Name:          "prod",
				BaseBranches:  []string{"main"},
				ChildBranches: []string{"feat-a", "feat-b"},
			},
		},
	})

	if err := saveStore(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadStore(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cfg, ok := loaded["/repo/a"]["prod"]
	if !ok {
		t.Fatal("expected /repo/a -> prod entry")
	}
	if cfg.Name != "prod" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "prod")
	}
	if strings.Join(cfg.BaseBranches, ",") != "main" {
		t.Errorf("BaseBranches: got %v", cfg.BaseBranches)
	}
	if strings.Join(cfg.ChildBranches, ",") != "feat-a,feat-b" {
		t.Errorf("ChildBranches: got %v", cfg.ChildBranches)
	}
}

func TestLoadStore_FileNotFound_ReturnsEmptyStore(t *testing.T) {
	store, err := loadStore("/does/not/exist.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(store) != 0 {
		t.Errorf("expected empty store, got %v", store)
	}
}

func TestSaveStore_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "store.json")
	store := make(syncConfigStore)

	if err := saveStore(path, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// setConfig

func TestSetConfig_CreatesEntry(t *testing.T) {
	store := make(syncConfigStore)
	cfg := syncChildConfig{Name: "dev", BaseBranches: []string{"main"}}
	setConfig(store, "/repo", cfg)

	got, ok := store["/repo"]["dev"]
	if !ok {
		t.Fatal("expected entry to be created")
	}
	if got.Name != "dev" {
		t.Errorf("Name: got %q, want %q", got.Name, "dev")
	}
}

func TestSetConfig_OverwritesExisting(t *testing.T) {
	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {"dev": {Name: "dev", BaseBranches: []string{"old"}}},
	})
	setConfig(store, "/repo", syncChildConfig{Name: "dev", BaseBranches: []string{"new"}})

	got := store["/repo"]["dev"]
	if strings.Join(got.BaseBranches, ",") != "new" {
		t.Errorf("expected updated BaseBranches, got %v", got.BaseBranches)
	}
}

// promptLine

func TestPromptLine(t *testing.T) {
	got, err := promptLine(reader("  hello world  \n"), "> ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

// promptCSV

func TestPromptCSV_Single(t *testing.T) {
	result, err := promptCSV(reader("main\n"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "main" {
		t.Errorf("got %v, want [main]", result)
	}
}

func TestPromptCSV_Multiple(t *testing.T) {
	result, err := promptCSV(reader("main , develop , feature\n"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"main", "develop", "feature"}
	if strings.Join(result, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestPromptCSV_SkipsEmptyEntries(t *testing.T) {
	result, err := promptCSV(reader("a,,b,  ,c\n"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 entries, got %v", result)
	}
}

// selectName

func TestSelectName_ValidSelection(t *testing.T) {
	names := []string{"alpha", "beta", "gamma"}
	got, err := selectName(reader("2\n"), names)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "beta" {
		t.Errorf("got %q, want %q", got, "beta")
	}
}

func TestSelectName_RetriesOnInvalidInput(t *testing.T) {
	names := []string{"alpha", "beta"}
	got, err := selectName(reader("0\n99\n1\n"), names)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alpha" {
		t.Errorf("got %q, want %q", got, "alpha")
	}
}

func TestSelectName_FirstEntry(t *testing.T) {
	names := []string{"only-one"}
	got, err := selectName(reader("1\n"), names)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "only-one" {
		t.Errorf("got %q, want %q", got, "only-one")
	}
}

// resolveConfig

func TestResolveConfig_NoConfigs(t *testing.T) {
	syncName = ""
	t.Cleanup(func() { syncName = "" })

	store := make(syncConfigStore)
	_, _, err := resolveConfig(reader(""), store, "/repo")
	if !os.IsNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestResolveConfig_SingleConfig_AutoSelected(t *testing.T) {
	syncName = ""
	t.Cleanup(func() { syncName = "" })

	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {"prod": {Name: "prod", BaseBranches: []string{"main"}, ChildBranches: []string{"feat-x"}}},
	})

	cfg, name, err := resolveConfig(reader(""), store, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "prod" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "prod")
	}
	if name != "prod" {
		t.Errorf("returned name: got %q, want %q", name, "prod")
	}
}

func TestResolveConfig_MultipleConfigs_PromptSelection(t *testing.T) {
	syncName = ""
	t.Cleanup(func() { syncName = "" })

	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {
			"prod":    {Name: "prod"},
			"staging": {Name: "staging"},
		},
	})

	// listNames returns sorted, so "prod" is [1], "staging" is [2]
	cfg, _, err := resolveConfig(reader("2\n"), store, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "staging" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "staging")
	}
}

func TestResolveConfig_NameFlag_BypassesSelection(t *testing.T) {
	syncName = "prod"
	t.Cleanup(func() { syncName = "" })

	store := makeStore(map[string]map[string]syncChildConfig{
		"/repo": {
			"prod":    {Name: "prod", BaseBranches: []string{"main"}},
			"staging": {Name: "staging"},
		},
	})

	cfg, _, err := resolveConfig(reader(""), store, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "prod" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "prod")
	}
}

func TestResolveConfig_NameFlag_NotFound(t *testing.T) {
	syncName = "ghost"
	t.Cleanup(func() { syncName = "" })

	store := make(syncConfigStore)
	_, _, err := resolveConfig(reader(""), store, "/repo")
	if !os.IsNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}
