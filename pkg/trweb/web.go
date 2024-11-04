package trweb

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"github.com/tinyrange/tinyrange/pkg/database"
	"github.com/tinyrange/tinyrange/pkg/htm"
	"github.com/tinyrange/tinyrange/pkg/htm/bootstrap"
	"github.com/tinyrange/tinyrange/pkg/htm/html"
	"github.com/tinyrange/tinyrange/pkg/login"
)

type WebApplication struct {
	mux           *http.ServeMux
	db            *database.PackageDatabase
	webSshAddress string
	runningCmd    *exec.Cmd
}

func (app *WebApplication) pageLayout(body ...htm.Fragment) htm.Fragment {
	return html.Html(
		htm.Attr("lang", "en"),
		html.Head(
			html.MetaCharset("UTF-8"),
			html.Title("TinyRange"),
			html.MetaViewport("width=device-width, initial-scale=1"),
			bootstrap.CSSSrc,
			bootstrap.JavaScriptSrc,
			bootstrap.ColorPickerSrc,
			html.Style(`iframe {
				width: 100%;
				height: 500px;
			}`),
		),
		html.Body(
			bootstrap.Navbar(
				bootstrap.NavbarBrand("/", html.Text("TinyRange")),
			),
			html.Div(bootstrap.Container, htm.Group(body)),
		),
	)
}

func (app *WebApplication) serveFragment(w http.ResponseWriter, r *http.Request, fragment htm.Fragment) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	htm.Render(r.Context(), w, fragment)
}

func (app *WebApplication) serveIndex(w http.ResponseWriter, r *http.Request) {
	if app.runningCmd != nil {
		http.Redirect(w, r, "/run", http.StatusFound)
		return
	}

	app.serveFragment(w, r, app.pageLayout(
		html.Form(
			html.FormTarget("POST", "/start"),
			bootstrap.SubmitButton("Start", bootstrap.ButtonColorPrimary),
		),
	))
}

func (app *WebApplication) serveRun(w http.ResponseWriter, r *http.Request) {
	if app.runningCmd == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	app.serveFragment(w, r, app.pageLayout(
		html.Form(
			html.FormTarget("POST", "/stop"),
			bootstrap.SubmitButton("Stop", bootstrap.ButtonColorDanger),
		),
		htm.NewHtmlFragment("iframe", htm.Attr("src", "http://"+app.webSshAddress)),
	))
}

func (app *WebApplication) runTemplate(filename string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	app.runningCmd = exec.Command(exe, "run-vm", filename)

	if err := app.runningCmd.Start(); err != nil {
		return err
	}

	return nil
}

func (app *WebApplication) getConfig(r *http.Request) (login.Config, error) {
	config := login.Config{
		Version:     login.CURRENT_CONFIG_VERSION,
		Builder:     "alpine@3.20",
		CpuCores:    1,
		MemorySize:  1024,
		StorageSize: 1024,
		WebSSH:      fmt.Sprintf("%s,minimal", app.webSshAddress),
	}

	return config, nil
}

func (app *WebApplication) handleStart(w http.ResponseWriter, r *http.Request) {
	config, err := app.getConfig(r)
	if err != nil {
		slog.Error("Failed to get config", "error", err)
		http.Error(w, "Failed to get config", http.StatusInternalServerError)
		return
	}

	templateFilename, err := config.MakeTemplate(app.db)
	if err != nil {
		slog.Error("Failed to get template filename", "error", err)
		http.Error(w, "Failed to get template filename", http.StatusInternalServerError)
		return
	}

	slog.Info("running template", "filename", templateFilename)

	if err := app.runTemplate(templateFilename); err != nil {
		slog.Error("Failed to run template", "error", err)
		http.Error(w, "Failed to run template", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/run", http.StatusFound)
}

func (app *WebApplication) handleStop(w http.ResponseWriter, r *http.Request) {
	if app.runningCmd != nil {
		if err := app.runningCmd.Process.Kill(); err != nil {
			slog.Error("Failed to kill process", "error", err)
			http.Error(w, "Failed to kill process", http.StatusInternalServerError)
			return
		}
		app.runningCmd = nil
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (app *WebApplication) Run(listen string) error {
	app.mux.HandleFunc("GET /", app.serveIndex)
	app.mux.HandleFunc("GET /run", app.serveRun)
	app.mux.HandleFunc("POST /start", app.handleStart)
	app.mux.HandleFunc("POST /stop", app.handleStop)

	slog.Info("Listening", "listen", "http://"+listen)

	return http.ListenAndServe(listen, app.mux)
}

func New(db *database.PackageDatabase) *WebApplication {
	return &WebApplication{
		db:            db,
		mux:           http.NewServeMux(),
		webSshAddress: "127.0.0.1:5124",
	}
}
