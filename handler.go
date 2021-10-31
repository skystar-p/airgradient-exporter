package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// e.g. {"wifi":-73,"pm02":297,"rco2":1009,"atmp":26.10,"rhum":51}
type metric struct {
	Id   string  `json:"-"`
	Wifi int     `json:"wifi"`
	PM25 int     `json:"pm02"`
	CO2  int     `json:"rco2"`
	Temp float64 `json:"atmp"`
	Hum  int     `json:"rhum"`
}

var (
	metricMutex sync.RWMutex
	lastMetric  metric
)

func mainHandler(w http.ResponseWriter, r *http.Request) {
	// read all body
	b, err := io.ReadAll(r.Body)
	if err != nil {
		errMsg := "cannot read body"
		logrus.WithError(err).Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	// unmarshal into metric struct
	var met metric
	if err := json.Unmarshal(b, &met); err != nil {
		errMsg := "failed to unmarshal"
		logrus.WithError(err).Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	vars := mux.Vars(r)
	// get id from variable
	instanceId := "null"
	if id, ok := vars["id"]; ok {
		splitted := strings.Split(id, ":")
		if len(splitted) == 2 {
			instanceId = splitted[1]
		}
	}
	met.Id = instanceId

	logrus.Debugf("received metric: %s", string(b))

	// store into lastMetric
	metricMutex.Lock()
	defer metricMutex.Unlock()
	lastMetric = met
}

const metricTemplate = `
# HELP instance The ID of the AirGradient sensor.
# instance %s

# HELP wifi Current WiFi signal strength, in dB
# TYPE wifi gauge
wifi %d

# HELP pm02 Particulat Matter PM2.5 value
# TYPE pm02 gauge
pm02 %d

# HELP rc02 CO2 value, in ppm
# TYPE rc02 gauge
rco2 %d

# HELP atmp Temperature, in degrees Celsius
# TYPE atmp gauge
atmp %f

# HELP rhum Relative humidity, in percent
# TYPE rhum gauge
rhum %d
`

func metricHandler(w http.ResponseWriter, r *http.Request) {
	// get last metric data
	var met metric
	metricMutex.RLock()
	met = lastMetric
	metricMutex.RUnlock()

	// write into response
	template := fmt.Sprintf(metricTemplate, met.Id, met.Wifi, met.PM25, met.CO2, met.Temp, met.Hum)
	if _, err := w.Write([]byte(template)); err != nil {
		errMsg := "failed to write response"
		logrus.Error(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
}
