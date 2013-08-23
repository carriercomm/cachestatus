package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ServerName of the current box
var ServerName string

type File struct {
	Path             string
	ChecksumExpected string
	Size             int64
	LastModified     time.Time
	LastChecked      time.Time
	Cached           bool
}

type FileChannel chan *File

var VERSION string = "1.3"

var (
	flagListLocation       = flag.String("filelist", "", "URL for filelist or manifest")
	flagCreateManifestPath = flag.String("createmanifest", "", "path for manifest to be created")
	flagServer             = flag.String("server", "localhost", "Server to check")
	flagHostname           = flag.String("hostname", "", "Host header for checks or source for creating manifest")
	flagChecksum           = flag.Bool("checksum", false, "Check (or create) checksums")
	flagWorkers            = flag.Int("workers", 6, "How many concurrent requests to make")
	flagVersion            = flag.Bool("version", false, "Show version")
	flagVerbose            = flag.Bool("verbose", false, "Verbose output")
	flagPort               = flag.String("port", "", "Http server port to listen on")
	flagHashFunction       = flag.String("hash", "sha256", "hash function, sha256 or crc32")
)

func init() {
	log.SetPrefix("cachestatus ")
	// log.SetFlags(log.Lmicroseconds | log.Lshortfile)

	flag.Parse()

	if *flagVersion {
		fmt.Println("cachestatus", VERSION)
		os.Exit(0)
	}

	var err error

	ServerName, err = os.Hostname()
	if err != nil {
		log.Fatalln("Could not get hostname", err)
	}

	ncpus := runtime.NumCPU()

	ncpus /= 2
	if ncpus > 6 {
		ncpus = 6
	}

	if *flagVerbose {
		log.Printf("Using up to %d CPUs for checksum'ing\n", ncpus)
	}
	runtime.GOMAXPROCS(ncpus)

}

func main() {
	if *flagPort != "" {
		log.Println("Http Server Mode.")
		serverMode()
	} else {
		log.Println("Command Line Mode.")
		commandLineMode()
	}
}

func serverMode() {
	http.Handle("/cachestatus", new(CacheHandler))
	http.ListenAndServe(":"+*flagPort, nil)
}

func commandLineMode() {
	if len(*flagListLocation) == 0 {
		log.Fatalln("-filelist url option is required")
	}

	vhost := new(VHost)
	vhost.FileListLocation = *flagListLocation
	vhost.Hostname = *flagHostname

	log.Println("Getting file list")
	err := getFileList(vhost)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Got file list")

	workQueue := make(FileChannel)

	nworkers := *flagWorkers

	status := NewStatusBoard(nworkers)

	waitGroup := new(sync.WaitGroup)

	w := NewWorkerGroup(vhost, *flagServer, status, workQueue)
	w.waitGroup = waitGroup
	if *flagChecksum {
		w.Options.Checksum = true
	}

	if len(*flagCreateManifestPath) > 0 {
		manifestFile, err := os.Create(*flagCreateManifestPath)
		if err != nil {
			log.Fatalf("Could not open manifest '%s': %s", *flagCreateManifestPath, err)
		}
		manifest := CreateManifest(manifestFile)

		defer manifest.Close()
		w.SetOutput(manifest.in)
	}

	for n := 0; n < nworkers; n++ {
		w.Start()
	}

	go status.Printer()

	for i, _ := range vhost.Files {
		// log.Printf("File: %#v\n", file)
		workQueue <- vhost.Files[i]
	}

	waitGroup.Wait()

	for n, st := range status.Status {
		log.Println(n, st.Path, st.Status, string(st.Mark))
	}

	for _, path := range status.BadFiles {
		fmt.Println(path)
	}

	log.Println(status.String())
	status.Quit()

	log.Println("exiting")
}

func openURL(rawurl string) (io.ReadCloser, error) {
	u, err := url.Parse(rawurl)

	if err != nil {
		return nil, fmt.Errorf("Could not parse url '%s': %s", rawurl, err)
	}

	if u.Scheme == "file" || (u.Scheme == "" && strings.HasPrefix(rawurl, "/")) {
		path := rawurl
		if len(u.Path) > 0 {
			path = u.Path
		}
		fh, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		return fh, nil
	}

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Could not get file list '%s': %d", u.String(), resp.StatusCode)
	}
	return resp.Body, nil
}

func getFileList(vhost *VHost) error {
	url := vhost.FileListLocation
	body, err := openURL(url)
	if err != nil {
		return fmt.Errorf("Could not get url %v: %v", url, err)
	}
	defer body.Close()

	if strings.HasSuffix(url, ".json") {
		files, err := ReadManifest(body)
		if err != nil {
			return errors.New(fmt.Sprintf("Error parsing manifest %s: %s", url, err))
		}
		vhost.Files = files
	}

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		checksumPath := strings.SplitN(scanner.Text(), "  .", 2)
		file := new(File)
		if len(checksumPath) > 1 {
			file.ChecksumExpected = checksumPath[0]
			file.Path = checksumPath[1]
		} else {
			file.Path = checksumPath[0]
		}
		if len(file.Path) == 0 {
			continue
		}

		vhost.Files = append(vhost.Files, file)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil

}
