package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/grafana/tns/utils"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
)


func main() {
	serverConfig := server.Config{
		MetricsNamespace:    "tns",
		ExcludeRequestInLog: true,
	}
	serverConfig.RegisterFlags(flag.CommandLine)
	flag.Parse()

	// Use a gokit logger, and tell the server to use it.
	logger := level.NewFilter(log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)), serverConfig.LogLevel.Gokit)
	serverConfig.Log = logging.GoKit(logger)

	closeTracing := utils.SetupTracing("db", logger)
	defer closeTracing()

	s, err := server.New(serverConfig)
	if err != nil {
		level.Error(logger).Log("msg", "error starting server", "err", err)
		os.Exit(1)
	}
	defer s.Shutdown()

	rand.Seed(time.Now().UnixNano())

	db := New(logger)

	s.HTTP.HandleFunc("/", db.Fetch)
	s.HTTP.HandleFunc("/fail", db.Fail)
	s.HTTP.HandleFunc("/post", db.Post)
	s.HTTP.HandleFunc("/vote", db.Vote)

	s.Run()
}

type db struct {
	logger log.Logger

	mtx   sync.Mutex
	fail  bool
	links map[int]*Link
}

type Link struct {
	ID     int
	Points int
	URL    string
	Title  string
}

func New(logger log.Logger) *db {
	return &db{
		logger: logger,
		links:  map[int]*Link{},
	}
}

func (db *db) Fail(w http.ResponseWriter, r *http.Request) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	db.fail = !db.fail
	level.Info(db.logger).Log("msg", "toggled fail flag", "fail", db.fail)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "failing: %t\n", db.fail)
}

func (db *db) Fetch(w http.ResponseWriter, r *http.Request) {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	// Every 5mins, randomly fail 40% of requests for 30 seconds.
	if time.Now().Unix()%(5*60) < 30 && rand.Intn(10) <= 8 {
		time.Sleep(50 * time.Millisecond)
		if rand.Intn(10) <= 4 {
			level.Error(db.logger).Log("err", "too many open connections")
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			level.Error(db.logger).Log("err", "query lock timeout")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	if db.fail {
		level.Error(db.logger).Log("err", "spline matriculation failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	links := make([]*Link, 0, len(db.links))
	for _, link := range db.links {
		links = append(links, link)
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Points != links[j].Points {
			return links[i].Points > links[j].Points
		}
		return links[i].ID > links[j].ID
	})

	max := 10
	if len(links) < max {
		max = len(links)
	}
	links = links[:max]

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(struct {
		Links []*Link
	}{
		Links: links,
	}); err != nil {
		level.Error(db.logger).Log("msg", "error encoding response", "err", err)
	}
}

func (db *db) Post(w http.ResponseWriter, r *http.Request) {
	var link Link
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		level.Error(db.logger).Log("msg", "error decoding link", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db.mtx.Lock()
	defer db.mtx.Unlock()

	if _, ok := db.links[link.ID]; ok {
		http.Error(w, "post exists", http.StatusAlreadyReported)
		return
	}

	db.links[link.ID] = &link
	w.WriteHeader(http.StatusNoContent)
}

func (db *db) Vote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		level.Error(db.logger).Log("msg", "error decoding link", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db.mtx.Lock()
	defer db.mtx.Unlock()
	db.links[req.ID].Points++

	w.WriteHeader(http.StatusNoContent)
}
