package main

import (
	"io/ioutil"
	"log"

	"github.com/google/go-github/github"
	"gopkg.in/yaml.v2"
)

func firstFetch(dataDir, name string, GitHubClient *github.Client) error {
	log.Printf("Working on %s...", name)

	repo, err := OpenRepository(dataDir, name)
	if err != nil {
		return err
	}

	err = repo.Fetch(name, true)
	if err != nil {
		return err
	}

	forks, err := getForks(name, GitHubClient)
	if err != nil {
		return err
	}

	for i, fork := range forks {
		log.Printf("[%d / %d] %s", i+1, len(forks), fork)
		if err := repo.Fetch(fork, false); err != nil {
			return err
		}
	}

	return repo.Close()
}

type Config struct {
	Repositories []map[string]string `yaml:"Repositories"`
	DataDir      string              `yaml:"DataDir"`

	UserAgent    string `yaml:"UserAgent"`
	GitHubID     string `yaml:"GitHubID"`
	GitHubSecret string `yaml:"GitHubSecret"`

	QueueSize int `yaml:"QueueSize"`
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

	var repositories []string
	for _, e := range C.Repositories {
		for entryType, entry := range e {
			switch entryType {
			case "repo":
				repositories = append(repositories, entry)
			case "owner":
				result, err := getUserRepos(entry, GitHubClient)
				if err != nil {
					log.Fatal(err)
				}
				repositories = append(repositories, result...)
			default:
				log.Fatal("unknown Repositories type")
			}
		}
	}

	changedRepos := make(chan string, C.QueueSize)
	go monitorRepoChanges(repositories, changedRepos, GitHubClient)

	for _, repo := range repositories {
		err = firstFetch(C.DataDir, repo, GitHubClient)
		if err != nil {
			log.Fatal(err)
		}
	}

	for changedRepo := range changedRepos {
		log.Println(changedRepo)
	}
}
