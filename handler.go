
package main

import (
	"net/http"
	"strconv"
	"sync"
	"encoding/json"
	"fmt"
	"os"
	"io"
)

type CacheHandler struct {
	
}

func (*CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filelist := r.FormValue("filelist")
	createManifestPath := r.FormValue("createmanifest")
	server := r.FormValue("server")
	if server == "" {
		server = "localhost"
	}
	hostname := r.FormValue("hostname")
	workers, err := strconv.Atoi(r.FormValue("workers"))
	if err != nil {
		workers = 6
	}
	checksum := r.FormValue("checksum") == "true"
	if r.FormValue("version") == "true" {
		w.Header().Set("Version", VERSION)
	}
	vhost := new(VHost)
	vhost.FileListLocation = filelist
	vhost.Hostname = hostname
	err = getFileList(vhost)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	workQueue := make(FileChannel)
	status := NewStatusBoard(workers)
	waitGroup := new(sync.WaitGroup)
	workGroup := NewWorkerGroup(vhost, server, status, workQueue)
	workGroup.waitGroup = waitGroup
	workGroup.Options.Checksum = checksum

	if len(createManifestPath) > 0 {
		manifest, err := CreateManifest(createManifestPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not open manifest '%s': %s", createManifestPath, err), 400)
		}
		defer manifest.Close()
		workGroup.SetOutput(manifest.in)
	}

	for i := 0; i < workers; i++ {
		workGroup.Start()
	}

	for i, _ := range vhost.Files {
		// log.Printf("File: %#v\n", file)
		workQueue <- vhost.Files[i]
	}
	for i := 0; i < workers; i++ {
		workQueue <- nil
	}
	waitGroup.Wait()
	buf, _ := json.MarshalIndent(status, "", "\t")
	w.Write(buf)
}

type ManifestHandler struct {

}

func (*ManifestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	manifestPath := r.URL.Path[len("/manifest"):]

	if r.Method == "GET" {
		file, err := os.Open(manifestPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer file.Close()
		io.Copy(w, file)
	}else if r.Method == "POST" {
		file, err := os.Create(manifestPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer file.Close()
		io.Copy(file, r.Body)
	}
}

type FilelistHandler struct {

}

func (*FilelistHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filelistPath := r.URL.Path[len("/filelist"):]

	if r.Method == "GET" {
		file, err := os.Open(filelistPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer file.Close()
		io.Copy(w, file)
	} else if r.Method == "POST" {
		file, err := os.Create(filelistPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer file.Close()
		io.Copy(file, r.Body)
	}

}
