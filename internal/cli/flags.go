package cli

import "github.com/spf13/cobra"

// GlobalFlags holds the persistent flag values shared across subcommands.
type GlobalFlags struct {
	Config   string
	Source   string
	Target   string
	Profiles []string
	Quiet    bool
	Verbose  bool
	Continue bool
}

// Bind attaches g as persistent flags on cmd. Call once on the root command.
func (g *GlobalFlags) Bind(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&g.Config, "config", "", "path to .overlay.toml (default: ./.overlay.toml)")
	f.StringVar(&g.Source, "source", "", "override source directory from config")
	f.StringVar(&g.Target, "target", "", "override target directory from config")
	f.StringSliceVar(&g.Profiles, "profiles", nil, "comma-separated profile list (replaces config/env)")
	f.BoolVarP(&g.Quiet, "quiet", "q", false, "suppress INFO logs (show WARN and above)")
	f.BoolVarP(&g.Verbose, "verbose", "v", false, "enable DEBUG logging")
	f.BoolVar(&g.Continue, "continue", false, "continue past invalid source files")
}

// changed reports whether the user explicitly set the named persistent flag.
func changed(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name)
}
