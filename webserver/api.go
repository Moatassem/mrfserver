package webserver

import (
	. "MRFGo/global"
	"MRFGo/sip"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"
)

func StartWS(ip net.IP) {
	r := http.NewServeMux()
	ws := fmt.Sprintf("%s:%d", ip, HttpTcpPort)
	srv := &http.Server{Addr: ws, Handler: r, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second}

	r.HandleFunc("GET /api/v1/session", serveSession)
	r.HandleFunc("GET /api/v1/stats", serveStats)
	r.Handle("GET /metrics", Prometrics.Handler())
	r.HandleFunc("GET /", serveHome)

	WtGrp.Add(1)
	go func() {
		defer WtGrp.Done()
		log.Fatal(srv.ListenAndServe())
	}()

	fmt.Print("Loading API Webserver...")
	fmt.Println("Success: HTTP", ws)

	fmt.Printf("Prometheus metrics available at http://%s/metrics\n", ws)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(fmt.Sprintf("<h1>%s API Webserver</h1>\n", B2BUAName)))
}

func serveSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var lst []string
	for _, ses := range sip.Sessions.Range() {
		lst = append(lst, ses.String())
	}

	data := struct {
		Sessions []string
	}{Sessions: lst}

	response, _ := json.Marshal(data)
	_, err := w.Write(response)
	if err != nil {
		LogError(LTWebserver, err.Error())
	}
}

func serveStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	BToMB := func(b uint64) uint64 {
		return b / 1000 / 1000
	}

	data := struct {
		CPUCount        int
		GoRoutinesCount int
		Alloc           uint64
		System          uint64
		GCCycles        uint32
		WaitGroupLength int
	}{CPUCount: runtime.NumCPU(),
		GoRoutinesCount: runtime.NumGoroutine(),
		Alloc:           BToMB(m.Alloc),
		System:          BToMB(m.Sys),
		GCCycles:        m.NumGC,
		WaitGroupLength: sip.WorkerCount + 3,
	}

	response, _ := json.Marshal(data)
	_, err := w.Write(response)
	if err != nil {
		LogError(LTWebserver, err.Error())
	}
}
