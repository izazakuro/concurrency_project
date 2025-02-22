package main

import (
	"concurrency_project/data"
	"database/sql"
	"encoding/gob"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

const webPort = "80"

func main() {
	// Connect to DB

	db := initDB()

	// Create Sessions

	session := initSession()

	// Create Loggers
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Create Channel

	// Create WaitGroup
	wg := sync.WaitGroup{}

	// Set up app Config
	app := Config{
		Session:       session,
		DB:            db,
		InfoLog:       infoLog,
		ErrorLog:      errorLog,
		Wait:          &wg,
		Models:        data.New(db),
		ErrorChan:     make(chan error),
		ErrorChanDone: make(chan bool),
	}

	// Set up Mail
	app.Mailer = app.createMail()

	go app.listenForMail()

	// listen for signals
	go app.listenForShutdown()

	go app.listenForError()

	// Listen and Serve
	app.serve()

}

func (app *Config) serve() {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", webPort),
		Handler: app.routes(),
	}

	app.InfoLog.Println("Starting Web Server...")

	err := srv.ListenAndServe()
	if err != nil {
		log.Panic()
	}
}

// Connect to DB

func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil {
		log.Panic("Failed to connect DB")
	}
	return conn
}

func connectToDB() *sql.DB {
	counts := 0

	dsn := os.Getenv("DSN")

	for {
		connection, err := openDB(dsn)
		if err != nil {
			log.Println("Postgres is not ready...")
		} else {
			log.Println("Connected!")
			return connection
		}

		if counts > 10 {
			return nil
		}
		log.Printf("Backing off for 1 second")

		time.Sleep(time.Second * 1)

		counts++

		continue
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	err = db.Ping()

	if err != nil {
		return nil, err
	}

	return db, nil
}

// Create Session

func initSession() *scs.SessionManager {

	gob.Register(data.User{})

	// set up session
	session := scs.New()

	session.Store = redisstore.New(initRedis())
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

// Connect to Redis

func initRedis() *redis.Pool {
	redisPool := &redis.Pool{
		MaxIdle: 10,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}

	return redisPool
}

func (app *Config) listenForError() {
	for {
		select {
		case err := <-app.ErrorChan:
			app.ErrorLog.Println(err)
		case <-app.ErrorChanDone:
			return
		}
	}
}

func (app *Config) listenForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.shutdown()
	os.Exit(0)
}

func (app *Config) shutdown() {

	app.InfoLog.Println("run cleanup tasks")

	app.Wait.Wait()

	app.Mailer.DoneChan <- true

	app.ErrorChanDone <- true

	app.InfoLog.Println("closing channels and shutting down app...")

	close(app.Mailer.MailerChan)
	close(app.Mailer.ErrorChan)
	close(app.createMail().DoneChan)
	close(app.ErrorChan)
	close(app.ErrorChanDone)
}

func (app *Config) createMail() Mail {
	errorChan := make(chan error)
	mailerChan := make(chan Message, 100)
	doneChan := make(chan bool)

	m := Mail{
		Domain:      "localhost",
		Host:        "localhost",
		Port:        1025,
		Encryption:  "none",
		FromName:    "Info",
		FromAddress: "Info@mycompany.com",
		Wait:        app.Wait,
		ErrorChan:   errorChan,
		MailerChan:  mailerChan,
		DoneChan:    doneChan,
	}
	return m
}
