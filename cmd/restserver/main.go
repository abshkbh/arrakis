package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/chv-lambda/out/gen/serverapi"
	"github.com/abshkbh/chv-lambda/pkg/server"
)

const (
	restServerConfigKey = "host.restserver"
)

type restServer struct {
	vmServer *server.Server
}

// Implement handler functions
func (s *restServer) startVM(w http.ResponseWriter, r *http.Request) {
	var req serverapi.StartVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.GetVmName() == "" {
		http.Error(w, "Empty vm name", http.StatusBadRequest)
		return
	}

	if req.GetKernel() == "" {
		http.Error(w, "Empty kernel", http.StatusBadRequest)
		return
	}

	if req.GetRootfs() == "" {
		http.Error(w, "Empty rootfs", http.StatusBadRequest)
		return
	}

	resp, err := s.vmServer.StartVM(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) stopVM(w http.ResponseWriter, r *http.Request) {
	var req serverapi.VMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := s.vmServer.StopVM(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) destroyVM(w http.ResponseWriter, r *http.Request) {
	var req serverapi.VMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := s.vmServer.DestroyVM(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to destroy VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) destroyAllVMs(w http.ResponseWriter, r *http.Request) {
	resp, err := s.vmServer.DestroyAllVMs(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to destroy all VMs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) listAllVMs(w http.ResponseWriter, r *http.Request) {
	resp, err := s.vmServer.ListAllVMs(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list all VMs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) listVM(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]
	resp, err := s.vmServer.ListVM(r.Context(), vmName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	var serverConfig server.ServerConfig
	var configFile string

	app := &cli.App{
		Name:  "chv-restserver",
		Usage: "A daemon for spawning and managing cloud-hypervisor based microVMs.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Required:    true,
				Usage:       "Path to config file",
				Destination: &configFile,
			},
		},
		Action: func(ctx *cli.Context) error {
			viper.SetConfigFile(configFile)
			err := viper.ReadInConfig()
			if err != nil {
				return fmt.Errorf("failed to read config: %v", err)
			}

			restServerConfig := viper.Sub(restServerConfigKey)
			if restServerConfig == nil {
				return fmt.Errorf("restserver configuration not found")
			}

			if err := restServerConfig.Unmarshal(&serverConfig); err != nil {
				return fmt.Errorf("error unmarshalling config: %v", err)
			}
			log.Infof("server config: %v", serverConfig)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.WithError(err).Fatal("server exited with error")
	}

	// At this point `serverConfig` is populated.
	// Create the VM server
	vmServer, err := server.NewServer(serverConfig)
	if err != nil {
		log.Fatalf("failed to create VM server: %v", err)
	}

	// Create REST server
	s := &restServer{vmServer: vmServer}
	r := mux.NewRouter()

	// Register routes
	r.HandleFunc("/vm/start", s.startVM).Methods("POST")
	r.HandleFunc("/vm/stop", s.stopVM).Methods("POST")
	r.HandleFunc("/vm/destroy", s.destroyVM).Methods("POST")
	r.HandleFunc("/vm/destroy-all", s.destroyAllVMs).Methods("POST")
	r.HandleFunc("/vm/list", s.listAllVMs).Methods("GET")
	r.HandleFunc("/vm/{name}", s.listVM).Methods("GET")

	// Start HTTP server
	srv := &http.Server{
		Addr:    serverConfig.Host + ":" + serverConfig.Port,
		Handler: r,
	}

	go func() {
		log.Printf("REST server listening on: %s:%s", serverConfig.Host, serverConfig.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	vmServer.DestroyAllVMs(context.Background())
	log.Println("Server stopped")
}
