package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xanzy/go-gitlab"

	"github.com/florianloch/gomodgraph/internal/graph"
	"github.com/florianloch/gomodgraph/internal/mods"
)

// TODO: Annotate module with the latest version, allows indication whether a used version is outdated
// TODO: Consider "Replace" directive in go.mod
// TODO: Add CLI flag to clean cache
// TODO: Add route parameter to request PNG instead of SVG

const (
	glTokenEnvVar    = "GITLAB_API_TOKEN"
	glBaseURLEnvVar  = "GITLAB_BASE_URL"
	downloadLocation = "/tmp/gomodgraph/"
)

type config struct {
	glToken          string
	glBaseURL        string
	homeModule       string
	goRegistryPrefix string
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	cfg := configure()

	glClient, err := gitlab.NewClient(cfg.glToken, gitlab.WithBaseURL(cfg.glBaseURL))
	if err != nil {
		log.Fatal().Msgf("Initializing GitLab client: %v", err)
	}

	if err := os.Mkdir(downloadLocation, 0o700); err != nil {
		if !os.IsExist(err) {
			log.Fatal().Msgf("Directory for downloaded mod files (%q) does not exist and cannot be created: %v",
				downloadLocation,
				err)
		}
	} else {
		log.Info().Msgf("Cache at %q is empty, will scan for projects and download mod files", downloadLocation)

		if err := mods.Download(mods.NewGitLabModFetcher(glClient), downloadLocation); err != nil {
			log.Fatal().Msgf("Could not download mod files: %v", err)
		}
	}

	modFiles, err := mods.ReadModFiles(downloadLocation)
	if err != nil {
		log.Fatal().Msgf("Could not read mod files: %v", err)
	}

	depGraph := graph.BuildDependencyGraph(modFiles)

	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		renderAndReply := func(graph *graph.DependencyGraph) {
			// We buffer the output in order to ensure we do not end up with an error half-way
			buffer := bytes.NewBuffer([]byte{})

			if err := graph.RenderSVG(buffer, cfg.goRegistryPrefix); err != nil {
				log.Error().Msgf("Failed to serve request: %v", err)

				http.Error(w, "Failed to render graph", http.StatusInternalServerError)

				return
			}

			w.Header().Set("Content-Type", "image/svg+xml")

			_, _ = w.Write(buffer.Bytes())
		}

		mod := r.URL.Query().Get("mod")

		if mod == "" {
			log.Info().Msg("Serving overview graph")

			renderAndReply(depGraph)

			return
		}

		if centerModule := depGraph.LookupNode(mod); centerModule != nil {
			log.Info().Msgf("Serving graph for module: %s", mod)

			renderAndReply(depGraph.SubgraphFrom(centerModule))

			return
		}

		http.Error(w, fmt.Sprintf("%q is not a known module.", mod), http.StatusBadRequest)
	})

	// Take a free port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal().Msgf("Could not open listener: %v", err)
	}

	// TODO: Check whether we need to encode the mod when including it in the URL
	log.Info().Msgf("Serving at http://localhost:%d/?mod=%s", listener.Addr().(*net.TCPAddr).Port, url.QueryEscape(cfg.homeModule))

	if err := http.Serve(listener, r); err != nil {
		log.Fatal().Msgf("Serving failed: %v", err)
	}
}

func configure() *config {
	var (
		baseURL    string
		homeModule string
	)

	flag.StringVar(&baseURL, "gitlab-base-url", "", "GitLab's API Base URL")
	flag.StringVar(&homeModule, "mod", "", "Show graph of this module instead of giant overview graph")

	flag.Parse()

	if baseURL == "" {
		baseURL = os.Getenv(glBaseURLEnvVar)

		if baseURL == "" {
			log.Fatal().Msgf("GitLab's API Base URL is required but neither the flag nor the env variable (%q) is set", glBaseURLEnvVar)
		}
	}

	baseURLAsURL, err := url.Parse(baseURL)
	if err != nil {
		log.Fatal().Msgf("GitLab API Base URL seems not to be an invalid URL: %v", err)
	}

	token := os.Getenv(glTokenEnvVar)

	if token == "" {
		log.Fatal().Msgf("GitLab's API token required but the env variable (%q) is set", glTokenEnvVar)
	}

	return &config{
		glToken:          token,
		glBaseURL:        baseURL,
		homeModule:       homeModule,
		goRegistryPrefix: fmt.Sprintf("%s/", baseURLAsURL.Hostname()),
	}
}
