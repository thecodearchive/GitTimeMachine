package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/libgit2/git2go"
	"gopkg.in/yaml.v2"
)

const (
	TimeFormat    = "20060102T150405Z" // ISO 8601 basic format
	Refspec       = "+refs/*:refs/%s/%s/*"
	GitHubUrl     = "https://github.com/%s.git"
	RepoDirPrefix = "github.com/"
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

	log.Printf("Fetching %s...", GHRepoName)
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
		ListOptions: github.ListOptions{PerPage: 100},
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
		log.Printf("Found %d forks, continuing...", len(allForks))
	}

	var result []string
	for _, f := range allForks {
		result = append(result, *f.FullName)
	}
	log.Printf("Found %d forks", len(result))
	return result, nil
}

func firstFetch(dataDir, name string, GitHubClient *github.Client) error {
	repo, err := getOrCreateRepo(dataDir, RepoDirPrefix+name)
	if err != nil {
		return err
	}

	err = fetch(repo, name)
	if err != nil {
		return err
	}

	forks, err := getForks(name, GitHubClient)
	if err != nil {
		return err
	}

	for _, fork := range forks {
		err = fetch(repo, fork)
		if err != nil {
			return err
		}
	}

	return nil
}

type Config struct {
	Repositories []string `yaml:"Repositories"`
	DataDir      string   `yaml:"DataDir"`

	UserAgent    string `yaml:"UserAgent"`
	GitHubID     string `yaml:"GitHubID"`
	GitHubSecret string `yaml:"GitHubSecret"`
}

func main() {
	configText, err := ioutil.ReadFile("config.yml")
	if err != nil {
		log.Fatal(err)
	}

	var C Config
	err = yaml.Unmarshal(configText, &C)
	if err != nil {
		log.Fatal(err)
	}

	t := &github.UnauthenticatedRateLimitedTransport{
		ClientID:     C.GitHubID,
		ClientSecret: C.GitHubSecret,
	}
	GitHubClient := github.NewClient(t.Client())
	GitHubClient.UserAgent = C.UserAgent

	for _, repo := range C.Repositories {
		err = firstFetch(C.DataDir, repo, GitHubClient)
		if err != nil {
			log.Fatal(err)
		}
	}
}
