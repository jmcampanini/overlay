package cli

// Shared help fragments compose command long descriptions so repeated
// contract text cannot drift between commands. Fragments carry no leading or
// trailing newline; compose them with explicit separators.

const configFileHelp = `The configuration file is $XDG_CONFIG_HOME/overlay/config.toml, or
~/.config/overlay/config.toml when XDG_CONFIG_HOME is empty. A relative
XDG_CONFIG_HOME resolves against the current directory. No other location
is searched, and a missing file counts as empty.`

const layerSelectionHelp = `Layers come from the highest layer that sets them: --layers LIST
(comma-separated) and --layer NAME, which may repeat and combine with the
--layers values first, override OVERLAY_LAYERS, which overrides layers in
the file. --layers "" clears the list. When a name repeats, its last
position wins, so 'work,laptop,work' becomes 'laptop,work'. Names are
lowercase letters or digits in groups separated by single hyphens, at most
64 characters; blank entries are errors. 'base' and 'local' are automatic:
base applies before the configured layers and local after, and neither may
be listed.`
