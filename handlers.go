package main

import (
	"io"
	"net/http"
	"sync/atomic"
	"log"
	"path/filepath"
	"os"
	"io/ioutil"
	"encoding/json"
	"context"
	"crypto/subtle"
	"strings"
)

const maxUploadSize = 10 * 1024 * 1024 // 10 mb

type DirEntry struct{
	Name string
	Kind string
	Link string
}

func authHandler(handler http.HandlerFunc, userhash, passhash []byte, realm string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, basicAuthOk := r.BasicAuth()
		ctx := context.WithValue(r.Context(), "X-User", user)
		context.WithValue(r.Context(), "isAdmin", false)
		if !basicAuthOk {
			log.Println("unable to read basic auth")
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		} else {
			userData, userFoundOK := config.Users[user]
			if !userFoundOK {
				// try with admin user
				if subtle.ConstantTimeCompare(hasher(user),
					userhash) != 1 || subtle.ConstantTimeCompare(hasher(pass), passhash) != 1 {

					log.Printf("unknown user %s", user)
					w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
					http.Error(w, "Unauthorized.", http.StatusUnauthorized)
					return
				}
				context.WithValue(r.Context(), "isAdmin", true)
			} else if userData.Password != pass {
				log.Printf("invalid credentials for %s", user)
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
		}


		handler(w, r.WithContext(ctx))
	}
}

func fsHandle(w http.ResponseWriter, r *http.Request) {
	//targetPath := filepath.Join(uploadPath, strings.Replace(r.URL.Path, "/files", "", -1))
	targetPath := r.URL.Path

	connectedUser, _ := r.Context().Value("X-User").(string)
	isAdmin, _ := r.Context().Value("isAdmin").(bool)
	userData, _ := config.Users[connectedUser]
	log.Printf("handle %s:%s for user=%s\n", r.Method, targetPath, connectedUser)

	if !isAdmin {
		accessAllowed := false
		for path, mode := range userData.Paths {
			log.Printf("check allowed/required path '%s' / '%s'", path, targetPath)
			toTestPath := targetPath
			if !strings.HasSuffix(targetPath, "/") {
				toTestPath = toTestPath + "/"
			}
			// we compare with a / at the end of the paths to ensure /someZ is not accepted if path /some is allowed
			if strings.HasPrefix(toTestPath, path + "/") {
				if r.Method == "GET" || mode == "rw" {
					accessAllowed = true
				}
			}
		}

		if !accessAllowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}



	if r.Method == "POST" {

		if readWriteMode != "rw" {
			http.Error(w, "Operation not allowed", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			renderError(w, "FILE_TOO_BIG", http.StatusBadRequest)
			return
		}

		// parse and validate file and post parameters
		file, multipartFileHeader, err := r.FormFile("uploadFile")
		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			renderError(w, "INVALID_FILE", http.StatusBadRequest)
			return
		}

		fileName := multipartFileHeader.Filename

		log.Printf("receiving file '%s' to save into '%s'\n", fileName, r.URL.Path)
		targetDir := filepath.Join(fsBaseDir, targetPath)
		os.MkdirAll(targetDir, os.ModePerm);

		newPath := filepath.Join(targetDir, fileName)
		log.Printf("saving File to %s\n", newPath)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil {
			renderError(w, "CANT_WRITE_FILE", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{\"status\": \"OK\"}"))

	} else if r.Method == "GET" {

		targetDirOrFile := filepath.Join(fsBaseDir, targetPath)
		fileOrDir, err := os.Stat(targetDirOrFile)
		//if _, err := os.Stat(targetPath); err != nil {
		if err != nil {
			log.Printf("%s", err)
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			switch mode := fileOrDir.Mode(); {
			case mode.IsDir():
				// do directory stuff
				log.Printf("listing content of %s\n", targetDirOrFile)

				files, err := ioutil.ReadDir(targetDirOrFile)
				if err != nil {
					log.Fatal(err)
				}

				dirEntries := []DirEntry{}
				for _, f := range files {
					dirEntry := DirEntry{Name: f.Name()}
					if f.IsDir() {
						dirEntry.Kind = "directory"
					} else {
						dirEntry.Kind = "file"
					}
					dirEntry.Link = filepath.Join(targetPath, f.Name())

					dirEntries = append(dirEntries, dirEntry)
				}
				//fileList = strings.Join(dirEntries, ",")

				jsonResponse, err := json.Marshal(dirEntries)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(jsonResponse)
				log.Printf("response status set")

			case mode.IsRegular():
				// do file stuff
				log.Printf("streaming file '%s' it to response", fileOrDir.Name())
				//jsonResponse, _ := json.Marshal(fileOrDir.Name())
				//w.WriteHeader(http.StatusOK)
				//w.Write(jsonResponse)
				w.Header().Set("Content-Disposition", "inline; filename=\"" + fileOrDir.Name() + "\"")
				http.ServeFile(w, r, targetDirOrFile)
			}


		}

	} else if r.Method == "HEAD" {
		targetDirOrFile := filepath.Join(fsBaseDir, targetPath)
		log.Printf("Testing if file or dir exists '%s'", targetDirOrFile)
		if _, err := os.Stat(targetDirOrFile); err != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	} else {
		log.Print("Bad request verb " + r.Method)
		w.WriteHeader(http.StatusBadRequest)
	}



}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			io.WriteString(w, `{"alive": true, "fsPath": "` + fsBaseDir + `"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func renderError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("{\"status\": \"ERROR\", \"message\": \""+message+"\"}" ))
}