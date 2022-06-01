package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// e.g. {"wifi":-73,"pm02":297,"rco2":1009,"atmp":26.10,"rhum":51}
type metric struct {
	Id string `json:"-"`
	Ts int64  `json:"ts,omitempty"`

	Wifi int     `json:"wifi"`
	PM25 int     `json:"pm02"`
	CO2  int     `json:"rco2"`
	Temp float64 `json:"atmp"`
	Hum  int     `json:"rhum"`
}

var (
	metricMutex sync.RWMutex
	lastMetric  *metric
)

func mainHandler(w http.ResponseWriter, r *http.Request) {
	if config.EnableBasicAuth {
		ok, errMsg := checkBasicAuthCredential(w, r)
		if !ok {
			logrus.Error(errMsg)
			http.Error(w, errMsg, http.StatusUnauthorized)
			return
		}
	}

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
	now := time.Now().Unix()
	metricMutex.Lock()
	defer metricMutex.Unlock()
	// use last value when CO2 censor return invalid value
	if met.CO2 <= 0 {
		met.CO2 = lastMetric.CO2
	}
	// also for PM25
	if met.PM25 <= 0 {
		met.PM25 = lastMetric.PM25
	}
	met.Ts = now
	lastMetric = &met

	go func(m metric) {
		b, err := json.Marshal(m)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal metric into json")
			return
		}
		if err := os.WriteFile(config.BackupFilename, b, 0644); err != nil {
			logrus.WithError(err).Error("failed to write into file")
			return
		}
	}(met)
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
	if config.EnableBasicAuth {
		ok, errMsg := checkBasicAuthCredential(w, r)
		if !ok {
			logrus.Error(errMsg)
			http.Error(w, errMsg, http.StatusUnauthorized)
			return
		}
	}

	// get last metric data
	var met metric
	metricMutex.RLock()
	if lastMetric == nil {
		// if lastMetric is nil, try to load last metric from file
		b, err := os.ReadFile(config.BackupFilename)
		if err != nil {
			logrus.WithError(err).Errorf("failed to read init data")
		} else {
			var met_ metric
			if err := json.Unmarshal(b, &met_); err != nil {
				logrus.WithError(err).Errorf("failed to unmarshal init data")
			} else {
				now := time.Now().Unix()
				if now-met_.Ts < config.MaxTimeDelta {
					// restore from local file only when time diff is less than `maxTimeDelta`
					met = met_
				}
			}
		}
	} else {
		met = *lastMetric
	}
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

func checkBasicAuthCredential(w http.ResponseWriter, r *http.Request) (bool, string) {
	username, password, ok := r.BasicAuth()
	if !ok {
		errMsg := "failed to get basic auth credential"
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		return false, errMsg
	}
	hashedUsername := sha256.Sum256([]byte(username))
	hashedPassword := sha256.Sum256([]byte(password))
	expectedUsernameHash := config.BasicAuthUsername
	expectedPasswordHash := config.BasicAuthPassword

	usernameMatch := (subtle.ConstantTimeCompare(hashedUsername[:], expectedUsernameHash[:]) == 1)
	passwordMatch := (subtle.ConstantTimeCompare(hashedPassword[:], expectedPasswordHash[:]) == 1)

	if !usernameMatch || !passwordMatch {
		errMsg := "credential mismatched"
		return false, errMsg
	}

	return true, ""
}
