package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/arrakis/out/gen/serverapi"
	"github.com/abshkbh/arrakis/pkg/config"
	"github.com/abshkbh/arrakis/pkg/server"
)

// sendErrorResponse sends a standardized error response to the client.
func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	resp := serverapi.ErrorResponse{
		Error: &serverapi.ErrorResponseError{
			Message: &message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

type restServer struct {
	vmServer *server.Server
}

// Implement handler functions
func (s *restServer) startVM(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "startVM")
	startTime := time.Now()

	var req serverapi.StartVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WithError(err).Error("Invalid request body")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	if req.GetVmName() == "" {
		logger.Error("Empty vm name")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			"Empty vm name")
		return
	}

	vmName := req.GetVmName()
	resp, err := s.vmServer.StartVM(r.Context(), &req)
	if err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Failed to start VM")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to start VM: %v", err))
		return
	}

	elapsedTime := time.Since(startTime)
	logger.WithFields(log.Fields{
		"vmName":      vmName,
		"startupTime": elapsedTime.String(),
	}).Info("VM started successfully")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) destroyVM(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "destroyVM")
	vars := mux.Vars(r)
	vmName := vars["name"]

	// Create request object with the VM name
	req := serverapi.VMRequest{
		VmName: &vmName,
	}

	resp, err := s.vmServer.DestroyVM(r.Context(), &req)
	if err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Failed to destroy VM")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to destroy VM: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) destroyAllVMs(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "destroyAllVMs")
	resp, err := s.vmServer.DestroyAllVMs(r.Context())
	if err != nil {
		logger.WithError(err).Error("Failed to destroy all VMs")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to destroy all VMs: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) listAllVMs(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "listAllVMs")
	resp, err := s.vmServer.ListAllVMs(r.Context())
	if err != nil {
		logger.WithError(err).Error("Failed to list all VMs")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to list all VMs: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) listVM(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "listVM")
	vars := mux.Vars(r)
	vmName := vars["name"]
	resp, err := s.vmServer.ListVM(r.Context(), vmName)
	if err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Failed to list VM")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to list VM: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) snapshotVM(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "snapshotVM")
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req struct {
		SnapshotId string `json:"snapshotId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Invalid request body")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	resp, err := s.vmServer.SnapshotVM(r.Context(), vmName, req.SnapshotId)
	if err != nil {
		logger.WithFields(log.Fields{
			"vmName":     vmName,
			"snapshotId": req.SnapshotId,
		}).WithError(err).Error("Failed to create snapshot")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to create snapshot: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) updateVMState(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "updateVMState")
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req serverapi.VmsNamePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Invalid request body")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	status := req.GetStatus()
	if status != "stopped" && status != "paused" && status != "resume" {
		logger.WithFields(log.Fields{
			"vmName": vmName,
			"status": status,
		}).Error("Invalid status value")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid status value: %s", status))
		return
	}

	vmReq := serverapi.VMRequest{
		VmName: &vmName,
	}

	var resp *serverapi.VMResponse
	var err error
	if status == "stopped" {
		resp, err = s.vmServer.StopVM(r.Context(), &vmReq)
	} else if status == "paused" {
		resp, err = s.vmServer.PauseVM(r.Context(), &vmReq)
	} else { // status == "resume"
		resp, err = s.vmServer.ResumeVM(r.Context(), &vmReq)
	}

	if err != nil {
		logger.WithFields(log.Fields{
			"vmName": vmName,
			"status": status,
		}).WithError(err).Error("Failed to update VM state")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to change VM state to '%s': %v", status, err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) vmCommand(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "vmCommand")
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req serverapi.VmCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Invalid request body")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	if req.GetCmd() == "" {
		logger.WithField("vmName", vmName).Error("Command cannot be empty")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			"Command cannot be empty")
		return
	}

	cmd := req.GetCmd()
	// Default to blocking if not specified
	blocking := true
	if req.Blocking != nil {
		blocking = *req.Blocking
	}

	resp, err := s.vmServer.VMCommand(r.Context(), vmName, cmd, blocking)
	if err != nil {
		logger.WithFields(log.Fields{
			"vmName": vmName,
			"cmd":    cmd,
		}).WithError(err).Error("Failed to execute command")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to execute command: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) vmFileUpload(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "vmFileUpload")
	vars := mux.Vars(r)
	vmName := vars["name"]

	var req serverapi.VmFileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.WithField("vmName", vmName).WithError(err).Error("Invalid request body")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	if len(req.GetFiles()) == 0 {
		logger.WithField("vmName", vmName).Error("No files provided")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			"No files provided for upload")
		return
	}

	files := req.GetFiles()
	resp, err := s.vmServer.VMFileUpload(r.Context(), vmName, files)
	if err != nil {
		logger.WithFields(log.Fields{
			"vmName":    vmName,
			"fileCount": len(files),
		}).WithError(err).Error("Failed to upload files")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to upload files: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *restServer) vmFileDownload(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("api", "vmFileDownload")
	vars := mux.Vars(r)
	vmName := vars["name"]

	paths := r.URL.Query().Get("paths")
	if paths == "" {
		logger.WithField("vmName", vmName).Error("Missing 'paths' query parameter")
		sendErrorResponse(
			w,
			http.StatusBadRequest,
			"Missing 'paths' query parameter")
		return
	}

	resp, err := s.vmServer.VMFileDownload(r.Context(), vmName, paths)
	if err != nil {
		logger.WithFields(log.Fields{
			"vmName": vmName,
			"paths":  paths,
		}).WithError(err).Error("Failed to download files")
		sendErrorResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to download files: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	var serverConfig *config.ServerConfig
	var configFile string

	app := &cli.App{
		Name:  "arrakis-restserver",
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
	r.HandleFunc("/vms/{name}/cmd", s.vmCommand).Methods("POST")
	r.HandleFunc("/vms/{name}/files", s.vmFileUpload).Methods("POST")
	r.HandleFunc("/vms/{name}/files", s.vmFileDownload).Methods("GET")

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
