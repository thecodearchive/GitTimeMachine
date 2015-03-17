package main

import (
	"log"
	"strings"

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
