package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/intake"
)

type Options struct {
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
	Open   func(databaseURL string) error
	Getenv func(string) string
	Getwd  func() (string, error)
}

func Execute(opts Options) int {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	var repo, data, addr, sha, project string
	root := &cobra.Command{
		Use:           "orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&repo, "repo", "", "target repository")
	root.PersistentFlags().StringVar(&data, "data", "", "data directory")
	root.PersistentFlags().StringVar(&addr, "addr", "127.0.0.1:8787", "listen address")
	root.PersistentFlags().StringVar(&project, "project", "", "project id")
	users := &cobra.Command{Use: "users"}
	users.AddCommand(&cobra.Command{
		Use: "bootstrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				_, err := app.Bootstrap(ctx, db, opts.getenv("ORCHESTRATOR_ADMIN_EMAIL"), opts.getenv("ORCHESTRATOR_ADMIN_PASSWORD"))
				if err != nil {
					return err
				}
				fmt.Fprintln(opts.Stdout, "admin created")
				return nil
			})
		},
	})
	root.AddCommand(users)
	root.AddCommand(&cobra.Command{
		Use: "serve",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(opts, repo, data, addr)
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "ingest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				result, err := app.Ingest(ctx, db, paths.Repo, paths.Data)
				if err != nil {
					return err
				}
				fmt.Fprintf(opts.Stdout, "requirements %d\ndecisions %d\ndraftPending %t\n", result.Requirements, result.Decisions, result.DraftPending)
				return nil
			})
		},
	})
	root.AddCommand(&cobra.Command{
		Use:  "accept",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				return app.Accept(ctx, db, paths.Data, args)
			})
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				n, err := app.CardCount(ctx, db)
				if project != "" {
					err = db.QueryRowContext(ctx, `SELECT count(*) FROM cards WHERE project_id = $1`, project).Scan(&n)
				}
				if err != nil {
					return err
				}
				if n == 0 {
					fmt.Fprint(opts.Stdout, "board is empty\nnext: orchestrator ingest\n")
				}
				return nil
			})
		},
	})
	approve := &cobra.Command{
		Use:  "approve CARD",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sha == "" {
				return errors.New("approve requires --sha")
			}
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				cfg, err := intake.LoadConfig(paths.Data)
				if err != nil {
					return err
				}
				_, err = app.Approve(ctx, db, args[0], sha, app.GateConfig{Operator: cfg.Operator, BotLogin: "bot", Approvers: []string{cfg.Operator}})
				return err
			})
		},
	}
	approve.Flags().StringVar(&sha, "sha", "", "commit SHA")
	root.AddCommand(approve)
	root.AddCommand(&cobra.Command{
		Use:  "request-changes CARD",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				_, err := app.RequestChanges(ctx, db, args[0], app.GateConfig{Operator: "ada", BotLogin: "bot", Approvers: []string{"ada"}})
				return err
			})
		},
	})
	root.AddCommand(&cobra.Command{
		Use:  "reject CARD",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				_, err := app.Reject(ctx, db, args[0], app.GateConfig{Operator: "ada", BotLogin: "bot", Approvers: []string{"ada"}})
				return err
			})
		},
	})
	var reset bool
	unblock := &cobra.Command{
		Use:  "unblock CARD",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				_, err := app.Unblock(ctx, db, args[0], reset)
				return err
			})
		},
	}
	unblock.Flags().BoolVar(&reset, "reset-attempts", false, "zero the attempt counter")
	root.AddCommand(unblock)
	root.AddCommand(&cobra.Command{
		Use: "run-next",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBoard(opts, repo, data, func(ctx context.Context, db *sql.DB, paths resolvedPaths) error {
				var n int
				if err := db.QueryRowContext(ctx, `SELECT count(*) FROM cards WHERE status = 'ready'`).Scan(&n); err != nil {
					return err
				}
				if n == 0 {
					fmt.Fprint(opts.Stdout, "no card is ready\n")
				}
				return nil
			})
		},
	})
	root.SetOut(opts.Stderr)
	root.SetErr(opts.Stderr)
	if opts.Args != nil {
		root.SetArgs(opts.Args)
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(opts.Stderr, err)
		return 1
	}
	return 0
}

func (o Options) getenv(key string) string {
	if o.Getenv != nil {
		return o.Getenv(key)
	}
	return os.Getenv(key)
}

func (o Options) getwd() (string, error) {
	if o.Getwd != nil {
		return o.Getwd()
	}
	return os.Getwd()
}
