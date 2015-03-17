package main

import (
	"log"
	"strings"
	"time"

	"github.com/google/go-github/github"
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

func getUserRepos(owner string, client *github.Client) ([]string, error) {
	// Should this also get the mains of the forks?

	opt := &github.RepositoryListOptions{
		Type:        "owner",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []github.Repository
	for {
		repos, resp, err := client.Repositories.List(owner, opt)
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
		log.Printf("Found %d repos, continuing...", len(allRepos))
	}

	var result []string
	for _, f := range allRepos {
		if !*f.Fork {
			result = append(result, *f.FullName)
		}
	}
	log.Printf("Found %d repos owned by %s", len(result), owner)
	return result, nil
}

func gitHubFirehose(firehose chan github.Event,
	GitHubClient *github.Client) {

	var (
		lastRoundResults = make(map[string]struct{})
		lastRoundTime    time.Time
	)

	for {
		// Make sure that a second passed since last round
		time.Sleep(lastRoundTime.Add(time.Second).Sub(time.Now()))
		lastRoundTime = time.Now()

		events, _, err := GitHubClient.Activity.ListEvents(nil)
		if err != nil {
			log.Println("Firehose error:", err)
			continue
		}

		thisRoundResults := make(map[string]struct{})
		newResultsCount := 0

		for _, e := range events {
			thisRoundResults[*e.ID] = struct{}{}

			if _, ok := lastRoundResults[*e.ID]; !ok {
				newResultsCount += 1
				firehose <- e
			}
		}

		if newResultsCount > 25 && len(lastRoundResults) > 0 {
			log.Println("[!] Firehose getting behind:", newResultsCount)
		}

		lastRoundResults = thisRoundResults
	}
}

func monitorRepoChanges(repositories []string, changedRepos chan string,
	GitHubClient *github.Client) {

	monitoredReposCount := 0
	monitoredRepos := make(map[string]struct{})
	for _, mainRepo := range repositories {
		log.Printf("[-] %s", mainRepo)

		monitoredRepos[mainRepo] = struct{}{}
		monitoredReposCount += 1

		forks, err := getForks(mainRepo, GitHubClient)
		if err != nil {
			log.Fatal(err)
		}

		for _, fork := range forks {
			monitoredRepos[fork] = struct{}{}
			monitoredReposCount += 1
		}
	}

	log.Printf("[+] Monitoring %d repositories...", monitoredReposCount)

	firehose := make(chan github.Event, 30)
	go gitHubFirehose(firehose, GitHubClient)
	for e := range firehose {
		if *e.Type == "PushEvent" {
			name := *e.Repo.Name
			if _, ok := monitoredRepos[name]; ok {
				changedRepos <- name
			}
		}
	}
}
