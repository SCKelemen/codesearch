package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
)

func newServeCommand() *clix.Command {
	cmd := clix.NewCommand("serve")
	cmd.Short = "Serve a JSON search API"
	cmd.Usage = "csx serve [--addr :8080] [--index ./index]"

	var addr string
	var indexDir string
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "addr", Short: "a", Usage: "Listen address"},
		Default:     defaultListenAddr,
		Value:       &addr,
	})
	cmd.Flags.StringVar(clix.StringVarOptions{
		FlagOptions: clix.FlagOptions{Name: "index", Short: "i", Usage: "Path to the local index directory"},
		Default:     defaultIndexDir,
		Value:       &indexDir,
	})

	cmd.Run = func(ctx *clix.Context) error {
		engine, err := openEngine(indexDir)
		if err != nil {
			return fmt.Errorf("open index: %w", err)
		}
		defer func() {
			_ = engine.Close()
		}()
		return runServer(ctx.App.Out, engine, addr, indexDir)
	}
	return cmd
}

func runServer(out io.Writer, engine *codesearch.Engine, addr, indexDir string) error {
	ui := newCLIUI(out)
	mux := http.NewServeMux()
	mux.HandleFunc(searchAPIPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		query := r.URL.Query().Get("q")
		if err := requireQuery(query); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		mode, err := normalizeMode(r.URL.Query().Get("mode"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		limit := parseLimit(r.URL.Query().Get("limit"), defaultSearchLimit)
		results, err := engine.Search(r.Context(), query, codesearch.WithLimit(limit), codesearch.WithMode(mode))
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := writeJSON(w, http.StatusOK, buildSearchResponse(query, limit, mode, "remote", results)); err != nil {
			ui.warnf("write response: %v", err)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "csx search service\nGET %s?q=<query>&limit=20&mode=hybrid\n", searchAPIPath)
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ui.section("Serving")
	ui.kv("addr", addr)
	ui.kv("index", indexDir)
	ui.kv("endpoint", searchAPIPath)
	return server.ListenAndServe()
}
