package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gioui.org/app"

	"github.com/chromafish/check/internal/journal"
	"github.com/chromafish/check/internal/repo"
	"github.com/chromafish/check/internal/state"
	"github.com/chromafish/check/internal/ui"
	"github.com/chromafish/check/internal/vcs"
)

// Set at link time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	revset := flag.String("r", "", "revisions to list: a jj revset, or arguments for git log")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: check [-r revset] [path]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("check", version)
		return
	}

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	journal.Start()

	var (
		open  vcs.Repo
		store *state.Store
	)
	if r, err := repo.Open(context.Background(), dir); err == nil {
		open, store = r, state.New()
	} else if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "check:", err)
	}

	a := ui.New(open, dir, store, *revset)
	go func() {
		if err := a.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}
