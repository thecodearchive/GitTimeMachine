package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/libgit2/git2go"
)

const (
	TimeFormat = "20060102T150405Z" // ISO 8601 basic format
	Refspec    = "+refs/*:refs/%s/%s/*"
	GitHubUrl  = "https://github.com/%s.git"
)

func getOrCreateRepo(dataDir, name string) (*git.Repository, error) {
	log.Printf("Working on %s...", name)

	r, err := git.OpenRepository(filepath.Join(dataDir, name))
	if err, ok := err.(*git.GitError); ok {
		if err.Code == git.ErrNotFound {
			log.Printf("Creating %s...", name)
			return git.InitRepository(filepath.Join(dataDir, name), true)
		}
	}
	return r, err
}

func fetch(r *git.Repository, GHRepoName string) error {
	var (
		timestamp = time.Now().UTC().Format(TimeFormat)
		refspec   = fmt.Sprintf(Refspec, GHRepoName, timestamp)
		url       = fmt.Sprintf(GitHubUrl, GHRepoName)
	)

	// Don't ask me why, but the refspec needs to be on both calls for the tags to get fetched
	rem, err := r.CreateAnonymousRemote(url, refspec)
	if err != nil {
		return err
	}

	err = rem.Fetch([]string{refspec}, nil, "")
	if err != nil {
		return err
	}

	log.Printf("Fetched %s", GHRepoName)
	rem.Free()
	return nil
}

func getForks(name string, client *github.Client) ([]string, error) {
	parts := strings.Split(name, "/")
	owner, repo := parts[0], parts[1]

	opt := &github.RepositoryListForksOptions{
		ListOptions: github.ListOptions{PerPage: 1000},
	}

	var allForks []github.Repository
	for {
		repos, resp, err := client.Repositories.ListForks(owner, repo, opt)
		if err != nil {
			return nil, err
		}
		allForks = append(allForks, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	var result []string
	for _, f := range allForks {
		result = append(result, *f.FullName)
	}
	log.Printf("Found %d forks", len(result))
	return result, nil
}

func main() {
	var (
		REPO     = "FiloSottile/Heartbleed"
		DATA_DIR = "./continuum"
		PREFIX   = "github.com/"

		USER_AGENT    = "FiloSottile Git Time Machine"
		GITHUB_ID     = "eabd9463ca136768a4d4"
		GITHUB_SECRET = ""
	)

	repo, err := getOrCreateRepo(DATA_DIR, PREFIX+REPO)
	if err != nil {
		log.Fatal(err)
	}

	err = fetch(repo, REPO)
	if err != nil {
		log.Fatal(err)
	}

	t := &github.UnauthenticatedRateLimitedTransport{
		ClientID:     GITHUB_ID,
		ClientSecret: GITHUB_SECRET,
	}
	GitHubClient := github.NewClient(t.Client())
	GitHubClient.UserAgent = USER_AGENT

	forks, err := getForks(REPO, GitHubClient)
	if err != nil {
		log.Fatal(err)
	}

	for _, fork := range forks {
		err = fetch(repo, fork)
		if err != nil {
			log.Fatal(err)
		}
	}
}
