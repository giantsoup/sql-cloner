package main

import (
	"fmt"
	"os"

	"github.com/taylor/dbgold/internal/app"
)

func main() {
	screen := "dashboard"
	if len(os.Args) > 1 {
		screen = os.Args[1]
	}

	rendered, err := app.RenderDocsScreen(screen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render %s: %v\n", screen, err)
		os.Exit(1)
	}

	fmt.Print(rendered)
}
