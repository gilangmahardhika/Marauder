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
	var repo, data, addr, sha string
	root := &cobra.Command{
		Use:           "orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&repo, "repo", "", "target repository")
	root.PersistentFlags().StringVar(&data, "data", "", "data directory")
	root.PersistentFlags().StringVar(&addr, "addr", "127.0.0.1:8787", "listen address")
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
			return errors.New("approve is not available")
		},
	}
	approve.Flags().StringVar(&sha, "sha", "", "commit SHA")
	root.AddCommand(approve)
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
