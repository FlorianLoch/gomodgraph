package mods

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/xanzy/go-gitlab"
)

const (
	paginationProjectsPerPage = 100
	downloadRoutines          = 10
)

type GitLabModFetcher struct {
	glClient *gitlab.Client
}

func NewGitLabModFetcher(glClient *gitlab.Client) *GitLabModFetcher {
	return &GitLabModFetcher{
		glClient: glClient,
	}
}

func (g *GitLabModFetcher) ProvideModFilesAndVersions(storeModFile StoreModFileFn) error {
	projects, err := g.fetchAllProjects()
	if err != nil {
		return fmt.Errorf("fetching projects from GitLab: %w", err)
	}

	log.Info().Msgf("Going to check %d projects for go.mod files and released versions/tags", len(projects))

	if err := g.downloadModFilesAndLookupVersions(projects, storeModFile); err != nil {
		return fmt.Errorf("fetching module metadata: %w", err)
	}

	return nil
}

func (g *GitLabModFetcher) downloadModFilesAndLookupVersions(projects []*gitlab.Project, storeModFile StoreModFileFn) error {
	var (
		noErrorCnt, noModFile int64
		wg                    sync.WaitGroup
	)

	downloader := func(routineIdx, numRoutines int) {
		for i := routineIdx; i < len(projects); i = i + numRoutines {
			project := projects[i]

			rawFile, resp, err := g.glClient.RepositoryFiles.GetRawFile(project.ID, "go.mod", nil)
			if err != nil {
				if resp.StatusCode == http.StatusNotFound {
					atomic.AddInt64(&noModFile, 1)

					continue
				}

				log.Error().Msgf("Failed to download go.mod for project %q: %v", project.NameWithNamespace, err)

				continue
			}

			version, err := g.latestVersion(project.ID)
			if err != nil {
				log.Error().Msgf("Could not get latest version of project %q: %v", project.Name, err)
			}

			if err := storeModFile(project.NameWithNamespace, version, rawFile); err != nil {
				log.Error().Msgf("Failed to store mod file: %v", err)

				continue
			}

			atomic.AddInt64(&noErrorCnt, 1)
		}

		wg.Done()
	}

	wg.Add(downloadRoutines)

	for i := 0; i < downloadRoutines; i++ {
		go downloader(i, downloadRoutines)
	}

	wg.Wait()

	log.Info().Msgf("%d repositories contain no go.mod file. Downloaded %d files, %d errors occurred.", noModFile, noErrorCnt, int64(len(projects))-noErrorCnt-noModFile)

	return nil
}

func (g *GitLabModFetcher) fetchAllProjects() ([]*gitlab.Project, error) {
	listOptions := gitlab.ListOptions{
		Page:    1, // first page
		PerPage: paginationProjectsPerPage,
	}

	t := true

	var allProjects []*gitlab.Project

	for {
		projects, resp, err := g.glClient.Projects.ListProjects(&gitlab.ListProjectsOptions{
			ListOptions: listOptions,
			Simple:      &t,
		})

		if err != nil {
			return nil, err
		}

		allProjects = append(allProjects, projects...)

		if resp.NextPage == 0 {
			break
		}

		listOptions.Page = resp.NextPage
	}

	return allProjects, nil
}

var (
	orderByUpdated   = "updated"
	sortDescending   = "desc"
	glListTagOptions = &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    1, // first page
			PerPage: 1,
		},
		OrderBy: &orderByUpdated,
		Sort:    &sortDescending,
	}
)

func (g *GitLabModFetcher) latestVersion(projectID int) (string, error) {
	tags, _, err := g.glClient.Tags.ListTags(projectID, glListTagOptions)
	if err != nil {
		return "n.a.", err
	}

	if len(tags) == 0 {
		return "", nil
	}

	return tags[0].Name, nil
}
