package main

import (
	"io/ioutil"
	"log"
	"strings"

	"github.com/google/go-github/github"
	"gopkg.in/yaml.v2"
)

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
	log.Printf("Working on %s...", name)

	repo, err := OpenRepository(dataDir, name)
	if err != nil {
		return err
	}

	err = repo.Fetch(name)
	if err != nil {
		return err
	}

	forks, err := getForks(name, GitHubClient)
	if err != nil {
		return err
	}

	for i, fork := range forks {
		log.Printf("[%d / %d] %s", i+1, len(forks), fork)
		if err := repo.Fetch(fork); err != nil {
			return err
		}
	}

	return repo.Close()
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
