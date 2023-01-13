package birdnest

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"reaktor-birdnest/internal/models"
)

type Birdnest struct {
}

func (b Birdnest) GetReport() (models.Report, error) {
	resp, err := http.Get("https://assignments.reaktor.com/birdnest/drones")
	if err != nil {
		return models.Report{}, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.Report{}, err
	}

	var report models.Report
	err = xml.Unmarshal(body, &report)
	if err != nil {
		return models.Report{}, err
	}
	return report, nil
}

func (b Birdnest) GetDronePilot(droneSerialNumber string) (models.Pilot, error) {
	droneUrl, err := url.JoinPath("https://assignments.reaktor.com/birdnest/pilots", droneSerialNumber)
	if err != nil {
		return models.Pilot{}, err
	}

	resp, err := http.Get(droneUrl)
	if err != nil {
		return models.Pilot{}, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.Pilot{}, err
	}

	var pilot models.Pilot
	err = json.Unmarshal(body, &pilot)
	if err != nil {
		return models.Pilot{}, err
	}

	return pilot, nil
}
