package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type Options struct {
	Args   []string
	Stderr io.Writer
	Open   func(databaseURL string) error
	Getenv func(string) string
	Getwd  func() (string, error)
}

func Execute(opts Options) int {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	var repo, data, addr string
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
