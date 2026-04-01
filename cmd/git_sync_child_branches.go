package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type syncChildConfig struct {
	Name          string   `json:"name"`
	BaseBranches  []string `json:"base_branches"`
	ChildBranches []string `json:"child_branches"`
}

// syncConfigStore maps: directory path -> config name -> config.
type syncConfigStore map[string]map[string]syncChildConfig

var syncUpdate bool
var syncNew bool
var syncDel string
var syncAddBranch string
var syncAddCurrentBranch bool
var syncList bool
var syncRemoveBranch string
var syncName string
var syncRun bool
var syncVerbose bool

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C853"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87"))
)

var syncChildBranchesCmd = &cobra.Command{
	Use:   "sync-child-branches",
	Short: "Merge base branches into child branches and push",
	Long: `Merges a set of base branches into each child branch, then pushes.

Configs are stored in ~/.belt/git-sync-child-branches.json, keyed by directory path and config name.
Multiple named configs can coexist in the same directory.
On first run you will be prompted for a name, base branches, and child branches.
Use --name to target a specific config, --update to reconfigure.`,
	RunE: runSyncChildBranches,
}

func init() {
	syncChildBranchesCmd.Flags().BoolVar(&syncRun, "run", false, "Run the merge+push cycle")
	syncChildBranchesCmd.Flags().BoolVar(&syncVerbose, "verbose", false, "Show full git command output")
	syncChildBranchesCmd.Flags().BoolVar(&syncNew, "new", false, "Create a new config for this directory")
	syncChildBranchesCmd.Flags().StringVar(&syncDel, "del", "", "Delete a config by name")
	syncChildBranchesCmd.Flags().BoolVar(&syncUpdate, "update", false, "Re-prompt and update stored config")
	syncChildBranchesCmd.Flags().StringVar(&syncAddBranch, "add-branch", "", "Add a branch to the child branches list")
	syncChildBranchesCmd.Flags().BoolVar(&syncAddCurrentBranch, "add-current-branch", false, "Add the current git branch to the child branches list")
	syncChildBranchesCmd.Flags().BoolVar(&syncList, "list", false, "List configured base and child branches")
	syncChildBranchesCmd.Flags().StringVar(&syncRemoveBranch, "remove", "", "Remove a branch from the child branches list")
	syncChildBranchesCmd.Flags().StringVar(&syncName, "name", "", "Target a specific config by name")
	gitCmd.AddCommand(syncChildBranchesCmd)
}

func syncStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".belt", "git-sync-child-branches.json"), nil
}

func loadStore(path string) (syncConfigStore, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(syncConfigStore), nil
	}
	if err != nil {
		return nil, err
	}
	var store syncConfigStore
	return store, json.Unmarshal(data, &store)
}

func saveStore(path string, store syncConfigStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func listNames(store syncConfigStore, cwd string) []string {
	configs, ok := store[cwd]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func setConfig(store syncConfigStore, cwd string, cfg syncChildConfig) {
	if store[cwd] == nil {
		store[cwd] = make(map[string]syncChildConfig)
	}
	store[cwd][cfg.Name] = cfg
}

func getLocalBranches() ([]string, error) {
	out, err := exec.Command("git", "branch", "--format=%(refname:short)").Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if b = strings.TrimSpace(b); b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

func selectBranches(title, description string, branches, preSelected []string) ([]string, error) {
	var selected []string
	options := make([]huh.Option[string], len(branches))
	for i, b := range branches {
		isPreSelected := false
		for _, ps := range preSelected {
			if ps == b {
				isPreSelected = true
				break
			}
		}
		options[i] = huh.NewOption(b, b).Selected(isPreSelected)
	}
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description(description).
				Options(options...).
				Validate(func(s []string) error {
					if len(s) == 0 {
						return fmt.Errorf("select at least one branch")
					}
					return nil
				}).
				Value(&selected),
		),
	).Run()
	return selected, err
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

func selectName(r *bufio.Reader, names []string) (string, error) {
	fmt.Println("Multiple configs found. Select one:")
	for i, n := range names {
		fmt.Printf("  [%d] %s\n", i+1, n)
	}
	for {
		raw, err := promptLine(r, "Enter number: ")
		if err != nil {
			return "", err
		}
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 1 || n > len(names) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(names))
			continue
		}
		return names[n-1], nil
	}
}

// resolveConfig returns the config to operate on and its name.
// Returns os.ErrNotExist if no config is found.
func resolveConfig(reader *bufio.Reader, store syncConfigStore, cwd string) (*syncChildConfig, string, error) {
	if syncName != "" {
		configs, ok := store[cwd]
		if !ok {
			return nil, "", os.ErrNotExist
		}
		cfg, ok := configs[syncName]
		if !ok {
			return nil, "", os.ErrNotExist
		}
		return &cfg, syncName, nil
	}

	names := listNames(store, cwd)

	switch len(names) {
	case 0:
		return nil, "", os.ErrNotExist
	case 1:
		cfg := store[cwd][names[0]]
		return &cfg, names[0], nil
	default:
		name, err := selectName(reader, names)
		if err != nil {
			return nil, "", err
		}
		cfg := store[cwd][name]
		return &cfg, name, nil
	}
}

func printSuccess(msg string) {
	fmt.Println(successStyle.Render("  ✓ " + msg))
}

func printGitError(msg string, gitOut string) {
	fmt.Println(errorStyle.Render("  ✗ " + msg))
	if gitOut != "" {
		fmt.Println(errorStyle.Render("    " + strings.ReplaceAll(strings.TrimSpace(gitOut), "\n", "\n    ")))
	}
}

func gitRun(args ...string) error {
	c := exec.Command("git", args...)
	if syncVerbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutputStr(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// executeSyncForConfig runs the merge+push cycle for cfg without touching the current branch.
func executeSyncForConfig(cfg *syncChildConfig) error {
	if len(cfg.BaseBranches) == 0 {
		return fmt.Errorf("no base branches configured — run with --update to reconfigure")
	}
	if len(cfg.ChildBranches) == 0 {
		return fmt.Errorf("no child branches configured — run with --update to reconfigure")
	}

	for _, branch := range cfg.ChildBranches {
		fmt.Printf("==> %s\n", branch)
		if err := gitRun("checkout", branch); err != nil {
			printGitError(fmt.Sprintf("checkout %s", branch), err.Error())
			return fmt.Errorf("checkout %s: %w", branch, err)
		}
		for _, base := range cfg.BaseBranches {
			if err := gitRun("merge", base, "--no-edit"); err != nil {
				printGitError(fmt.Sprintf("merge %s into %s", base, branch), err.Error())
				return fmt.Errorf("merge %s into %s: %w", base, branch, err)
			}
			if !syncVerbose {
				printSuccess(fmt.Sprintf("merged %s", base))
			}
		}
		if err := gitRun("push", "origin", branch); err != nil {
			printGitError(fmt.Sprintf("push %s", branch), err.Error())
			return fmt.Errorf("push %s: %w", branch, err)
		}
		if !syncVerbose {
			printSuccess("pushed")
		}
	}
	return nil
}

func runSyncChildBranches(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	storePath, err := syncStorePath()
	if err != nil {
		return err
	}

	store, err := loadStore(storePath)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	cfg, _, loadErr := resolveConfig(reader, store, cwd)

	// Default behaviour (no flags): show config and hint.
	if !syncList && !syncRun && !syncNew && !syncUpdate && syncDel == "" && syncRemoveBranch == "" && syncAddBranch == "" && !syncAddCurrentBranch {
		if loadErr != nil {
			fmt.Println("No config found for this directory. Run with --new to set it up.")
			return nil
		}
		fmt.Printf("Name: %s\n", cfg.Name)
		fmt.Println("Base branches:")
		for _, b := range cfg.BaseBranches {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Println("Child branches:")
		for _, b := range cfg.ChildBranches {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Println("\nRun with --run to execute the sync.")
		return nil
	}

	if syncList {
		if loadErr != nil {
			return fmt.Errorf("no config found for this directory — run with --new to set it up")
		}
		fmt.Printf("Name: %s\n", cfg.Name)
		fmt.Println("Base branches:")
		for _, b := range cfg.BaseBranches {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Println("Child branches:")
		for _, b := range cfg.ChildBranches {
			fmt.Printf("  - %s\n", b)
		}
		fmt.Printf("\nStore: %s\n", storePath)
		return nil
	}

	if syncDel != "" {
		configs, ok := store[cwd]
		if !ok {
			return fmt.Errorf("config %q not found", syncDel)
		}
		if _, ok := configs[syncDel]; !ok {
			return fmt.Errorf("config %q not found", syncDel)
		}
		delete(store[cwd], syncDel)
		if len(store[cwd]) == 0 {
			delete(store, cwd)
		}
		if err := saveStore(storePath, store); err != nil {
			return fmt.Errorf("saving store: %w", err)
		}
		fmt.Printf("Config %q deleted.\n", syncDel)
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
		setConfig(store, cwd, *cfg)
		if err := saveStore(storePath, store); err != nil {
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
		setConfig(store, cwd, *cfg)
		if err := saveStore(storePath, store); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Added %q to child branches.\n", syncAddBranch)
		return nil
	}

	if !syncRun && !syncNew && !syncUpdate {
		return nil
	}

	if syncNew || syncUpdate {
		if syncUpdate && loadErr != nil {
			return fmt.Errorf("no config found for this directory — run with --new to create one")
		}
		if syncUpdate {
			fmt.Printf("Updating config %q.\n", cfg.Name)
		}

		name := syncName
		if syncNew && name == "" {
			var err error
			name, err = promptLine(reader, "Config name (a short name to identify this config): ")
			if err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("name cannot be empty")
			}
		} else if syncUpdate {
			name = cfg.Name
		}

		localBranches, err := getLocalBranches()
		if err != nil {
			return fmt.Errorf("listing git branches: %w", err)
		}

		var preBase, preChild []string
		if syncUpdate && cfg != nil {
			preBase = cfg.BaseBranches
			preChild = cfg.ChildBranches
		}

		baseBranches, err := selectBranches("Base branches", "These branches will be merged into each child branch.", localBranches, preBase)
		if err != nil {
			return err
		}
		childBranches, err := selectBranches("Child branches", "Each of these branches will receive merges from all base branches.", localBranches, preChild)
		if err != nil {
			return err
		}

		newCfg := syncChildConfig{
			Name:          name,
			BaseBranches:  baseBranches,
			ChildBranches: childBranches,
		}
		setConfig(store, cwd, newCfg)
		if err := saveStore(storePath, store); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		cfg = &newCfg
		fmt.Printf("Config %q saved.\n\n", name)
	}

	currentBranch, err := gitOutputStr("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	ran := map[string]bool{}
	for {
		fmt.Printf("\nRunning config: %s\n", cfg.Name)
		if err := executeSyncForConfig(cfg); err != nil {
			gitRun("checkout", currentBranch)
			return err
		}
		ran[cfg.Name] = true

		// Build list of configs not yet run in this chain.
		var available []string
		for _, n := range listNames(store, cwd) {
			if !ran[n] {
				available = append(available, n)
			}
		}
		if len(available) == 0 {
			break
		}

		options := make([]huh.Option[string], len(available)+1)
		for i, n := range available {
			options[i] = huh.NewOption(n, n)
		}
		options[len(available)] = huh.NewOption("Done", "")

		var next string
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Run another config?").
					Options(options...).
					Value(&next),
			),
		).Run(); err != nil || next == "" {
			break
		}

		nextCfg := store[cwd][next]
		cfg = &nextCfg
	}

	if err := gitRun("checkout", currentBranch); err != nil {
		return fmt.Errorf("returning to %s: %w", currentBranch, err)
	}
	fmt.Printf("Back on %s\n", currentBranch)
	return nil
}
