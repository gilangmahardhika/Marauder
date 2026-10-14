package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"orchestrator/backend/internal/runner"
	"orchestrator/backend/internal/runnerapi"
)

// Version is the release of this binary; release.sh sets it with -ldflags.
var Version = "dev"

func runnerCommand(opts Options, data *string) *cobra.Command {
	cmd := &cobra.Command{Use: "runner", Short: "run leased implement jobs for a control plane (RFC-106)"}
	var config string
	cmd.PersistentFlags().StringVar(&config, "config", "", "runner config file (default: <user config dir>/orchestrator/runner.json)")
	configPath := func() (string, error) {
		if config != "" {
			return config, nil
		}
		return runner.DefaultConfigPath()
	}

	var url, token, name, labels string
	var slots int
	register := &cobra.Command{
		Use:   "register",
		Short: "exchange a registration token for a runner secret and write the config",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			url = strings.TrimRight(strings.TrimSpace(url), "/")
			if err := runner.CheckURL(url); err != nil {
				return err
			}
			if token == "" {
				token = opts.getenv("ORCHESTRATOR_RUNNER_TOKEN")
			}
			if token == "" {
				return errors.New("--token is required")
			}
			if name == "" {
				name, _ = os.Hostname()
			}
			dir := *data
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = filepath.Join(home, ".local", "share", "orchestrator-runner")
			}
			if dir, err = filepath.Abs(dir); err != nil {
				return err
			}
			var labelList []string
			for _, label := range strings.Split(labels, ",") {
				if label = strings.TrimSpace(label); label != "" {
					labelList = append(labelList, label)
				}
			}
			client := &runner.Client{URL: url}
			reg, err := client.Register(cmd.Context(), runnerapi.Register{
				Token: token, Name: name, Labels: labelList, Slots: slots, Version: Version, Protocol: runnerapi.Protocol,
			})
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			if err := runner.SaveConfig(path, runner.Config{
				URL: url, RunnerID: reg.RunnerID, Secret: reg.Secret, Name: name, Slots: slots, Labels: labelList, Data: dir,
			}); err != nil {
				return err
			}
			fmt.Fprintf(opts.Stdout, "registered %s as %s\nconfig %s\n", name, reg.RunnerID, path)
			return nil
		},
	}
	register.Flags().StringVar(&url, "url", "", "control plane origin, https://…")
	register.Flags().StringVar(&token, "token", "", "registration token from the Runners page (or ORCHESTRATOR_RUNNER_TOKEN)")
	register.Flags().StringVar(&name, "name", "", "name on the Runners page (default: host name)")
	register.Flags().IntVar(&slots, "slots", 2, "implement jobs at once, 1 to 16")
	register.Flags().StringVar(&labels, "labels", "", "comma-separated labels")
	cmd.AddCommand(register)

	cmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "heartbeat and run leased jobs until stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			cfg, err := runner.LoadConfig(path)
			if err != nil {
				return err
			}
			tools := runner.Tools{
				Git:    lookTool("git"),
				Podman: lookTool("podman", "/usr/bin/podman", "/usr/local/bin/podman"),
				Docker: lookTool("docker", "/usr/bin/docker", "/usr/local/bin/docker"),
			}
			if tools.Git == "" {
				return errors.New("git is not installed")
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			r := &runner.Runner{
				Client: &runner.Client{URL: cfg.URL, Secret: cfg.Secret},
				Name:   cfg.Name, Data: cfg.Data, Slots: cfg.Slots, Version: Version, Tools: tools,
				Logf: func(format string, args ...any) { fmt.Fprintf(opts.Stderr, format+"\n", args...) },
			}
			fmt.Fprintf(opts.Stderr, "runner %s (%s) on %s with %d slots\n", cfg.Name, cfg.RunnerID, cfg.URL, cfg.Slots)
			return r.Run(ctx)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove",
		Short: "delete the runner config; remove the runner on the Runners page to revoke its secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			fmt.Fprintf(opts.Stdout, "removed %s\nremove the runner on the Runners page to revoke its secret\n", path)
			return nil
		},
	})
	return cmd
}
