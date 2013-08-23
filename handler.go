package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type CacheHandler struct {
}

func (*CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filelist := r.FormValue("filelist")
	validFilelist := strings.HasPrefix(filelist, "http://") || strings.HasPrefix(filelist, "https://")
	if !validFilelist {
		http.Error(w, "filelist must be http url", 400)
		return
	}
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
	createManifest := r.FormValue("createmanifest") == "true"

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

	var manifest *Manifest
	var manifestBuf *bytes.Buffer
	if createManifest {
		manifestBuf = new(bytes.Buffer)
		manifest = CreateManifest(manifestBuf)
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

	if manifest != nil {
		manifest.Close()
	}

	if createManifest && status.BadFiles == nil {
		w.Write(manifestBuf.Bytes())
	} else {
		buf, _ := json.MarshalIndent(status, "", "\t")
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
	}
}
