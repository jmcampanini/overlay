package config

import "github.com/jmcampanini/go-config-loader/configloader"

// NewFileLoader returns a GoConfigLoader file loader for path. When required is
// true, path must exist; otherwise a missing file leaves defaults unchanged.
func NewFileLoader(path string, required bool) (configloader.ConfigLoader[Config], error) {
	if required {
		return configloader.NewRequiredFileLoader[Config](path)
	}
	return configloader.NewMergeAllFilesLoader[Config](configloader.File(path))
}

// Load reads an optional raw .overlay.toml file. The returned bool reports
// whether the file existed; a missing file yields Default() and no error.
func Load(path string) (Config, bool, configloader.LoadReport, error) {
	loader, err := configloader.NewMergeAllFilesLoader[Config](configloader.File(path))
	if err != nil {
		return Config{}, false, configloader.LoadReport{}, err
	}
	cfg, report, err := configloader.Load(Default(), loader)
	if err != nil {
		return Config{}, false, configloader.LoadReport{}, err
	}
	return cfg, len(report.LoadedFiles) > 0, report, nil
}

// LoadRequired reads a required raw .overlay.toml file.
func LoadRequired(path string) (Config, configloader.LoadReport, error) {
	loader, err := configloader.NewRequiredFileLoader[Config](path)
	if err != nil {
		return Config{}, configloader.LoadReport{}, err
	}
	return configloader.Load(Default(), loader)
}
