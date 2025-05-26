package middleware

import (
	"database/sql"
	"github.com/BurntSushi/toml"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"scheduling/config"
	"time"
)

// db
var db *sql.DB

// ConnectToDB
func ConnectToDB() *sql.DB {

	if db != nil {
		return db
	}

	dsn := config.Mysqldb
	var err error

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Println("Error ConnectToDB:", err)
		return nil
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("Error pinging the database: %v", err)
		return nil
	}

	log.Println("Database connection pool initialized successfully.")
	return db
}

func CloseDB() {
	if db != nil {
		err := db.Close()
		if err != nil {
			log.Println("Error closing the database connection pool:", err)
		} else {
			log.Println("Database connection pool closed.")
		}
	}
}

func UseToml() config.ConfigInfo {
	var c config.ConfigInfo
	var path string = "D:\\goland_workspace\\Arcturus\\scheduling\\config\\conf.toml"
	if _, err := toml.DecodeFile(path, &c); err != nil {
		log.Fatal(err)

	}
	return c
}
