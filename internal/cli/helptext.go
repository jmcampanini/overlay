package cli

// Shared help fragments compose command long descriptions so repeated
// contract text cannot drift between commands. Fragments carry no leading or
// trailing newline; compose them with explicit separators.

const configFileHelp = `The configuration file is $XDG_CONFIG_HOME/overlay/config.toml, or
~/.config/overlay/config.toml when XDG_CONFIG_HOME is empty. A relative
XDG_CONFIG_HOME resolves against the current directory. No other location
is searched, and a missing file counts as empty.`

const layerSelectionHelp = `Layers come from the highest-priority input that sets them: --layers LIST
(comma-separated) and --layer NAME, which may repeat and combine with the
--layers values first, override OVERLAY_LAYERS, which overrides layers in
the file. --layers "" clears the list.

env_layers in the file then names environment variables, read in list
order and exactly as written, whose comma-separated values append to that
list rather than replace it, even after --layers "". An unset or empty
variable adds nothing, and blank items in a value are dropped.

When a name repeats across the list and the env_layers additions, its last
position wins, so 'work,laptop,work' becomes 'laptop,work'. Names are
lowercase letters or digits in groups separated by single hyphens, at most
64 characters; a blank entry in layers, OVERLAY_LAYERS, or the flags is an
error. 'base' and 'local' are automatic: base applies before the configured
layers and local after, and neither may be listed, not even through
env_layers.`
