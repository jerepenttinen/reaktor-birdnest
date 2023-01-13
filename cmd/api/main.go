package main

import (
	"bytes"
	"container/list"
	"flag"
	"fmt"
	"github.com/tmaxmax/go-sse"
	"html/template"
	"math"
	"net/http"
	"os"
	"reaktor-birdnest/internal/data"
	"strconv"
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
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
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

	tmpl, err := template.ParseGlob("./ui/html/*")
	if err != nil {
		panic("failed to read templates")
	}

	app := &application{
		sseHandler: sse.NewServer(),
		cfg:        cfg,
		tmpl:       tmpl,
	}

	go app.monitor()
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

type Violation struct {
	Pilot             data.Pilot
	LastTime          time.Time
	ClosestDistance   float64
	droneSerialNumber string
}

type templateData struct {
	Violations []Violation
}

func (app *application) monitor() {
	drones := make(map[string]*list.Element)
	violationQueue := list.New()
	violationMutex := sync.RWMutex{}

	ticker := time.NewTicker(app.cfg.sleepDuration)
	for {
		report, err := data.GetReport()
		if err != nil {
			fmt.Println(err)
			<-ticker.C
			continue
		}

		wg := sync.WaitGroup{}
		for _, drone := range report.Capture.Drone {
			// Capture variable for goroutine
			drone := drone

			wg.Add(1)
			go func() {
				defer wg.Done()
				distance := math.Hypot(app.cfg.noFlyZoneOriginX-drone.PositionX, app.cfg.noFlyZoneOriginY-drone.PositionY)
				// Convert millimeters to meters
				distance /= 1000
				if distance > app.cfg.noFlyZoneRadius {
					return
				}

				// Check if violation entry exists already
				violationMutex.RLock()
				element, ok := drones[drone.SerialNumber]
				violationMutex.RUnlock()
				if ok {
					violation := element.Value.(Violation)
					if distance < violation.ClosestDistance {
						violation.ClosestDistance = distance
					}

					violation.LastTime = report.Capture.SnapshotTimestamp

					violationMutex.Lock()
					element.Value = violation
					violationQueue.MoveToFront(element)
					violationMutex.Unlock()
				} else {
					pilot, err := data.GetDronePilot(drone.SerialNumber)
					if err != nil {
						fmt.Println(err)
						return
					}

					violationMutex.Lock()
					element := violationQueue.PushFront(Violation{
						Pilot:             pilot,
						LastTime:          report.Capture.SnapshotTimestamp,
						ClosestDistance:   distance,
						droneSerialNumber: drone.SerialNumber,
					})
					drones[drone.SerialNumber] = element
					violationMutex.Unlock()
				}
			}()
		}

		wg.Wait()

		// Delete old violation entries
		for back := violationQueue.Back(); back != nil; back = violationQueue.Back() {
			violation := back.Value.(Violation)
			if report.Capture.SnapshotTimestamp.Sub(violation.LastTime) > app.cfg.persistDuration {
				delete(drones, violation.droneSerialNumber)
				violationQueue.Remove(back)
			} else {
				// Queue is sorted by LastTime
				break
			}
		}

		// Transform violationQueue to slice, so it can be used in html/template
		violations := make([]Violation, 0, violationQueue.Len())
		for element := violationQueue.Front(); element != nil; element = element.Next() {
			violations = append(violations, element.Value.(Violation))
		}

		td := &templateData{
			Violations: violations,
		}

		homeBuf := new(bytes.Buffer)
		err = app.tmpl.ExecuteTemplate(homeBuf, "home", td)
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

		<-ticker.C
	}
}
