// Command overlay merges layered JSON/TOML configuration files by profile.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/jmcampanini/overlay/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		var code cli.DiffExitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		fmt.Fprintln(os.Stderr, "overlay:", err)
		os.Exit(1)
	}
}
