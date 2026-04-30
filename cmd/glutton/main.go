package main

import (
	"fmt"

	"github.com/cyrus/glutton/internal/version"
)

func main() {
	fmt.Printf("glutton %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
}
