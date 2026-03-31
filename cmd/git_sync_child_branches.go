package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type syncChildConfig struct {
	Tag           string   `json:"tag"`
	BaseBranches  []string `json:"base_branches"`
	ChildBranches []string `json:"child_branches"`
}

var syncUpdate bool
var syncAddBranch string
var syncAddCurrentBranch bool
var syncList bool
var syncRemoveBranch string
var syncTag string

var syncChildBranchesCmd = &cobra.Command{
	Use:   "sync-child-branches",
	Short: "Merge base branches into child branches and push",
	Long: `Merges a set of base branches into each child branch, then pushes.

Configs are stored per-directory in ~/.belt/git-sync-child-branches/.
Multiple tagged configs can coexist in the same directory.
On first run you will be prompted for a tag name, base branches, and child branches.
Use --tag to target a specific config, --update to reconfigure.`,
	RunE: runSyncChildBranches,
}

func init() {
	syncChildBranchesCmd.Flags().BoolVar(&syncUpdate, "update", false, "Re-prompt and update stored config")
	syncChildBranchesCmd.Flags().StringVar(&syncAddBranch, "add-branch", "", "Add a branch to the child branches list")
	syncChildBranchesCmd.Flags().BoolVar(&syncAddCurrentBranch, "add-current-branch", false, "Add the current git branch to the child branches list")
	syncChildBranchesCmd.Flags().BoolVar(&syncList, "list", false, "List configured base and child branches")
	syncChildBranchesCmd.Flags().StringVar(&syncRemoveBranch, "remove", "", "Remove a branch from the child branches list")
	syncChildBranchesCmd.Flags().StringVar(&syncTag, "tag", "", "Target a specific config by tag name")
	gitCmd.AddCommand(syncChildBranchesCmd)
}

// syncConfigDir returns the directory that holds all tagged configs for the current working directory.
func syncConfigDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(cwd))
	key := hex.EncodeToString(h[:8])
	return filepath.Join(home, ".belt", "git-sync-child-branches", key), nil
}

func syncConfigPathForTag(dir, tag string) string {
	return filepath.Join(dir, tag+".json")
}

// listConfigTags returns all tag names that have a saved config in dir.
func listConfigTags(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			tags = append(tags, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return tags, nil
}

func loadSyncConfig(path string) (*syncChildConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg syncChildConfig
	return &cfg, json.Unmarshal(data, &cfg)
}

func saveSyncConfig(path string, cfg *syncChildConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func promptLine(r *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := r.ReadString('\n')
	return strings.TrimSpace(line), err
}

func promptCSV(r *bufio.Reader, prompt string) ([]string, error) {
	line, err := promptLine(r, prompt)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, p := range strings.Split(line, ",") {
		if b := strings.TrimSpace(p); b != "" {
			result = append(result, b)
		}
	}
	return result, nil
}

// selectTag presents a numbered list of tags and returns the one the user picks.
func selectTag(r *bufio.Reader, tags []string) (string, error) {
	fmt.Println("Multiple configs found. Select one:")
	for i, t := range tags {
		fmt.Printf("  [%d] %s\n", i+1, t)
	}
	for {
		raw, err := promptLine(r, "Enter number: ")
		if err != nil {
			return "", err
		}
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 1 || n > len(tags) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(tags))
			continue
		}
		return tags[n-1], nil
	}
}

// resolveConfig loads the config to operate on, handling tag selection when needed.
// Returns the config, its file path, and any load error (nil means config was loaded successfully).
func resolveConfig(reader *bufio.Reader, dir string) (*syncChildConfig, string, error) {
	if syncTag != "" {
		path := syncConfigPathForTag(dir, syncTag)
		cfg, err := loadSyncConfig(path)
		return cfg, path, err
	}

	tags, err := listConfigTags(dir)
	if err != nil {
		return nil, "", err
	}

	switch len(tags) {
	case 0:
		return nil, "", os.ErrNotExist
	case 1:
		path := syncConfigPathForTag(dir, tags[0])
		cfg, err := loadSyncConfig(path)
		return cfg, path, err
	default:
		tag, err := selectTag(reader, tags)
		if err != nil {
			return nil, "", err
		}
		path := syncConfigPathForTag(dir, tag)
		cfg, err := loadSyncConfig(path)
		return cfg, path, err
	}
}

func gitRun(args ...string) error {
	c := exec.Command("git", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func gitOutputStr(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func runSyncChildBranches(_ *cobra.Command, _ []string) error {
	dir, err := syncConfigDir()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	cfg, cfgPath, loadErr := resolveConfig(reader, dir)

	if syncList {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run without flags first to set it up")
		}
		fmt.Printf("Tag: %s\n", cfg.Tag)
		fmt.Println("Base branches:")
		for _, b := range cfg.BaseBranches {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Println("Child branches:")
		for _, b := range cfg.ChildBranches {
			fmt.Printf("  - %s\n", b)
		}
		return nil
	}

	if syncRemoveBranch != "" {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run without flags first to set it up")
		}
		filtered := cfg.ChildBranches[:0]
		found := false
		for _, b := range cfg.ChildBranches {
			if b == syncRemoveBranch {
				found = true
				continue
			}
			filtered = append(filtered, b)
		}
		if !found {
			return fmt.Errorf("branch %q not found in child branches list", syncRemoveBranch)
		}
		cfg.ChildBranches = filtered
		if err := saveSyncConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Removed %q from child branches.\n", syncRemoveBranch)
		return nil
	}

	if syncAddCurrentBranch {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run without flags first to set it up")
		}
		current, err := gitOutputStr("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		syncAddBranch = current
	}

	if syncAddBranch != "" {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run without flags first to set it up")
		}
		for _, b := range cfg.ChildBranches {
			if b == syncAddBranch {
				fmt.Printf("Branch %q is already in the child branches list.\n", syncAddBranch)
				return nil
			}
		}
		cfg.ChildBranches = append(cfg.ChildBranches, syncAddBranch)
		if err := saveSyncConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Added %q to child branches.\n", syncAddBranch)
		return nil
	}

	if loadErr != nil || syncUpdate {
		if loadErr != nil {
			fmt.Println("No config found for this directory. Let's set it up.")
		} else {
			fmt.Printf("Updating config %q for this directory.\n", cfg.Tag)
		}

		tag := syncTag
		if tag == "" {
			var err error
			tag, err = promptLine(reader, "Config tag (a short name to identify this config): ")
			if err != nil {
				return err
			}
			if tag == "" {
				return fmt.Errorf("tag cannot be empty")
			}
		}

		baseBranches, err := promptCSV(reader, "Base branches (comma-separated): ")
		if err != nil {
			return err
		}
		childBranches, err := promptCSV(reader, "Child branches (comma-separated): ")
		if err != nil {
			return err
		}

		cfg = &syncChildConfig{
			Tag:           tag,
			BaseBranches:  baseBranches,
			ChildBranches: childBranches,
		}
		cfgPath = syncConfigPathForTag(dir, tag)
		if err := saveSyncConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Config %q saved.\n\n", tag)
	}

	if len(cfg.BaseBranches) == 0 {
		return fmt.Errorf("no base branches configured — run with --update to reconfigure")
	}
	if len(cfg.ChildBranches) == 0 {
		return fmt.Errorf("no child branches configured — run with --update to reconfigure")
	}

	currentBranch, err := gitOutputStr("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	for _, branch := range cfg.ChildBranches {
		fmt.Printf("==> %s\n", branch)
		if err := gitRun("checkout", branch); err != nil {
			return fmt.Errorf("checkout %s: %w", branch, err)
		}
		for _, base := range cfg.BaseBranches {
			fmt.Printf("    merging %s\n", base)
			if err := gitRun("merge", base, "--no-edit"); err != nil {
				return fmt.Errorf("merge %s into %s: %w", base, branch, err)
			}
		}
		if err := gitRun("push", "origin", branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
		fmt.Println("    done")
	}

	if err := gitRun("checkout", currentBranch); err != nil {
		return fmt.Errorf("returning to %s: %w", currentBranch, err)
	}
	fmt.Printf("Back on %s\n", currentBranch)
	return nil
}
