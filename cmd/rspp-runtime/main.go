package main

import (
	"fmt"
	"os"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/bootstrap"
)

func main() {
	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "rspp-runtime: provider bootstrap failed: %v\n", err)
		os.Exit(1)
	}
	summary, err := bootstrap.Summary(runtimeProviders.Catalog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rspp-runtime: provider summary failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("rspp-runtime: %s\n", summary)
}
