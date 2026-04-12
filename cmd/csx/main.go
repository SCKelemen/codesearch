package main

import (
	"context"
	"fmt"
	"os"

	"github.com/SCKelemen/clix"
	clixhelp "github.com/SCKelemen/clix/ext/help"
	clixversion "github.com/SCKelemen/clix/ext/version"
)

var version = "dev"

func main() {
	app := newApp()
	if err := app.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(app.Err, err)
		os.Exit(1)
	}
}

func newApp() *clix.App {
	app := clix.NewApp("csx")
	app.Description = "CodeSearch eXplorer for local and remote code indexes"

	root := clix.NewCommand("csx")
	root.Short = "Index, search, serve, and inspect code search indexes"
	root.Long = "csx builds local code indexes, searches them locally or remotely, serves a JSON search API, and reports index health."
	root.Run = func(ctx *clix.Context) error {
		return clix.HelpRenderer{App: ctx.App, Command: ctx.Command}.Render(ctx.App.Out)
	}
	root.Children = []*clix.Command{
		newIndexCommand(),
		newSearchCommand(),
		newServeCommand(),
		newStatusCommand(),
	}

	app.Root = root
	app.AddExtension(clixhelp.Extension{})
	app.AddExtension(clixversion.Extension{Version: version})
	return app
}
