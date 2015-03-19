package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/google/go-github/github"
	"gopkg.in/yaml.v2"
)

func firstFetch(dataDir string, repositories []string,
	reposInsert *sql.Stmt, GitHubClient *github.Client) {

	for _, main := range repositories {
		log.Printf("Doing the startup fetch of %s...", main)

		repo, err := OpenRepository(dataDir, main)
		if err != nil {
			log.Println("[!] Startup fetch failure:", err)
			continue
		}

		err = repo.Fetch(main, true)
		if err != nil {
			log.Println("[!] Startup fetch failure:", err)
			continue
		}
		if _, err := reposInsert.Exec(main, main); err != nil {
			log.Println("[!] Startup fetch failure:", err)
			continue
		}

		forks, err := getForks(main, GitHubClient)
		if err != nil {
			log.Println("[!] Startup fetch failure:", err)
			continue
		}

		for i, fork := range forks {
			log.Printf("[%d / %d] %s", i+1, len(forks), fork)
			if err := repo.Fetch(fork, false); err != nil {
				log.Println("[!] Startup fetch failure:", err)
				continue
			}
			if _, err := reposInsert.Exec(fork, main); err != nil {
				log.Println("[!] Startup fetch failure:", err)
				continue
			}
		}

		repo.Close()
	}
}

func monitorRepoChanges(reposSelect *sql.Stmt, changedRepos chan [2]string,
	GitHubClient *github.Client) {
	firehose := make(chan github.Event, 30)
	go gitHubFirehose(firehose, GitHubClient)

	for e := range firehose {
		if *e.Type == "PushEvent" {
			name := *e.Repo.Name

			var mainName string
			err := reposSelect.QueryRow(name).Scan(&mainName)
			switch {
			case err == sql.ErrNoRows:
				// not a monitored repo
			case err != nil:
				log.Println("[!] Name lookup failure", err)
			default:
				changedRepos <- [...]string{name, mainName}
				if len(changedRepos) > cap(changedRepos)/10*9 {
					log.Println("[!] Queue is filling up:", len(changedRepos))
				}
			}
		}
	}
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

	reposDB, reposInsert, reposSelect, err := OpenReposDb(
		filepath.Join(C.DataDir, "repos.sqlite"))
	if err != nil {
		log.Fatal(err)
	}
	defer reposDB.Close()

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

	changedRepos := make(chan [2]string, C.QueueSize)
	go monitorRepoChanges(reposSelect, changedRepos, GitHubClient)

	go firstFetch(C.DataDir, repositories, reposInsert, GitHubClient)

	for changedRepo := range changedRepos {
		log.Println(changedRepo)
	}
}
