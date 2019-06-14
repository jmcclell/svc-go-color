package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var config Config
var randomSvc RandomService
var metrics = prometheus.NewRegistry()
var health = healthcheck.NewMetricsHandler(metrics, "color")
var serverStatus = Starting
var version = "dev"

type Config struct {
	Port                    int16         `split_words:"true" default:"80"`
	AdminPort               int16         `split_words:"true" default:"9000"`
	GracefulShutdownTimeout time.Duration `split_words:"true" default:"30s"`
	RandomServiceBaseUrl    string        `split_words:"true" default:"http://localhost/random"`
}

func main() {
	shutdown := make(chan os.Signal)
	signal.Notify(shutdown, os.Interrupt)

	initConfig()
	initRandomService()
	initAdminServer()

	router := http.NewServeMux()
	router.HandleFunc("/next", randomColorHandler)

	server := &http.Server{
		Handler: router,
	}

	log.Printf("Starting HTTP on 0.0.0.0:%d", config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.Fatal(err)
	}

	go server.Serve(listener)
	log.Println("Ready to serve requests")
	serverStatus = Running

	<-shutdown

	serverStatus = ShuttingDown
	log.Println("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}

	log.Println("Graceful shutdown complete.")
}

func initConfig() {
	err := envconfig.Process("", &config)
	if err != nil {
		log.Fatal(err)
	}
}

func initRandomService() {
	log.Printf("Initializing external random service at url: %s", config.RandomServiceBaseUrl)
	randomSvc = RandomService{BaseUrl: config.RandomServiceBaseUrl, Client: http.DefaultClient}
}

func initAdminServer() {
	initHealthcheck()

	adminRouter := http.NewServeMux()
	adminRouter.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{}))
	adminRouter.HandleFunc("/live", health.LiveEndpoint)
	adminRouter.HandleFunc("/ready", health.ReadyEndpoint)
	adminRouter.HandleFunc("/about", aboutHandler)

	adminServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.AdminPort),
		Handler: adminRouter,
	}

	log.Printf("Starting admin server on 0.0.0.0:%d", config.AdminPort)
	go func() {
		err := adminServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Println(err.Error())
		}
	}()
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	response := AboutResponse{Name: "color", Version: version, Hostname: hostname}
	response.render(w)
}

func initHealthcheck() {
	health.AddReadinessCheck("http", func() error {
		if serverStatus == Running {
			return nil
		} else {
			return fmt.Errorf("HTTP server is %s", serverStatus)
		}
	})
}

func randomColorHandler(w http.ResponseWriter, r *http.Request) {
	vals, err := randomSvc.next(0, 255, 3)
	if err != nil {
		ErrorResponse{Error: err.Error()}.render(w, http.StatusBadRequest)
		return
	}

	if len(vals) != 3 {
		log.Printf("Random service returned invalid data. Needed %d random numbers but got %d instead.", 3, len(vals))
		ErrorResponse{Error: "Invalid response from random service: wrong number of random  numbers received."}.render(w, http.StatusBadRequest)
		return
	}

	hex := fmt.Sprintf("%02x%02x%02x", vals[0], vals[1], vals[2])
	res := ColorResponse{Hex: fmt.Sprintf("#%s", hex), R: uint8(vals[0]), G: uint8(vals[1]), B: uint8(vals[2])}
	res.render(w)
}

type RandomService struct {
	BaseUrl string
	Client  *http.Client
}

func (s RandomService) next(min, max, num int) ([]int, error) {
	resp, err := s.Client.Get(fmt.Sprintf("%s/next?min=%d&max=%d&num=%d", s.BaseUrl, min, max, num))
	if err != nil {
		log.Printf("Error contacting random service: %s", err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	rand := new(RandomResponse)
	err = json.Unmarshal(body, rand)
	if err != nil {
		return nil, err
	}

	return rand.Values, nil
}

type RandomResponse struct {
	Values []int `json:"values"`
}

type ColorResponse struct {
	Hex string `json:"hex"`
	R   uint8  `json:"r"`
	G   uint8  `json:"g"`
	B   uint8  `json:"b"`
}

func (v ColorResponse) render(w http.ResponseWriter) {
	encoded, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded)
}

type AboutResponse struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Hostname string `json:"hostname"`
}

func (a AboutResponse) render(w http.ResponseWriter) {
	encoded, err := json.Marshal(a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded)
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (e ErrorResponse) render(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")

	encoded, err := json.Marshal(e)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{ "error": "%s" }`, err.Error()), http.StatusInternalServerError)
	}

	http.Error(w, string(encoded), code)
}

type ServerStatus int

const (
	Starting ServerStatus = iota
	Running
	ShuttingDown
)

func (s ServerStatus) String() string {
	return [...]string{"starting", "running", "shutting down"}[s]
}
