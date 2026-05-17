package cli

import (
	"github.com/jmcampanini/go-config-loader/pflagloader"
	"github.com/spf13/cobra"

	"github.com/jmcampanini/overlay/internal/config"
)

// GlobalFlags holds the persistent manual flag values shared across subcommands.
type GlobalFlags struct {
	Config  string
	Quiet   bool
	Verbose bool
}

// Bind attaches g as persistent flags on cmd. Call once on the root command.
func (g *GlobalFlags) Bind(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&g.Config, "config", "", "path to .overlay.toml (default: ./.overlay.toml)")
	f.BoolVarP(&g.Quiet, "quiet", "q", false, "suppress INFO logs (show WARN and above)")
	f.BoolVarP(&g.Verbose, "verbose", "v", false, "enable DEBUG logging")
	if err := pflagloader.Register[config.Config](f); err != nil {
		panic(err)
	}
}

// changed reports whether the user explicitly set the named persistent flag.
func changed(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name) || cmd.PersistentFlags().Changed(name)
}
