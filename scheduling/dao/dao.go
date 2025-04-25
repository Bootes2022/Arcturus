package dao

import (
	"control/config"
	"database/sql"
	"time"

	"log"

	"github.com/BurntSushi/toml"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
)

// db
var db *sql.DB

// Redis
var pool *redis.Pool

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

func CreateRedisPool() *redis.Pool {
	pool = &redis.Pool{
		MaxIdle:     10,
		MaxActive:   20,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {

			c, err := redis.Dial("tcp", "localhost:6379")
			if err != nil {
				log.Fatalf("Failed to connect to Redis: %v", err)
				return nil, err
			}
			return c, err
		},
	}
	return pool
}

func GetRedisConn() redis.Conn {
	return pool.Get()
}

func CloseRedisPool() {
	if pool != nil {
		err := pool.Close()
		if err != nil {
			log.Println("Error closing Redis connection pool:", err)
		} else {
			log.Println("Redis connection pool closed.")
		}
	}
}

func UseToml() config.ConfigInfo {
	var c config.ConfigInfo
	var path string = "control/config/conf.toml"
	if _, err := toml.DecodeFile(path, &c); err != nil {
		log.Fatal(err)

	}
	return c
}
