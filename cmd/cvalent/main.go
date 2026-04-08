// cvalent — local code contract and data flow graph CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/codevalent/cvalent/internal/build"
	"github.com/codevalent/cvalent/internal/config"
	"github.com/codevalent/cvalent/internal/mcp"
	"github.com/codevalent/cvalent/internal/query"
	"github.com/codevalent/cvalent/internal/store"
)

var version = "0.2.0-dev"

var rootCmd = &cobra.Command{
	Use:   "cvalent",
	Short: "CodeValent — local code contract and data flow graph",
	Long: `cvalent parses code with tree-sitter, extracts function contracts,
resolves cross-file relationships, builds a data flow graph, and makes
it queryable via CLI commands and an MCP server.

Code never leaves your machine.`,
	Version: version,
}

var parseCmd = &cobra.Command{
	Use:   "parse [file]",
	Short: "Parse a single file and print extracted functions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stderr, "cvalent parse: not yet implemented\n")
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .cvalent/ directory with auto-detected config",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := os.Getwd()
		cfg, err := config.Init(root)
		if err != nil {
			return err
		}
		fmt.Printf("Initialized .cvalent/ with languages: %v\n", cfg.Languages)
		return nil
	},
}

var buildCmd = &cobra.Command{
	Use:   "build [path]",
	Short: "Build the code graph from source files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := os.Getwd()
		opts := build.Options{Root: root}
		if len(args) > 0 {
			opts.ScopePath = args[0]
		}
		result, err := build.Run(opts)
		if err != nil {
			return err
		}
		fmt.Print(build.FormatSummary(result))
		return nil
	},
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query the code graph",
}

var mcpFlag bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !mcpFlag {
			return fmt.Errorf("use --mcp flag to start MCP server on stdio")
		}
		s, err := openStoreForQuery()
		if err != nil {
			return err
		}
		defer s.Close()

		server := mcp.NewServer(s)
		fmt.Fprintln(os.Stderr, "cvalent MCP server started on stdio")
		return server.Serve(os.Stdin, os.Stdout)
	},
}

func openStoreForQuery() (*store.Store, error) {
	root, _ := os.Getwd()
	if _, err := config.Load(root); errors.Is(err, config.ErrLegacyStorePresent) {
		return nil, fmt.Errorf("legacy graph.db detected — run `cvalent migrate-store` to upgrade")
	}
	storePath := config.StorePath(root)
	if _, err := os.Stat(storePath); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no store at %s — run `cvalent build` first", storePath)
	}
	return store.Open(context.Background(), storePath)
}

var depthFlag int

func init() {
	serveCmd.Flags().BoolVar(&mcpFlag, "mcp", false, "Start MCP server on stdio transport")
}

func init() {
	ctx := context.Background()
	withStore := func(fn func(*store.Store) error) func(*cobra.Command, []string) error {
		return func(*cobra.Command, []string) error {
			s, err := openStoreForQuery()
			if err != nil {
				return err
			}
			defer s.Close()
			return fn(s)
		}
	}

	callersCmd := &cobra.Command{
		Use: "callers <function>", Short: "Show functions that call the given function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Callers(ctx, s, args[0], query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	contractCmd := &cobra.Command{
		Use: "contract <function>", Short: "Show the contract of a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				d, err := query.Contract(ctx, s, args[0])
				if err != nil {
					return err
				}
				fmt.Print(query.FormatDetail(d))
				return nil
			})(cmd, args)
		},
	}
	impactCmd := &cobra.Command{
		Use: "impact <function>", Short: "Show the blast radius of changing a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Impact(ctx, s, args[0], depthFlag, query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	impactCmd.Flags().IntVar(&depthFlag, "depth", 3, "Maximum traversal depth")

	breaksCmd := &cobra.Command{
		Use: "breaks <function>", Short: "Show callers whose data shape doesn't match the contract",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Breaks(ctx, s, args[0], query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	entryPointsCmd := &cobra.Command{
		Use: "entry-points", Short: "Show functions with no incoming call edges",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.EntryPoints(ctx, s, query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	exportsCmd := &cobra.Command{
		Use: "exports <module>", Short: "Show the public API of a module",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Exports(ctx, s, args[0], query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	domainsCmd := &cobra.Command{
		Use: "domains", Short: "List directory-based module groupings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Domains(ctx, s, query.UnlimitedOpts())
				if err != nil {
					return err
				}
				for _, d := range r.Items {
					fmt.Printf("  %-30s %d functions\n", d.Module, d.Functions)
				}
				return nil
			})(cmd, args)
		},
	}
	domainCmd := &cobra.Command{
		Use: "domain <name>", Short: "Show functions within a module",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Domain(ctx, s, args[0], query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	couplingCmd := &cobra.Command{
		Use: "coupling", Short: "Show cross-module dependency density",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Coupling(ctx, s, query.UnlimitedOpts())
				if err != nil {
					return err
				}
				for _, c := range r.Items {
					fmt.Printf("  %s -> %s  (%d edges)\n", c.FromModule, c.ToModule, c.EdgeCount)
				}
				return nil
			})(cmd, args)
		},
	}
	untestedCmd := &cobra.Command{
		Use: "untested", Short: "Show application functions with no test coverage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.Untested(ctx, s, query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}
	testCoverageCmd := &cobra.Command{
		Use: "test-coverage <function>", Short: "Show which tests exercise a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(func(s *store.Store) error {
				r, err := query.TestCoverage(ctx, s, args[0], query.UnlimitedOpts())
				if err != nil {
					return err
				}
				fmt.Print(query.FormatRefs(r.Items))
				return nil
			})(cmd, args)
		},
	}

	queryCmd.AddCommand(callersCmd, contractCmd, impactCmd, breaksCmd,
		entryPointsCmd, exportsCmd, domainsCmd, domainCmd,
		couplingCmd, untestedCmd, testCoverageCmd)

	rootCmd.AddCommand(parseCmd, initCmd, buildCmd, queryCmd, serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
