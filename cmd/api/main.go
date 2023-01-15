package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/tmaxmax/go-sse"
	"html/template"
	"net/http"
	reaktorbirdnest "reaktor-birdnest"
	"reaktor-birdnest/internal/datastore"
	"reaktor-birdnest/internal/models"
	"reaktor-birdnest/internal/models/birdnest"
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

type Violation struct {
	Pilot           models.Pilot
	LastTime        time.Time
	ClosestDistance float64
}

type IBirdnest interface {
	GetReport() (models.Report, error)
	GetDronePilot(droneSerialNumber string) (models.Pilot, error)
}

type application struct {
	sseHandler    *sse.Server
	cfg           config
	tmpl          *template.Template
	homepage      []byte
	homepageMutex sync.RWMutex
	birdnest      IBirdnest
	violations    interface {
		Get(id string) (Violation, bool)
		Upsert(id string, data Violation)
		DeleteOldestWhile(cond func(violation Violation) bool)
		AsSlice() []Violation
		HasChanges() bool
	}
}

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
		violations: datastore.New[Violation](),
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
	Violations []Violation
}

func (app *application) processViolations(violations []Violation) {
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
