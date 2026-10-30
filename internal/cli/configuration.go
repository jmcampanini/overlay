package cli

import (
	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

// loadedConfiguration is a command's configuration as loaded and as
// resolved. The raw configuration and its report serve the commands that
// print them beside the effective configuration.
type loadedConfiguration struct {
	config    config.Config
	effective config.Effective
	report    configloader.LoadReport
}

// registerConfigFlag adds --config, bound to path, to the command's flags.
// loadConfiguration reads the flag's presence from the same flag set.
func registerConfigFlag(cmd *cobra.Command, path *string) {
	cmd.Flags().StringVar(path, "config", "", "configuration file to read instead of the default")
}

// loadConfiguration loads the configuration the command's flags and the
// process environment select, then resolves it. Both steps read one
// snapshot of the environment, so OVERLAY_LAYERS and the env_layers
// variables come from the same process state.
func loadConfiguration(cmd *cobra.Command, configPath string) (loadedConfiguration, error) {
	env := configloader.OSEnv()
	cfg, report, err := config.Load(config.LoadOptions{
		Env:     env,
		Flags:   cmd.Flags(),
		Path:    configPath,
		PathSet: cmd.Flags().Changed("config"),
	})
	if err != nil {
		return loadedConfiguration{}, err
	}
	effective, err := config.Resolve(cfg, env)
	if err != nil {
		return loadedConfiguration{}, err
	}
	return loadedConfiguration{config: cfg, effective: effective, report: report}, nil
}
