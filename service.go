package main

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/goccy/go-graphviz"
	"github.com/rs/zerolog/log"

	"github.com/florianloch/gomodgraph/internal/graph"
)

type GraphRenderService struct {
	rootGraph        *graph.DependencyGraph
	goRegistryPrefix string
}

func NewGraphRenderService(rootGraph *graph.DependencyGraph, goRegistryPrefix string) *GraphRenderService {
	return &GraphRenderService{
		rootGraph:        rootGraph,
		goRegistryPrefix: goRegistryPrefix,
	}
}

func (g *GraphRenderService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)

		return
	}

	mod := r.URL.Query().Get("mod")

	asPNG := r.URL.Query().Has("png")

	if mod == "" {
		log.Info().Msg("Serving overview graph")

		g.renderAndReply(w, g.rootGraph, asPNG)

		return
	}

	if centerModule := g.rootGraph.LookupNode(mod); centerModule != nil {
		log.Info().Msgf("Serving graph for module: %s", mod)

		g.renderAndReply(w, g.rootGraph.SubgraphFrom(centerModule), asPNG)

		return
	}

	http.Error(w, fmt.Sprintf("%q is not a known module.", mod), http.StatusBadRequest)
}

func (g *GraphRenderService) renderAndReply(w http.ResponseWriter, graph *graph.DependencyGraph, asPNG bool) {
	// We buffer the output in order to ensure we do not end up with an error half-way
	buffer := bytes.NewBuffer([]byte{})

	format := graphviz.SVG

	if asPNG {
		format = graphviz.PNG
	}

	if err := graph.Render(buffer, g.goRegistryPrefix, format); err != nil {
		log.Error().Msgf("Failed to serve request: %v", err)

		http.Error(w, "Failed to render graph", http.StatusInternalServerError)

		return
	}

	if asPNG {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	_, _ = w.Write(buffer.Bytes())
}
