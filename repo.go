package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FiloSottile/git2go"
	_ "github.com/mattn/go-sqlite3"
)

const (
	Refspec       = "*:*" // "+refs/*:refs/%s/%s/*"
	GitHubUrl     = "https://github.com/%s.git"
	RepoDirPrefix = "github.com"
	RefsName      = "refs/uniq/%s"
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
	Path    string
	Repo    *git.Repository
	RefsMap map[git.Oid]struct{}

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
		Path:    filepath.Join(dataDir, RepoDirPrefix, name),
		RefsMap: make(map[git.Oid]struct{}),
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

	iter, err := repo.NewReferenceIterator()
	if err != nil {
		return nil, err
	}
	for {
		ref, err := iter.Next()
		if e, ok := err.(*git.GitError); ok {
			if e.Code == git.ErrIterOver {
				break
			}
		}
		if err != nil {
			return nil, err
		}

		r.RefsMap[*ref.Target()] = struct{}{}
	}

	r.Repo = repo
	return r, err
}

func (r *Repository) Close() error {
	r.Repo.Free()
	return r.Db.Close()
}

func (r *Repository) Fetch(GHRepoName string) error {
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

	// if err := rem.Fetch([]string{Refspec}, nil, ""); err != nil {
	// 	return err
	// }

	if err := rem.ConnectFetch(); err != nil {
		return err
	}
	// TODO: check that obj only ref'd by a tag are downloaded
	if err := rem.Download([]string{Refspec}); err != nil {
		return err
	}
	rem.Disconnect()

	heads, err := rem.Ls()
	if err != nil {
		return err
	}
	for _, head := range heads {
		if !git.ReferenceIsValidName(head.Name) {
			continue
		}

		name := head.Name
		if strings.HasPrefix(name, "refs/") {
			name = name[5:]
		}
		// name = refPrefix + name

		_, err = r.InsertQuery.Exec(head.Id.String(),
			GHRepoName, now, name)

		if _, ok := r.RefsMap[*head.Id]; !ok {
			refName := fmt.Sprintf(RefsName, head.Id.String())
			if _, err := r.Repo.CreateReference(refName, head.Id,
				true, nil, ""); err != nil {
				return err
			}

			r.RefsMap[*head.Id] = struct{}{}

			fmt.Fprintf(os.Stderr, "New ref: [%s] %s\n",
				head.Id.String()[:7], name)
		}
	}

	rem.Free()
	return nil
}
