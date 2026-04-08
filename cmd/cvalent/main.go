package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/codevalent/cvalent/internal/build"
	"github.com/codevalent/cvalent/internal/config"
	"github.com/codevalent/cvalent/internal/graph"
	"github.com/codevalent/cvalent/internal/mcp"
	"github.com/codevalent/cvalent/internal/query"
)

var version = "0.1.0-dev"

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
		g, err := openGraphForQuery()
		if err != nil {
			return fmt.Errorf("open graph: %w (run cvalent build first)", err)
		}
		defer g.Close()

		server := mcp.NewServer(g)
		fmt.Fprintln(os.Stderr, "cvalent MCP server started on stdio")
		return server.Serve(os.Stdin, os.Stdout)
	},
}

func init() {
	serveCmd.Flags().BoolVar(&mcpFlag, "mcp", false, "Start MCP server on stdio transport")
}

var depthFlag int

func openGraphForQuery() (*graph.Graph, error) {
	root, _ := os.Getwd()
	graphPath := config.GraphPath(root)
	return graph.Open(graphPath)
}

func init() {
	// Wire all 11 query commands
	callersCmd := &cobra.Command{
		Use: "callers <function>", Short: "Show functions that call the given function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Callers(g, args[0], query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	contractCmd := &cobra.Command{
		Use: "contract <function>", Short: "Show the contract (input/output shape) of a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			info, err := query.Contract(g, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s\n  %s:%d-%d\n  Contract: %s\n  Completeness: %s\n",
				info.QualifiedName, info.File, info.StartLine, info.EndLine, info.Contract, info.Completeness)
			return nil
		},
	}
	impactCmd := &cobra.Command{
		Use: "impact <function>", Short: "Show the blast radius of changing a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Impact(g, args[0], depthFlag, query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Printf("Impact of %s (depth %d):\n%s", args[0], depthFlag, query.FormatInfo(results.Items))
			return nil
		},
	}
	impactCmd.Flags().IntVar(&depthFlag, "depth", 3, "Maximum traversal depth")

	breaksCmd := &cobra.Command{
		Use: "breaks <function>", Short: "Show callers whose data shape doesn't match the contract",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Breaks(g, args[0], query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	entryPointsCmd := &cobra.Command{
		Use: "entry-points", Short: "Show functions with no incoming call edges",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.EntryPoints(g, query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	exportsCmd := &cobra.Command{
		Use: "exports <module>", Short: "Show the public API of a module",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Exports(g, args[0], query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	domainsCmd := &cobra.Command{
		Use: "domains", Short: "List directory-based module groupings",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Domains(g, query.UnlimitedOpts())
			if err != nil {
				return err
			}
			for _, d := range results.Items {
				fmt.Printf("  %-30s %d functions\n", d.Module, d.Functions)
			}
			return nil
		},
	}
	domainCmd := &cobra.Command{
		Use: "domain <name>", Short: "Show functions and edges within a module",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Domain(g, args[0], query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	couplingCmd := &cobra.Command{
		Use: "coupling", Short: "Show cross-module dependency density",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Coupling(g, query.UnlimitedOpts())
			if err != nil {
				return err
			}
			for _, c := range results.Items {
				fmt.Printf("  %s -> %s  (%d edges)\n", c.FromModule, c.ToModule, c.EdgeCount)
			}
			return nil
		},
	}
	untestedCmd := &cobra.Command{
		Use: "untested", Short: "Show application functions with no test coverage",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.Untested(g, query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
		},
	}
	testCoverageCmd := &cobra.Command{
		Use: "test-coverage <function>", Short: "Show which tests exercise a function",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := openGraphForQuery()
			if err != nil {
				return fmt.Errorf("open graph: %w (run cvalent build first)", err)
			}
			defer g.Close()
			results, err := query.TestCoverage(g, args[0], query.UnlimitedOpts())
			if err != nil {
				return err
			}
			fmt.Print(query.FormatInfo(results.Items))
			return nil
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
