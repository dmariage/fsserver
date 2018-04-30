package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
)

type key int

const (
	requestIDKey key = 0
)

type middleware func(http.Handler) http.Handler
type middlewares []middleware

func (mws middlewares) apply(hdlr http.Handler) http.Handler {
	if len(mws) == 0 {
		return hdlr
	}
	return mws[1:].apply(mws[0](hdlr))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := statusRecorder{w, 200}
			defer func() {
				//requestID, ok := r.Context().Value(requestIDKey).(string)
				requestID, ok := r.Context().Value("X-Request-Id").(string)
				if !ok {
					requestID = "reqId??"
				}
				connectedUser, ok := r.Context().Value("X-User").(string)
				if !ok {
					connectedUser = "kiça?"
				}
				//log.Printf("response code is : %v", rec.status)

				logger.Println("<" + requestID + ", " + connectedUser + ">", r.Method, r.URL.Path + " <" + strconv.Itoa(rec.status) + " " + http.StatusText(rec.status) + ">", r.RemoteAddr, r.UserAgent())
			}()

			next.ServeHTTP(&rec, r)
		})
	}
}

func tracing(nextRequestID func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = nextRequestID()
			}
			//ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			ctx := context.WithValue(r.Context(), "X-Request-Id", requestID)
			w.Header().Set("X-Request-Id", requestID)
			w.Header().Set("Server", "FileSystem WebServer")
			log.Print("added request id to context" + requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}