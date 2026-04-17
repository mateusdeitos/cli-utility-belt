package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var ecsEnvExportOutput string
var ecsEnvExportProfile string

var ecsEnvExportCmd = &cobra.Command{
	Use:   "env-export",
	Short: "Export env vars from an ECS task definition into a .env file",
	Long: `Interactively selects an ECS cluster, service, and container,
then exports its environment variables and resolved SSM/Secrets Manager
references into a .env file.`,
	RunE: runEcsEnvExport,
}

func init() {
	ecsEnvExportCmd.Flags().StringVarP(&ecsEnvExportOutput, "output", "o", "", "Output file path (default: .env.<service-name>)")
	ecsEnvExportCmd.Flags().StringVar(&ecsEnvExportProfile, "profile", "", "AWS profile to use (overrides AWS_PROFILE env var)")
	ecsCmd.AddCommand(ecsEnvExportCmd)
}

var infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#90E0EF"))

func runEcsEnvExport(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	// ── AWS config ──────────────────────────────────────────────────────────
	var cfgOpts []func(*config.LoadOptions) error
	if ecsEnvExportProfile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(ecsEnvExportProfile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	ecsCli := ecs.NewFromConfig(cfg)

	// ── 1. select cluster ───────────────────────────────────────────────────
	fmt.Println(infoStyle.Render("Fetching ECS clusters..."))
	clusters, err := listAllClusters(ctx, ecsCli)
	if err != nil {
		return fmt.Errorf("listing clusters: %w", err)
	}
	if len(clusters) == 0 {
		return fmt.Errorf("no ECS clusters found in this account/region")
	}

	cluster, err := selectOne("Select ECS cluster", clusters)
	if err != nil {
		return err
	}

	// ── 2. select service ───────────────────────────────────────────────────
	fmt.Println(infoStyle.Render("Fetching services in cluster " + cluster + "..."))
	services, err := listAllServices(ctx, ecsCli, cluster)
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}
	if len(services) == 0 {
		return fmt.Errorf("no services found in cluster %q", cluster)
	}

	service, err := selectOne("Select ECS service", services)
	if err != nil {
		return err
	}

	// ── 3. get task definition ──────────────────────────────────────────────
	fmt.Println(infoStyle.Render("Fetching task definition for " + service + "..."))
	svcOut, err := ecsCli.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(cluster),
		Services: []string{service},
	})
	if err != nil {
		return fmt.Errorf("describing service: %w", err)
	}
	if len(svcOut.Services) == 0 {
		return fmt.Errorf("service %q not found", service)
	}
	taskDefArn := aws.ToString(svcOut.Services[0].TaskDefinition)

	tdOut, err := ecsCli.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(taskDefArn),
	})
	if err != nil {
		return fmt.Errorf("describing task definition: %w", err)
	}
	containers := tdOut.TaskDefinition.ContainerDefinitions
	if len(containers) == 0 {
		return fmt.Errorf("task definition has no containers")
	}

	// ── 4. select container ─────────────────────────────────────────────────
	var container ecstypes.ContainerDefinition
	if len(containers) == 1 {
		container = containers[0]
	} else {
		names := make([]string, len(containers))
		for i, c := range containers {
			names[i] = aws.ToString(c.Name)
		}
		chosen, err := selectOne("Select container", names)
		if err != nil {
			return err
		}
		for _, c := range containers {
			if aws.ToString(c.Name) == chosen {
				container = c
				break
			}
		}
	}

	fmt.Println(infoStyle.Render("Container: " + aws.ToString(container.Name)))

	// ── 5. collect env entries ──────────────────────────────────────────────
	type envEntry struct {
		key      string
		value    string
		isSecret bool // needs SSM / Secrets Manager resolution
	}
	var entries []envEntry

	for _, e := range container.Environment {
		entries = append(entries, envEntry{
			key:   aws.ToString(e.Name),
			value: aws.ToString(e.Value),
		})
	}
	for _, s := range container.Secrets {
		entries = append(entries, envEntry{
			key:      aws.ToString(s.Name),
			value:    aws.ToString(s.ValueFrom),
			isSecret: true,
		})
	}

	if len(entries) == 0 {
		fmt.Println("No environment variables defined for this container.")
		return nil
	}

	// ── 6. resolve secrets ──────────────────────────────────────────────────
	ssmCli := ssm.NewFromConfig(cfg)
	smCli := secretsmanager.NewFromConfig(cfg)

	secretCount := 0
	for _, e := range entries {
		if e.isSecret {
			secretCount++
		}
	}
	if secretCount > 0 {
		fmt.Println(infoStyle.Render(fmt.Sprintf("Resolving %d secret reference(s)...", secretCount)))
	}

	type result struct {
		index int
		value string
		err   error
	}
	results := make([]result, 0, secretCount)
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for i, e := range entries {
		if !e.isSecret {
			continue
		}
		i, e := i, e
		g.Go(func() error {
			resolved, resolveErr := resolveSecretRef(gCtx, ssmCli, smCli, e.value)
			mu.Lock()
			results = append(results, result{index: i, value: resolved, err: resolveErr})
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	for _, r := range results {
		if r.err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  ✗ %s: %v", entries[r.index].key, r.err)))
			entries[r.index].value = "__UNRESOLVED__"
		} else {
			entries[r.index].value = r.value
			fmt.Println(successStyle.Render("  ✓ " + entries[r.index].key))
		}
	}

	// ── 7. write .env file ──────────────────────────────────────────────────
	outPath := ecsEnvExportOutput
	if outPath == "" {
		outPath = ".env." + service
	}
	outPath = filepath.Clean(outPath)

	var sb strings.Builder
	fmt.Fprintf(&sb, "# ECS task definition: %s\n", taskDefArn)
	fmt.Fprintf(&sb, "# Cluster: %s  |  Service: %s  |  Container: %s\n\n",
		cluster, service, aws.ToString(container.Name))

	for _, e := range entries {
		escaped := strings.ReplaceAll(e.value, `"`, `\"`)
		fmt.Fprintf(&sb, "%s=\"%s\"\n", e.key, escaped)
	}

	if err := os.WriteFile(outPath, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Done! Env file written to: " + outPath))
	return nil
}

// resolveSecretRef resolves an SSM parameter ARN/name or Secrets Manager ARN.
func resolveSecretRef(ctx context.Context, ssmCli *ssm.Client, smCli *secretsmanager.Client, ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "arn:aws:secretsmanager:"):
		out, err := smCli.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(ref),
		})
		if err != nil {
			return "", err
		}
		return aws.ToString(out.SecretString), nil

	case strings.HasPrefix(ref, "arn:aws:ssm:"):
		// Extract parameter name from ARN: arn:aws:ssm:region:account:parameter/name
		parts := strings.SplitN(ref, ":parameter", 2)
		paramName := ref
		if len(parts) == 2 {
			paramName = parts[1]
		}
		return fetchSSMParam(ctx, ssmCli, paramName)

	default:
		// Plain SSM parameter name or path
		return fetchSSMParam(ctx, ssmCli, ref)
	}
}

func fetchSSMParam(ctx context.Context, ssmCli *ssm.Client, name string) (string, error) {
	out, err := ssmCli.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Parameter.Value), nil
}

// listAllClusters pages through all cluster ARNs and returns short names.
func listAllClusters(ctx context.Context, cli *ecs.Client) ([]string, error) {
	var names []string
	paginator := ecs.NewListClustersPaginator(cli, &ecs.ListClustersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, arn := range page.ClusterArns {
			parts := strings.Split(arn, "/")
			names = append(names, parts[len(parts)-1])
		}
	}
	return names, nil
}

// listAllServices pages through all service ARNs in a cluster and returns short names.
func listAllServices(ctx context.Context, cli *ecs.Client, cluster string) ([]string, error) {
	var names []string
	paginator := ecs.NewListServicesPaginator(cli, &ecs.ListServicesInput{
		Cluster: aws.String(cluster),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, arn := range page.ServiceArns {
			parts := strings.Split(arn, "/")
			names = append(names, parts[len(parts)-1])
		}
	}
	return names, nil
}

// selectOne shows a huh select form and returns the chosen option.
func selectOne(title string, options []string) (string, error) {
	huhOptions := make([]huh.Option[string], len(options))
	for i, o := range options {
		huhOptions[i] = huh.NewOption(o, o)
	}
	var selected string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(huhOptions...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", fmt.Errorf("no selection made")
	}
	return selected, nil
}
