package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type configStruct struct {
	// http listen addr
	ListenAddr string `env:"LISTEN_ADDR" envDefault:"0.0.0.0:12321"`
	// internal http listen addr. used for prometheus scraping (without auth)
	InternalListenAddr string `env:"INTERNAL_LISTEN_ADDR" envDefault:"0.0.0.0:12322"`
	// last metric backup file name
	BackupFilename string `env:"BACKUP_FILENAME" envDefault:"/tmp/airgradient.json"`
	// max time diff when restoring last metric from file
	MaxTimeDelta int64 `env:"MAX_TIME_DELTA" envDefault:"60"`

	EnableBasicAuth bool `env:"ENABLE_BASIC_AUTH" envDefault:"false"`
	// http basic auth username, hashed with sha256, encoded with base64
	BasicAuthUsernameHashed string `env:"BASIC_AUTH_USERNAME_HASHED" envDefault:""`
	// http basic auth password, hashed with sha256, encoded with base64
	BasicAuthPasswordHashed string `env:"BASIC_AUTH_PASSWORD_HASHED" envDefault:""`

	// filled after env parse
	BasicAuthUsername []byte
	BasicAuthPassword []byte
}

var config configStruct

func parseEnv() {
	if err := env.Parse(&config); err != nil {
		logrus.WithError(err).Fatal("failed to parse env")
	}

	if config.EnableBasicAuth {
		u, err := base64.StdEncoding.DecodeString(config.BasicAuthUsernameHashed)
		if err != nil {
			logrus.WithError(err).Fatal("failed to parse basic auth username")
		}
		config.BasicAuthUsername = u

		p, err := base64.StdEncoding.DecodeString(config.BasicAuthPasswordHashed)
		if err != nil {
			logrus.WithError(err).Fatal("failed to parse basic auth password")
		}
		config.BasicAuthPassword = p
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

	var (
		server         *http.Server
		internalServer *http.Server
	)

	{
		r := mux.NewRouter()
		r.HandleFunc("/metrics", metricHandlerWrapper(false))
		r.HandleFunc("/sensors/{id}/measures", mainHandlerWrapper(false))

		server = &http.Server{
			Addr:         config.ListenAddr,
			Handler:      r,
			ReadTimeout:  time.Second * 10,
			WriteTimeout: time.Second * 10,
		}
	}
	{
		r := mux.NewRouter()
		r.HandleFunc("/metrics", metricHandlerWrapper(true))
		r.HandleFunc("/sensors/{id}/measures", mainHandlerWrapper(true))

		internalServer = &http.Server{
			Addr:         config.InternalListenAddr,
			Handler:      r,
			ReadTimeout:  time.Second * 10,
			WriteTimeout: time.Second * 10,
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	doneChan := make(chan struct{}, 2)

	// run internal http Server
	go func() {
		logrus.Infof("serving internal server at %s...", config.InternalListenAddr)
		if err := internalServer.ListenAndServe(); err != http.ErrServerClosed {
			logrus.WithError(err).Error("failed to run internal http server")
		}
		doneChan <- struct{}{}
	}()

	// run http server
	go func() {
		logrus.Infof("serving server at %s...", config.ListenAddr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logrus.WithError(err).Error("failed to run http server")
		}
		doneChan <- struct{}{}
	}()

	// wait for signal
	<-sigCh

	logrus.Info("shutting down http server...")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logrus.WithError(err).Error("failed to shutdown http server")
	}
	if err := internalServer.Shutdown(ctx); err != nil {
		logrus.WithError(err).Error("failed to shutdown internal http server")
	}

	<-doneChan
	<-doneChan
}
