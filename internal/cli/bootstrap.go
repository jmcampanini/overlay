package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmcampanini/go-config-loader/configloader"
	"github.com/jmcampanini/go-config-loader/configreporter"
	"github.com/jmcampanini/overlay/internal/config"
	"github.com/jmcampanini/overlay/internal/gitrepo"
	"github.com/spf13/cobra"
)

type bootstrapRequest struct {
	configPath  string
	layers      string
	sourcePaths []string
	sourceURLs  []string
}

func bootstrapCommand() *cobra.Command {
	var req bootstrapRequest

	cmd := &cobra.Command{
		Use:   "bootstrap --source-url URL --source-path PATH [--layers LIST]",
		Short: "Write the configuration file and clone its sources",
		Long: `Write the configuration file and clone the sources it lists. Nothing is
rendered.

--source-url URL and --source-path PATH repeat and pair by position; a
count mismatch is an error, and at least one pair is required. --layers
LIST is the comma-separated layer list to write; omit it for an empty
list. Names follow the grammar in 'overlay config --help'. bootstrap does
not read OVERLAY_LAYERS.

` + configFileHelp + `
--config PATH writes PATH instead. bootstrap never overwrites: when the
file already exists, nothing happens and the command fails.

Each source whose path is absent is created by running 'git clone' from
PATH, with git's own output on stderr. An existing path is left alone when
'git remote get-url origin' there equals the source url exactly; any other
existing path is an error. Sources are cloned before the file is written,
so a failed clone leaves no file and bootstrap can simply be run again.
Writing the file creates its missing parent directories with mode 755.

git runs with its own prompts disabled, so a missing credential fails
rather than asks; ssh may still ask to confirm a host key or unlock a
key, so set up SSH or token authentication before running bootstrap. On
success stdout gets one line per source, 'cloned NAME into PATH' or
'exists NAME at PATH', then 'wrote FILE'.

` + contractHelp,
		Example: `  overlay bootstrap \
    --source-url git@github.com:jmcampanini/dotfiles-base.git \
    --source-path ~/code/github.com/jmcampanini/dotfiles-base \
    --source-url git@github.com:jmcampanini/dotfiles-xyz.git \
    --source-path ~/code/github.com/jmcampanini/dotfiles-xyz \
    --layers abc,xyz`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBootstrap(cmd.Context(), req, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringArrayVar(&req.sourceURLs, "source-url", nil, "git URL of a source; repeat per source")
	cmd.Flags().StringArrayVar(&req.sourcePaths, "source-path", nil, "checkout path for the matching --source-url")
	cmd.Flags().StringVar(&req.layers, "layers", "", "comma-separated layers to write")
	cmd.Flags().StringVar(&req.configPath, "config", "", "file to write instead of the default location")
	return cmd
}

func runBootstrap(ctx context.Context, req bootstrapRequest, out, progress io.Writer) error {
	if len(req.sourceURLs) != len(req.sourcePaths) {
		return fmt.Errorf("--source-url was given %d times and --source-path %d times; they pair by position", len(req.sourceURLs), len(req.sourcePaths))
	}
	if len(req.sourceURLs) == 0 {
		return errors.New("at least one --source-url and --source-path pair is required")
	}

	cfg := config.Config{Layers: splitLayers(req.layers)}
	for i, url := range req.sourceURLs {
		cfg.Sources = append(cfg.Sources, config.Source{Path: req.sourcePaths[i], URL: url})
	}
	effective, err := config.Resolve(cfg)
	if err != nil {
		return err
	}

	configPath := req.configPath
	if configPath == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	if err := requireAbsent(configPath); err != nil {
		return err
	}

	for _, source := range effective.Sources {
		cloned, err := ensureCheckout(ctx, source, progress)
		if err != nil {
			return err
		}
		line := "exists %s at %s\n"
		if cloned {
			line = "cloned %s into %s\n"
		}
		if _, err := fmt.Fprintf(out, line, source.Name, source.Path); err != nil {
			return err
		}
	}

	if err := writeConfigFile(configPath, cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "wrote %s\n", configPath)
	return err
}

// splitLayers parses the --layers value like OVERLAY_LAYERS: an empty value
// is an empty list, and blank items are kept so validation rejects them.
func splitLayers(csv string) []string {
	if csv == "" {
		return []string{}
	}

	items := strings.Split(csv, ",")
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}

func requireAbsent(path string) error {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return fmt.Errorf("config file %q already exists; bootstrap does not overwrite it", path)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("check config file %q: %w", path, err)
	}
}

// ensureCheckout clones source when its path is absent and reports whether it
// did. An existing path must already be a checkout of the source url.
func ensureCheckout(ctx context.Context, source config.ResolvedSource, progress io.Writer) (bool, error) {
	_, err := os.Lstat(source.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := gitrepo.Clone(ctx, source.URL, source.Path, progress); err != nil {
			return false, err
		}
		return true, nil
	case err != nil:
		return false, fmt.Errorf("check path %q: %w", source.Path, err)
	}

	origin, err := gitrepo.OriginURL(ctx, source.Path)
	if err != nil {
		return false, fmt.Errorf("path %q exists but is not a checkout of %q: %w", source.Path, source.URL, err)
	}
	if origin != source.URL {
		return false, fmt.Errorf("path %q is a checkout of %q, not %q", source.Path, origin, source.URL)
	}
	return false, nil
}

// writeConfigFile creates path exclusively so a file that appeared since the
// earlier check is still never overwritten.
func writeConfigFile(path string, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}

	reporter := configreporter.New(cfg, configloader.LoadReport{})
	if err := reporter.WriteTOML(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("write config file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write config file %q: %w", path, err)
	}
	return nil
}
