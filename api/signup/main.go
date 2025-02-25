package main

import (
	"github.com/micro/micro/v3/service"
	mauth "github.com/micro/micro/v3/service/auth/client"
	log "github.com/micro/micro/v3/service/logger"
	"m3o.dev/api/pkg/tracing"
	"m3o.dev/api/signup/handler"
)

func main() {
	// New Service
	srv := service.New(
		service.Name("signup"),
	)

	// passing in auth because the DefaultAuth is the one used to set up the service
	auth := mauth.NewAuth()

	// Register Handler
	srv.Handle(handler.NewSignup(srv, auth))

	// kick off event consumption
	handler.NewOnboarding(srv)
	traceCloser := tracing.SetupOpentracing("signup")
	defer traceCloser.Close()

	// Run service
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
