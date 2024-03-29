// db.go

package db
import (
    "database/sql"
    _ "github.com/lib/pq"
)
var DB *sql.DB

var PA string

func InitConnection() error {
    connectionString := "host=localhost port=5432 user=gor password=1 dbname=gor sslmode=disable"
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return err
    }
    err = db.Ping()
    if err != nil {
        return err
    }
    DB = db
    return nil
}
