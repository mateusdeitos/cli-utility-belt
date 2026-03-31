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
	BaseBranches  []string `json:"base_branches"`
	ChildBranches []string `json:"child_branches"`
}

var syncUpdate bool
var syncAddBranch string
var syncAddCurrentBranch bool
var syncList bool
var syncRemoveBranch string

var syncChildBranchesCmd = &cobra.Command{
	Use:   "sync-child-branches",
	Short: "Merge base branches into child branches and push",
	Long: `Merges a set of base branches into each child branch, then pushes.

Config is stored per-directory in ~/.belt/git-sync-child-branches/.
On first run you will be prompted for base and child branches.
Use --update to reconfigure.`,
	RunE: runSyncChildBranches,
}

func init() {
	syncChildBranchesCmd.Flags().BoolVar(&syncUpdate, "update", false, "Re-prompt and update stored config for this directory")
	syncChildBranchesCmd.Flags().StringVar(&syncAddBranch, "add-branch", "", "Add a branch to the child branches list in the stored config")
	syncChildBranchesCmd.Flags().BoolVar(&syncAddCurrentBranch, "add-current-branch", false, "Add the current git branch to the child branches list in the stored config")
	syncChildBranchesCmd.Flags().BoolVar(&syncList, "list", false, "List configured base and child branches for this directory")
	syncChildBranchesCmd.Flags().StringVar(&syncRemoveBranch, "remove", "", "Remove a branch from the child branches list in the stored config")
	gitCmd.AddCommand(syncChildBranchesCmd)
}

func syncConfigPath() (string, error) {
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
	return filepath.Join(home, ".belt", "git-sync-child-branches", key+".json"), nil
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

func promptCSV(r *bufio.Reader, prompt string) ([]string, error) {
	fmt.Print(prompt)
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var result []string
	for _, p := range strings.Split(strings.TrimSpace(line), ",") {
		if b := strings.TrimSpace(p); b != "" {
			result = append(result, b)
		}
	}
	return result, nil
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
	cfgPath, err := syncConfigPath()
	if err != nil {
		return err
	}

	cfg, loadErr := loadSyncConfig(cfgPath)

	if syncList {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run without flags first to set it up")
		}
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
			fmt.Println("Updating config for this directory.")
		}

		reader := bufio.NewReader(os.Stdin)
		baseBranches, err := promptCSV(reader, "Base branches (comma-separated): ")
		if err != nil {
			return err
		}
		childBranches, err := promptCSV(reader, "Child branches (comma-separated): ")
		if err != nil {
			return err
		}

		cfg = &syncChildConfig{
			BaseBranches:  baseBranches,
			ChildBranches: childBranches,
		}
		if err := saveSyncConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Config saved to %s\n\n", cfgPath)
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
