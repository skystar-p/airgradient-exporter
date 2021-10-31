package main

import (
	"net/http"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type configStruct struct {
	// http listen addr
	ListenAddr string `env:"LISTEN_ADDR" envDefault:"0.0.0.0:12321"`
}

var config configStruct

func parseEnv() {
	if err := env.Parse(&config); err != nil {
		panic(err)
	}
}

func main() {
	// parse environment variable
	parseEnv()

	// include timestamp in log
	formatter := &logrus.TextFormatter{
		FullTimestamp: true,
	}
	logrus.SetFormatter(formatter)

	r := mux.NewRouter()
	r.HandleFunc("/metrics", metricHandler)
	r.HandleFunc("/sensors/{id}/measures", mainHandler)

	server := &http.Server{
		Addr:         config.ListenAddr,
		Handler:      r,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
	}

	// run http server
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
