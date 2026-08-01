// Command overlay merges layered JSON/TOML/YAML configuration files by profile.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/jmcampanini/overlay/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var code cmd.ExitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		fmt.Fprintln(os.Stderr, "overlay:", err)
		os.Exit(1)
	}
}
