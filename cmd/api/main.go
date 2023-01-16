package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/tmaxmax/go-sse"
	"html/template"
	"net/http"
	reaktorbirdnest "reaktor-birdnest"
	"reaktor-birdnest/internal/interfaces"
	"reaktor-birdnest/internal/models"
	"reaktor-birdnest/internal/models/birdnest"
	"reaktor-birdnest/internal/persistance/datastore"
	"sync"
	"time"
)

type config struct {
	serverPort       int
	noFlyZoneOriginX float64
	noFlyZoneOriginY float64
	noFlyZoneRadius  float64
	sleepDuration    time.Duration
	persistDuration  time.Duration
}

type application struct {
	sseHandler    *sse.Server
	cfg           config
	tmpl          *template.Template
	homepage      []byte
	homepageMutex sync.RWMutex
	birdnest      interfaces.Birdnest
	violations    interfaces.Violations
}

// DeleteOldestWhile(cond func(violation Violation) bool)
func main() {
	var cfg config

	flag.IntVar(&cfg.serverPort, "port", getEnvInt("PORT", 8080), "API server port")
	var (
		sleepDuration   int
		persistDuration int
	)
	flag.IntVar(&sleepDuration, "sleep", 2000, "Timeout between drone position polls (milliseconds)")
	flag.IntVar(&persistDuration, "persist", 10, "Time to persist violating pilots (minutes)")
	flag.Float64Var(&cfg.noFlyZoneRadius, "no-fly-zone-radius", 100, "Radius of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginX, "no-fly-zone-origin-x", 250000, "Origin X coordinate of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginY, "no-fly-zone-origin-y", 250000, "Origin Y coordinate of no-fly zone in meters")

	flag.Parse()
	cfg.sleepDuration = time.Duration(sleepDuration) * time.Millisecond
	cfg.persistDuration = time.Duration(persistDuration) * time.Minute

	tmpl, err := template.ParseFS(reaktorbirdnest.TemplateFS, "ui/html/*")
	if err != nil {
		panic("failed to read templates")
	}

	homeBuf := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(homeBuf, "home", nil)
	if err != nil {
		panic("failed to render initial home template")
	}

	app := &application{
		sseHandler: sse.NewServer(),
		cfg:        cfg,
		tmpl:       tmpl,
		birdnest:   birdnest.Birdnest{},
		violations: datastore.New[models.Violation](cfg.persistDuration),
		homepage:   homeBuf.Bytes(),
	}

	go app.monitor(make(chan bool), app.processViolations)
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.serverPort), app.routes())
}

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", app.sseHandler.ServeHTTP)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		app.homepageMutex.RLock()
		defer app.homepageMutex.RUnlock()
		w.Write(app.homepage)
	})

	return mux
}

type templateData struct {
	Violations []models.Violation
}

func (app *application) processViolations(violations []models.Violation) {
	td := &templateData{
		Violations: violations,
	}

	homeBuf := new(bytes.Buffer)
	err := app.tmpl.ExecuteTemplate(homeBuf, "home", td)
	if err == nil {
		app.homepageMutex.Lock()
		app.homepage = homeBuf.Bytes()
		app.homepageMutex.Unlock()
	}

	pilotBuf := new(bytes.Buffer)
	err = app.tmpl.ExecuteTemplate(pilotBuf, "pilot", td)
	if err == nil {
		e := &sse.Message{}
		e.AppendData(pilotBuf.Bytes())
		app.sseHandler.Publish(e)
	}
}
