package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FiloSottile/git2go"
	_ "github.com/mattn/go-sqlite3"
)

const (
	Refspec       = "+refs/*:refs/*"
	GitHubUrl     = "git://github.com/%s.git"
	RepoDirPrefix = "github.com"
)

const (
	sqlInit = `
		CREATE TABLE "Heads" (
			"Sha" TEXT,
			"Repository" TEXT,
			"Timestamp" TEXT,
			"Name" TEXT
		);`
	sqlIndex = `
		CREATE INDEX "REPOSITORY" ON "Heads" ("Repository");
		CREATE INDEX "TIMESTAMP" ON "Heads" ("Timestamp");`
	sqlInsert = `INSERT INTO "Heads" VALUES (?, ?, ?, ?)`
)

var RemoteCallbacks = &git.RemoteCallbacks{
	SidebandProgressCallback: func(str string) git.ErrorCode {
		fmt.Fprint(os.Stderr, str)
		os.Stderr.Sync()
		return git.ErrOk
	},
	TransferProgressCallback: func(s git.TransferProgress) git.ErrorCode {
		// pp.Printf("%v %v %v %v %v %v %v %v %v %v %v %v\r",
		// 	"tot", s.TotalObjects,
		// 	"idx", s.IndexedObjects,
		// 	"rcvd", s.ReceivedObjects,
		// 	"loc", s.LocalObjects,
		// 	"delta", s.TotalDeltas,
		// 	"bytes", s.ReceivedBytes,
		// )
		// os.Stderr.Sync()
		return git.ErrOk
	},
}

// Repository is not safe for concurrent use
type Repository struct {
	Path string
	Repo *git.Repository

	Db          *sql.DB
	InsertQuery *sql.Stmt
}

func OpenDb(filename string) (*sql.DB, *sql.Stmt, error) {
	isNew := false
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		isNew = true
	}

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, nil, err
	}

	if isNew {
		_, err = db.Exec(sqlInit)
		if err != nil {
			return nil, nil, err
		}
		_, err = db.Exec(sqlIndex)
		if err != nil {
			return nil, nil, err
		}
	}

	insertQuery, err := db.Prepare(sqlInsert)
	if err != nil {
		return nil, nil, err
	}

	return db, insertQuery, nil
}

func OpenRepository(dataDir, name string) (*Repository, error) {
	r := &Repository{
		Path: filepath.Join(dataDir, RepoDirPrefix, name),
	}

	repo, err := git.OpenRepository(r.Path)
	if e, ok := err.(*git.GitError); ok {
		if e.Code == git.ErrNotFound {
			repo, err = git.InitRepository(r.Path, true)
		}
	}
	if err != nil {
		return nil, err
	}

	db, insertQuery, err := OpenDb(r.Path + ".sqlite")
	if err != nil {
		return nil, err
	}
	r.Db, r.InsertQuery = db, insertQuery

	c, err := repo.Config()
	if err != nil {
		return nil, err
	}
	if err := c.SetString("gc.auto", "0"); err != nil {
		return nil, err
	}

	r.Repo = repo
	return r, err
}

func (r *Repository) Close() error {
	r.Repo.Free()
	return r.Db.Close()
}

func (r *Repository) Fetch(GHRepoName string, isMain bool) error {
	// TimeFormat = "20060102T150405Z"    // ISO 8601 basic format
	// refPrefix = "refs/" + GHRepoName + "/" + timestamp + "/"
	// timestamp := time.Now().UTC().Format(TimeFormat)
	// refspec := fmt.Sprintf("+refs/*:refs/%s/%s/*", GHRepoName, timestamp)

	now := time.Now()

	url := fmt.Sprintf(GitHubUrl, GHRepoName)
	rem, err := r.Repo.CreateAnonymousRemote(url, Refspec)
	if err != nil {
		return err
	}
	if err := rem.SetCallbacks(RemoteCallbacks); err != nil {
		return err
	}

	if isMain {
		if err := rem.Fetch([]string{Refspec}, nil, ""); err != nil {
			return err
		}
	} else {
		if err := rem.ConnectFetch(); err != nil {
			return err
		}
		// TODO: check that obj only ref'd by a tag are downloaded
		if err := rem.Download([]string{Refspec}); err != nil {
			return err
		}
		rem.Disconnect()
	}

	heads, err := rem.Ls()
	if err != nil {
		return err
	}
	for _, head := range heads {
		if !git.ReferenceIsValidName(head.Name) {
			continue
		}

		_, err = r.InsertQuery.Exec(head.Id.String(),
			GHRepoName, now, head.Name)
	}

	rem.Free()
	return nil
}
