package main

import (
	"net/http"
)

func newRouter(userhash, passhash []byte, realm string) *http.ServeMux {
	router := http.NewServeMux()

	// routes
	router.Handle("/healthz", healthz())
	//router.Handle("/", index(userhash, passhash, realm))
	router.Handle("/", authHandler(fsHandle, userhash, passhash, realm))

	return router
}

