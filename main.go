package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xanzy/go-gitlab"

	"github.com/florianloch/gomodgraph/internal/graph"
	"github.com/florianloch/gomodgraph/internal/mods"
)

// TODO: Consider "Replace" directive in go.mod

const (
	glTokenEnvVar   = "GITLAB_API_TOKEN"
	glBaseURLEnvVar = "GITLAB_BASE_URL"
	tmpDir          = "/tmp/gomodgraph/"
	goModDir        = tmpDir + "go_mod_files" // has to be below tmpDir
)

type config struct {
	glToken          string
	glBaseURL        string
	homeModule       string
	goRegistryPrefix string
	cleanup          bool
	listenAddr       string
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	cfg := configure()

	if cfg.cleanup {
		if err := os.RemoveAll(tmpDir); err != nil && !os.IsNotExist(err) {
			log.Error().Msgf("Failed to remove the tmp dir (%q): %v", tmpDir, err)
		}

		log.Info().Msg("Emptied cache successfully")
	}

	glClient, err := gitlab.NewClient(cfg.glToken, gitlab.WithBaseURL(cfg.glBaseURL))
	if err != nil {
		log.Fatal().Msgf("Initializing GitLab client: %v", err)
	}

	var cacheFilled bool

	if info, err := os.Stat(goModDir); err == nil && info.IsDir() {
		cacheFilled = true // we simply assume the cache is filled in case the cache directory for the mod files exists
	}

	if err := os.MkdirAll(goModDir, 0o700); err != nil {
		log.Fatal().Msgf("Directory for downloaded mod files (%q) cannot be accesses and could not be created: %v",
			goModDir,
			err)
	}

	if !cacheFilled {
		log.Info().Msgf("Cache at %q is empty, will scan for projects and download mod files", goModDir)

		if err := mods.Download(mods.NewGitLabModFetcher(glClient), goModDir); err != nil {
			log.Fatal().Msgf("Could not download mod files: %v", err)
		}
	}

	modFiles, err := mods.ReadModFiles(goModDir)
	if err != nil {
		log.Fatal().Msgf("Could not read mod files: %v", err)
	}

	depGraph := graph.BuildDependencyGraph(modFiles)

	mux := http.NewServeMux()
	mux.Handle("/", NewGraphRenderService(depGraph, cfg.goRegistryPrefix))

	// We do this extra work because if port is set to 0 we want to choose a free port automatically
	listener, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		log.Fatal().Msgf("Could not open listener: %v", err)
	}

	log.Info().Msgf("Serving at http://%s/?mod=%s", listener.Addr().(*net.TCPAddr), url.QueryEscape(cfg.homeModule))

	if err := http.Serve(listener, mux); err != nil {
		log.Fatal().Msgf("Serving failed: %v", err)
	}
}

func configure() *config {
	var (
		baseURL    string
		homeModule string
		cleanup    bool
		listenAddr string
	)

	flag.StringVar(&baseURL, "gitlab-base-url", "", "GitLab's API Base URL")
	flag.StringVar(&homeModule, "mod", "", "Show graph of this module instead of giant overview graph")
	flag.BoolVar(&cleanup, "cleanup", false, "Clean up the cache directory, enforcing all information to be refetched")
	flag.StringVar(&listenAddr, "listen-addr", "localhost:0", "Listen on the given interface and port, set port to 0 to have a free one chosen automatically")

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
		cleanup:          cleanup,
		listenAddr:       listenAddr,
	}
}
