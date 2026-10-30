package cli

import (
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/spf13/cobra"
)

func filePatternArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
		return err
	}
	for _, pattern := range args {
		if err := config.ValidatePattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

func selectFiles(cmd *cobra.Command, configPath string, patterns []string) ([]string, error) {
	loaded, err := loadConfiguration(cmd, configPath)
	if err != nil {
		return nil, err
	}
	effective := loaded.effective
	manifests, err := config.LoadManifests(effective.Sources)
	if err != nil {
		return nil, err
	}
	return config.SelectFiles(effective.Sources, manifests, config.LayerOrder(effective.Layers), patterns)
}
