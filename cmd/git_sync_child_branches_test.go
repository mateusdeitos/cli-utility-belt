package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helpers

func writeConfig(t *testing.T, dir, tag string, cfg syncChildConfig) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := saveSyncConfig(syncConfigPathForTag(dir, tag), &cfg); err != nil {
		t.Fatalf("saveSyncConfig: %v", err)
	}
}

func reader(input string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(input))
}

// syncConfigPathForTag

func TestSyncConfigPathForTag(t *testing.T) {
	got := syncConfigPathForTag("/base/dir", "my-tag")
	want := "/base/dir/my-tag.json"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// listConfigTags

func TestListConfigTags_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	tags, err := listConfigTags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected empty, got %v", tags)
	}
}

func TestListConfigTags_NonExistentDir(t *testing.T) {
	tags, err := listConfigTags("/does/not/exist/ever")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected empty, got %v", tags)
	}
}

func TestListConfigTags_IgnoresNonJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644)
	os.WriteFile(filepath.Join(dir, "subdir"), []byte{}, 0o644)
	writeConfig(t, dir, "valid", syncChildConfig{Tag: "valid"})

	tags, err := listConfigTags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 1 || tags[0] != "valid" {
		t.Errorf("expected [valid], got %v", tags)
	}
}

func TestListConfigTags_MultipleTags(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "alpha", syncChildConfig{Tag: "alpha"})
	writeConfig(t, dir, "beta", syncChildConfig{Tag: "beta"})
	writeConfig(t, dir, "gamma", syncChildConfig{Tag: "gamma"})

	tags, err := listConfigTags(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %v", tags)
	}
}

// saveSyncConfig / loadSyncConfig round-trip

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := syncConfigPathForTag(dir, "test")
	original := &syncChildConfig{
		Tag:           "test",
		BaseBranches:  []string{"main", "develop"},
		ChildBranches: []string{"feat-a", "feat-b"},
	}

	if err := saveSyncConfig(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadSyncConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Tag != original.Tag {
		t.Errorf("Tag: got %q, want %q", loaded.Tag, original.Tag)
	}
	if strings.Join(loaded.BaseBranches, ",") != strings.Join(original.BaseBranches, ",") {
		t.Errorf("BaseBranches: got %v, want %v", loaded.BaseBranches, original.BaseBranches)
	}
	if strings.Join(loaded.ChildBranches, ",") != strings.Join(original.ChildBranches, ",") {
		t.Errorf("ChildBranches: got %v, want %v", loaded.ChildBranches, original.ChildBranches)
	}
}

func TestLoadSyncConfig_FileNotFound(t *testing.T) {
	_, err := loadSyncConfig("/does/not/exist.json")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestSaveSyncConfig_CreatesParentDirs(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "a", "b", "c", "tag.json")
	cfg := &syncChildConfig{Tag: "tag"}

	if err := saveSyncConfig(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
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

// selectTag

func TestSelectTag_ValidSelection(t *testing.T) {
	tags := []string{"alpha", "beta", "gamma"}
	got, err := selectTag(reader("2\n"), tags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "beta" {
		t.Errorf("got %q, want %q", got, "beta")
	}
}

func TestSelectTag_RetriesOnInvalidInput(t *testing.T) {
	tags := []string{"alpha", "beta"}
	// first two inputs are invalid, third is valid
	got, err := selectTag(reader("0\n99\n1\n"), tags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alpha" {
		t.Errorf("got %q, want %q", got, "alpha")
	}
}

func TestSelectTag_FirstEntry(t *testing.T) {
	tags := []string{"only-one"}
	got, err := selectTag(reader("1\n"), tags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "only-one" {
		t.Errorf("got %q, want %q", got, "only-one")
	}
}

// resolveConfig

func TestResolveConfig_NoConfigs(t *testing.T) {
	dir := t.TempDir()
	syncTag = ""
	t.Cleanup(func() { syncTag = "" })

	_, _, err := resolveConfig(reader(""), dir)
	if !os.IsNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestResolveConfig_SingleConfig_AutoSelected(t *testing.T) {
	dir := t.TempDir()
	syncTag = ""
	t.Cleanup(func() { syncTag = "" })

	writeConfig(t, dir, "prod", syncChildConfig{
		Tag:           "prod",
		BaseBranches:  []string{"main"},
		ChildBranches: []string{"feat-x"},
	})

	cfg, path, err := resolveConfig(reader(""), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tag != "prod" {
		t.Errorf("Tag: got %q, want %q", cfg.Tag, "prod")
	}
	if !strings.HasSuffix(path, "prod.json") {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestResolveConfig_MultipleConfigs_PromptSelection(t *testing.T) {
	dir := t.TempDir()
	syncTag = ""
	t.Cleanup(func() { syncTag = "" })

	writeConfig(t, dir, "staging", syncChildConfig{Tag: "staging"})
	writeConfig(t, dir, "prod", syncChildConfig{Tag: "prod"})

	// Tags are returned in directory order (alphabetical); "prod" is [1], "staging" is [2]
	tags, _ := listConfigTags(dir)
	var selectionIndex string
	for i, tag := range tags {
		if tag == "staging" {
			selectionIndex = string(rune('1' + i))
			break
		}
	}

	cfg, _, err := resolveConfig(reader(selectionIndex+"\n"), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tag != "staging" {
		t.Errorf("Tag: got %q, want %q", cfg.Tag, "staging")
	}
}

func TestResolveConfig_TagFlag_BypassesSelection(t *testing.T) {
	dir := t.TempDir()
	syncTag = "prod"
	t.Cleanup(func() { syncTag = "" })

	writeConfig(t, dir, "staging", syncChildConfig{Tag: "staging"})
	writeConfig(t, dir, "prod", syncChildConfig{Tag: "prod", BaseBranches: []string{"main"}})

	cfg, _, err := resolveConfig(reader(""), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tag != "prod" {
		t.Errorf("Tag: got %q, want %q", cfg.Tag, "prod")
	}
}

func TestResolveConfig_TagFlag_NotFound(t *testing.T) {
	dir := t.TempDir()
	syncTag = "ghost"
	t.Cleanup(func() { syncTag = "" })

	_, _, err := resolveConfig(reader(""), dir)
	if err == nil {
		t.Error("expected error for missing tag, got nil")
	}
}
