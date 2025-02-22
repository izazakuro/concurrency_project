package main

import (
	"net/http"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

func (app *Config) routes() http.Handler {
	// create router

	mux := chi.NewRouter()

	// set up middleware
	mux.Use(middleware.Recoverer)
	mux.Use(app.SessionLoad)

	// define applicatio routes
	mux.Get("/", app.HomePage)
	mux.Get("/login", app.LoginPage)
	mux.Post("/login", app.PostLoginPage)
	mux.Get("/logout", app.Logout)
	mux.Get("/register", app.RegisterPage)
	mux.Post("/register", app.PostRegisterPage)
	mux.Get("/activate", app.ActivateAccount)

	/*mux.Get("/test-email", func(w http.ResponseWriter, r *http.Request) {
		m := Mail{
			Domain:      "localhost",
			Host:        "localhost",
			Port:        1025,
			Encryption:  "none",
			FromAddress: "info@mycompany.com",
			FromName:    "info",
			ErrorChan:   make(chan error),
		}

		msg := Message{
			To:      "me@here.com",
			Subject: "Test Email",
			Data:    "This is a test email",
		}

		m.sendMail(msg, make(chan error))
	})*/

	return mux
}
