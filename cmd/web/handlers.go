package main

import (
	"concurrency_project/data"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
)

func (app *Config) HomePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "home.page.gohtml", nil)
}

func (app *Config) LoginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.page.gohtml", nil)
}

func (app *Config) PostLoginPage(w http.ResponseWriter, r *http.Request) {
	_ = app.Session.RenewToken(r.Context())

	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}

	// get email and password from post
	email := r.Form.Get("email")
	password := r.Form.Get("password")

	user, err := app.Models.User.GetByEmail(email)
	if err != nil {
		app.Session.Put(r.Context(), "error", "No Such Email.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// check password
	validPassword, err := user.PasswordMatches(password)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Invalid Password.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if !validPassword {
		msg := Message{
			To:      email,
			Subject: "Failed Login Attempt",
			Data:    "Someone tried to login to your account with invalid passoword.",
		}

		app.sendEmail(msg)

		app.Session.Put(r.Context(), "error", "Invalid Password.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// log in
	app.Session.Put(r.Context(), "userID", user.ID)
	app.Session.Put(r.Context(), "user", user)

	app.Session.Put(r.Context(), "flash", "Successful Login")

	// redirect user
	http.Redirect(w, r, "/", http.StatusSeeOther)

}

func (app *Config) Logout(w http.ResponseWriter, r *http.Request) {
	// clean up  session
	_ = app.Session.Destroy(r.Context())
	_ = app.Session.RenewToken(r.Context())

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *Config) RegisterPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "register.page.gohtml", nil)
}

func (app *Config) PostRegisterPage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.ErrorLog.Println(err)
	}

	// validate data

	// Create User
	u := data.User{
		Email:     r.Form.Get("email"),
		FirstName: r.Form.Get("first-name"),
		LastName:  r.Form.Get("last-name"),
		Password:  r.Form.Get("password"),
		Active:    0,
		IsAdmin:   0,
	}

	_, err = u.Insert(u)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Failed to create user.")
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}
	// Send an activation Email
	url := fmt.Sprintf("http://localhost/activate?email=%s", u.Email)
	signedURL := GenerateTokenFromString(url)
	app.InfoLog.Println(signedURL)

	msg := Message{
		To:       u.Email,
		Subject:  "Activate Your Account",
		Template: "confirmation-email",
		Data:     template.HTML(signedURL),
	}
	app.sendEmail(msg)

	app.Session.Put(r.Context(), "flash", "Please check your email to activate your account.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	// subscribe the user to an account
}

func (app *Config) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	// validate url
	url := r.RequestURI
	testURL := fmt.Sprintf("http://localhost%s", url)
	okay := VerifyToken(testURL)

	if !okay {
		app.Session.Put(r.Context(), "error", "Invalid Token.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	u, err := app.Models.User.GetByEmail(r.URL.Query().Get("email"))

	if err != nil {
		app.Session.Put(r.Context(), "error", "No such user.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	u.Active = 1
	err = u.Update()
	if err != nil {
		app.Session.Put(r.Context(), "error", "Cannot Update User")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "flash", "Account Activated.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	// Generate an Invoice

	// Send an email with attachments

	// send an Email with the invoice attached
}

func (app *Config) SubcribeToPlan(w http.ResponseWriter, r *http.Request) {
	// get id of plan
	id := r.URL.Query().Get("id")

	planID, _ := strconv.Atoi(id)
	// get plan from db
	plan, err := app.Models.Plan.GetOne(planID)

	if err != nil {
		app.Session.Put(r.Context(), "error", "Cannot Find Plan.")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	// get user from session
	user, ok := app.Session.Get(r.Context(), "user").(data.User)
	if !ok {
		app.Session.Put(r.Context(), "error", "Cannot get user from session.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// generate invoice
	app.Wait.Add(1)

	go func() {
		defer app.Wait.Done()

		invoice, err := app.getInvoice(user, plan)

		if err != nil {
			app.ErrorChan <- err
			return
		}

		// send an email with the invoice attached

		msg := Message{
			To:       user.Email,
			Subject:  "Invoice",
			Data:     invoice,
			Template: "invoice",
		}

		app.sendEmail(msg)
	}()

	// generate manual

	app.Wait.Add(1)
	go func() {
		defer app.Wait.Done()
		pdf := app.generateManual(user, plan)
		err := pdf.OutputFileAndClose(fmt.Sprintf("./tmp/%d_manual.pdf", user.ID))
		if err != nil {
			app.ErrorChan <- err
			return
		}

		msg := Message{
			To:      user.Email,
			Subject: "Manual",
			Data:    "Please find the manual attached.",
			AttachmentMap: map[string]string{
				"manual.pdf": fmt.Sprintf("./tmp/%d_manual.pdf", user.ID),
			},
		}
		app.sendEmail(msg)

		app.ErrorChan <- errors.New("Manual Sent")
	}()

	// subscribe user to plan

	err = app.Models.Plan.SubscribeUserToPlan(user, *plan)

	if err != nil {
		app.Session.Put(r.Context(), "error", "Cannot Subscribe User.")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	u, err := app.Models.User.GetOne(user.ID)
	if err != nil {
		app.Session.Put(r.Context(), "error", "Cannot Get User.")
		http.Redirect(w, r, "/members/plans", http.StatusSeeOther)
		return
	}

	app.Session.Put(r.Context(), "user", u)

	// redirect to home

	app.Session.Put(r.Context(), "flash", "Subscription Successful.")
	http.Redirect(w, r, "/", http.StatusSeeOther)

}

func (app *Config) generateManual(user data.User, plan *data.Plan) *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 13, 10)
	importer := gofpdi.NewImporter()

	time.Sleep(time.Second * 5)

	t := importer.ImportPage(pdf, "./pdf/manual.pdf", 1, "/MediaBox")

	pdf.AddPage()

	importer.UseImportedTemplate(pdf, t, 0, 0, 215.9, 0)

	pdf.SetX(75)
	pdf.SetY(150)

	pdf.SetFont("Arial", "B", 16)
	pdf.MultiCell(0, 4, fmt.Sprintf("Welcome %s %s", user.FirstName, user.LastName), "0", "C", false)

	pdf.Ln(5)

	pdf.MultiCell(0, 4, fmt.Sprintf("%s User Guide", plan.PlanName), "0", "C", false)

	return pdf
}

func (app *Config) getInvoice(user data.User, plan *data.Plan) (string, error) {
	return plan.PlanAmountFormatted, nil
}

func (app *Config) ChooseSubscription(w http.ResponseWriter, r *http.Request) {
	plans, err := app.Models.Plan.GetAll()
	if err != nil {
		app.ErrorLog.Println(err)
		return
	}

	dataMap := make(map[string]any)
	dataMap["plans"] = plans

	app.render(w, r, "plans.page.gohtml", &TemplateData{
		Data: dataMap,
	})
}
