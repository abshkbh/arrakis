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
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/chv-starter-pack/out/gen/serverapi"
	"github.com/abshkbh/chv-starter-pack/pkg/config"
	"github.com/abshkbh/chv-starter-pack/pkg/server"
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

	resp, err := s.vmServer.StartVM(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) destroyVM(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	// Create request object with the VM name
	req := serverapi.VMRequest{
		VmName: &vmName,
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

func (s *restServer) snapshotVM(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req struct {
		OutputFile string `json:"outputFile,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Create the VMSnapshotRequest with the path parameter
	snapshotReq := serverapi.VMSnapshotRequest{
		VmName:     vmName,
		OutputFile: &req.OutputFile,
	}

	resp, err := s.vmServer.SnapshotVM(r.Context(), &snapshotReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) updateVMState(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req serverapi.VmsNamePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.GetStatus() != "stopped" {
		http.Error(w, "Status must be 'stopped'", http.StatusBadRequest)
		return
	}

	vmReq := serverapi.VMRequest{
		VmName: &vmName,
	}

	resp, err := s.vmServer.StopVM(r.Context(), &vmReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop VM: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	var serverConfig *config.ServerConfig
	var configFile string

	app := &cli.App{
		Name:  "chv-restserver",
		Usage: "A daemon for spawning and managing cloud-hypervisor based microVMs.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Path to config file",
				Destination: &configFile,
				Value:       "./config.yaml",
			},
		},
		Action: func(ctx *cli.Context) error {
			var err error
			serverConfig, err = config.GetServerConfig(configFile)
			if err != nil {
				return fmt.Errorf("server config not found: %v", err)
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
	vmServer, err := server.NewServer(*serverConfig)
	if err != nil {
		log.Fatalf("failed to create VM server: %v", err)
	}

	// Create REST server
	s := &restServer{vmServer: vmServer}
	r := mux.NewRouter()

	// Register routes
	r.HandleFunc("/vms", s.startVM).Methods("POST")
	r.HandleFunc("/vms/{name}", s.updateVMState).Methods("PATCH")
	r.HandleFunc("/vms/{name}", s.destroyVM).Methods("DELETE")
	r.HandleFunc("/vms", s.destroyAllVMs).Methods("DELETE")
	r.HandleFunc("/vms", s.listAllVMs).Methods("GET")
	r.HandleFunc("/vms/{name}", s.listVM).Methods("GET")
	r.HandleFunc("/vms/{name}/snapshots", s.snapshotVM).Methods("POST")

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

	// Nuke all remaining state.
	if err := os.RemoveAll(vmServer.GetStateDir()); err != nil {
		log.Warnf("Failed to remove state dir: %v", err)
	}
	log.Println("Server stopped")
}
