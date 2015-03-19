package main

import (
	"database/sql"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const (
	ReposSQLInit = `
        CREATE TABLE "Repos" (
            "Name" TEXT,
            "Main" TEXT
        );
        CREATE INDEX "NAME" ON "Repos" ("Name");`
	ReposSQLInsert = `INSERT INTO "Repos" VALUES (?, ?)`
	ReposSQLSelect = `SELECT "Main" FROM "Repos" WHERE "Name" = ?`

	RefsSQLInit = `
        CREATE TABLE "Heads" (
            "Sha" TEXT,
            "Repository" TEXT,
            "Timestamp" TEXT,
            "Name" TEXT
        );
        CREATE INDEX "REPOSITORY" ON "Heads" ("Repository");
        CREATE INDEX "TIMESTAMP" ON "Heads" ("Timestamp");`
	RefsSQLInsert = `INSERT INTO "Heads" VALUES (?, ?, ?, ?)`
)

func OpenReposDb(filename string) (*sql.DB, *sql.Stmt, *sql.Stmt, error) {
	// NOTE: truncated at every start because it's also the reference of
	// the repositories that have been fetched since the monitor started
	os.Remove(filename)

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, nil, nil, err
	}

	_, err = db.Exec(ReposSQLInit)
	if err != nil {
		return nil, nil, nil, err
	}

	insertQuery, err := db.Prepare(ReposSQLInsert)
	if err != nil {
		return nil, nil, nil, err
	}

	selectQuery, err := db.Prepare(ReposSQLSelect)
	if err != nil {
		return nil, nil, nil, err
	}

	return db, insertQuery, selectQuery, nil
}

func OpenRefsDb(filename string) (*sql.DB, *sql.Stmt, error) {
	isNew := false
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		isNew = true
	}

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, nil, err
	}

	if isNew {
		_, err = db.Exec(RefsSQLInit)
		if err != nil {
			return nil, nil, err
		}
	}

	insertQuery, err := db.Prepare(RefsSQLInsert)
	if err != nil {
		return nil, nil, err
	}

	return db, insertQuery, nil
}
