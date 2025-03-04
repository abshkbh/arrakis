package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/chv-starter-pack/out/gen/serverapi"
	"github.com/abshkbh/chv-starter-pack/pkg/config"
)

var (
	apiClient *serverapi.APIClient
)

func stopVM(vmName string) error {
	req := apiClient.DefaultAPI.VmsNamePatch(context.Background(), vmName)

	req = req.VmsNamePatchRequest(serverapi.VmsNamePatchRequest{
		Status: serverapi.PtrString("stopped"),
	})

	_, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to stop VM: error: %s code: %v", string(body), err)
	}

	log.Infof("successfully stopped VM: %s", vmName)
	return nil
}

func destroyVM(vmName string) error {
	_, http_resp, err := apiClient.
		DefaultAPI.
		VmsNameDelete(context.Background(), vmName).Execute()
	if err != nil {
		body, _ := io.ReadAll(http_resp.Body)
		return fmt.Errorf("failed to destroy VM: error: %s code: %v", string(body), err)
	}

	log.Infof("successfully destroyed VM: %s", vmName)
	return nil
}

func destroyAllVMs() error {
	_, http_resp, err := apiClient.DefaultAPI.VmsDelete(context.Background()).Execute()
	if err != nil {
		body, _ := io.ReadAll(http_resp.Body)
		return fmt.Errorf("failed to destroy all VMs: error: %s code: %v", string(body), err)
	}

	log.Infof("destroyed all VMs")
	return nil
}

func startVM(
	vmName string,
	kernel string,
	rootfs string,
	entryPoint string,
	snapshotId string,
) error {
	var startVMRequest *serverapi.StartVMRequest
	if snapshotId != "" {
		// If snapshot ID is provided, restore the VM from the snapshot
		startVMRequest = &serverapi.StartVMRequest{
			VmName:     serverapi.PtrString(vmName),
			SnapshotId: serverapi.PtrString(snapshotId),
		}
	} else {
		startVMRequest = &serverapi.StartVMRequest{
			VmName:     serverapi.PtrString(vmName),
			Kernel:     serverapi.PtrString(kernel),
			Rootfs:     serverapi.PtrString(rootfs),
			EntryPoint: serverapi.PtrString(entryPoint),
		}
	}

	resp, http_resp, err := apiClient.DefaultAPI.
		VmsPost(context.Background()).
		StartVMRequest(*startVMRequest).Execute()
	if err != nil {
		body, _ := io.ReadAll(http_resp.Body)
		return fmt.Errorf("failed to start VM: error: %s code: %v", string(body), err)
	}

	resp_bytes, err := resp.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	log.Infof("started VM: %v", string(resp_bytes))
	return nil
}

func listAllVMs() error {
	resp, http_resp, err := apiClient.DefaultAPI.VmsGet(context.Background()).Execute()
	if err != nil {
		body, _ := io.ReadAll(http_resp.Body)
		return fmt.Errorf("failed to list all VMs: error: %s code: %v", string(body), err)
	}

	// Format output to better show port forwards with descriptions
	fmt.Println("Available VMs:")
	fmt.Println("-------------")

	for _, vm := range resp.GetVms() {
		fmt.Printf("VM Name: %s\n", vm.GetVmName())
		fmt.Printf("Status: %s\n", vm.GetStatus())
		fmt.Printf("IP Address: %s\n", vm.GetIp())
		fmt.Printf("Tap Device: %s\n", vm.GetTapDeviceName())

		// Print port forwards with descriptions
		if len(vm.GetPortForwards()) > 0 {
			fmt.Println("Port Forwards:")
			for _, pf := range vm.GetPortForwards() {
				fmt.Printf("  %s -> %s: %s\n",
					pf.GetHostPort(),
					pf.GetGuestPort(),
					pf.GetDescription())
			}
		}
		fmt.Println("-------------")
	}

	return nil
}

func createApiClient(serverAddr string) (*serverapi.APIClient, error) {
	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server address: %v", err)
	}

	serverConfiguration := &serverapi.ServerConfiguration{
		URL:         "http://{host}:{port}",
		Description: "Development server",
		Variables: map[string]serverapi.ServerVariable{
			"host": {
				Description:  "host",
				DefaultValue: host,
			},
			"port": {
				Description:  "port",
				DefaultValue: port,
			},
		},
	}

	configuration := serverapi.NewConfiguration()
	configuration.Servers = serverapi.ServerConfigurations{
		*serverConfiguration,
	}
	apiClient = serverapi.NewAPIClient(configuration)

	return apiClient, nil
}

func snapshotVM(vmName string, snapshotId string) error {
	if snapshotId == "" {
		timestamp := time.Now().Format("20060102-150405")
		snapshotId = fmt.Sprintf("snapshot-%s-%s", vmName, timestamp)
	}

	req := apiClient.DefaultAPI.VmsNameSnapshotsPost(context.Background(), vmName)
	req = req.VmsNameSnapshotsPostRequest(serverapi.VmsNameSnapshotsPostRequest{
		SnapshotId: serverapi.PtrString(snapshotId),
	})

	resp, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to create snapshot: error: %s code: %v", string(body), err)
	}
	log.Infof("successfully created snapshot for VM %s with ID %s", vmName, resp.GetSnapshotId())
	return nil
}

func restoreVM(vmName string, snapshotId string) error {
	return startVM(vmName, "", "", "", snapshotId)
}

func pauseVM(vmName string) error {
	req := apiClient.DefaultAPI.VmsNamePatch(context.Background(), vmName)
	req = req.VmsNamePatchRequest(serverapi.VmsNamePatchRequest{
		Status: serverapi.PtrString("paused"),
	})
	_, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to pause VM: error: %s code: %v", string(body), err)
	}
	log.Infof("successfully paused VM: %s", vmName)
	return nil
}

func resumeVM(vmName string) error {
	req := apiClient.DefaultAPI.VmsNamePatch(context.Background(), vmName)
	req = req.VmsNamePatchRequest(serverapi.VmsNamePatchRequest{
		Status: serverapi.PtrString("resume"),
	})
	_, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to resume VM: error: %s code: %v", string(body), err)
	}
	log.Infof("successfully resumed VM: %s", vmName)
	return nil
}

func uploadFiles(vmName string, fileSpecs []string) error {
	if len(fileSpecs) == 0 || len(fileSpecs)%2 != 0 {
		return fmt.Errorf("invalid number of file specifications: must be even")
	}

	apiFiles := make([]serverapi.VmFileUploadRequestFilesInner, len(fileSpecs)/2)
	for i := 0; i < len(fileSpecs); i += 2 {
		sourcePath := fileSpecs[i]
		destPath := fileSpecs[i+1]

		log.Infof("uploading file from %s to %s", sourcePath, destPath)

		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %v", sourcePath, err)
		}

		apiFiles[i/2] = serverapi.VmFileUploadRequestFilesInner{
			Path:    destPath,
			Content: string(content),
		}
	}

	req := apiClient.DefaultAPI.VmsNameFilesPost(context.Background(), vmName)
	req = req.VmFileUploadRequest(serverapi.VmFileUploadRequest{
		Files: apiFiles,
	})

	_, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to upload files: error: %s code: %v", string(body), err)
	}

	log.Infof("successfully uploaded %d files to VM: %s", len(fileSpecs)/2, vmName)
	return nil
}

func runCommand(vmName string, cmd string) error {
	req := apiClient.DefaultAPI.VmsNameCmdPost(context.Background(), vmName)
	req = req.VmCommandRequest(serverapi.VmCommandRequest{
		Cmd: cmd,
	})

	resp, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to run command: error: %s code: %v", string(body), err)
	}

	if resp.GetError() != "" {
		return fmt.Errorf("command failed: %s\nOutput: %s", resp.GetError(), resp.GetOutput())
	}

	fmt.Println(resp.GetOutput())
	return nil
}

func downloadFiles(vmName string, paths []string) error {
	pathsParam := strings.Join(paths, ",")
	req := apiClient.DefaultAPI.VmsNameFilesGet(context.Background(), vmName)
	req = req.Paths(pathsParam)

	resp, httpResp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to download files: error: %s code: %v", string(body), err)
	}

	for _, file := range resp.GetFiles() {
		if file.GetError() != "" {
			log.Warnf("error downloading %s: %s", file.GetPath(), file.GetError())
			continue
		}

		err := os.WriteFile(file.GetPath(), []byte(file.GetContent()), 0644)
		if err != nil {
			log.Warnf("failed to write file %s: %v", file.GetPath(), err)
			continue
		}
		log.Infof("downloaded file: %s", file.GetPath())
	}
	return nil
}

func info(vmName string) error {
	resp, http_resp, err := apiClient.DefaultAPI.VmsNameGet(context.Background(), vmName).Execute()
	if err != nil {
		body, _ := io.ReadAll(http_resp.Body)
		return fmt.Errorf("failed to get VM info: error: %s code: %v", string(body), err)
	}

	fmt.Printf("VM Name: %s\n", resp.GetVmName())
	fmt.Printf("Status: %s\n", resp.GetStatus())
	fmt.Printf("IP Address: %s\n", resp.GetIp())
	fmt.Printf("Tap Device: %s\n", resp.GetTapDeviceName())

	// Print port forwards with descriptions
	if len(resp.GetPortForwards()) > 0 {
		fmt.Println("Port Forwards:")
		for _, pf := range resp.GetPortForwards() {
			fmt.Printf("  %s -> %s: %s\n",
				pf.GetHostPort(),
				pf.GetGuestPort(),
				pf.GetDescription())
		}
	}

	return nil
}

func main() {
	app := &cli.App{
		Name:  "chv-client",
		Usage: "A CLI for managing VMs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to config file",
				Value:   "./config.yaml",
			},
		},
		Before: func(ctx *cli.Context) error {
			configPath := ctx.String("config")
			clientConfig, err := config.GetClientConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to get client config: %v", err)
			}
			log.Infof("client config: %v", clientConfig)

			apiClient, err = createApiClient(
				fmt.Sprintf("%s:%s", clientConfig.ServerHost, clientConfig.ServerPort),
			)
			if err != nil {
				return fmt.Errorf("failed to initialize api client: %v", err)
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to create",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "kernel",
						Aliases: []string{"k"},
						Usage:   "Path of the kernel image to be used",
					},
					&cli.StringFlag{
						Name:    "rootfs",
						Aliases: []string{"r"},
						Usage:   "Path of the rootfs image to be used",
					},
					&cli.StringFlag{
						Name:     "entry-point",
						Aliases:  []string{"e"},
						Usage:    "Entry point of the VM",
						Required: false,
					},
					&cli.StringFlag{
						Name:    "snapshot",
						Aliases: []string{"s"},
						Usage:   "Path to snapshot directory to restore from",
					},
				},
				Action: func(ctx *cli.Context) error {
					return startVM(
						ctx.String("name"),
						ctx.String("kernel"),
						ctx.String("rootfs"),
						ctx.String("entry-point"),
						ctx.String("snapshot"),
					)
				},
			},
			{
				Name:  "stop",
				Usage: "Stop a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to stop",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return stopVM(ctx.String("name"))
				},
			},
			{
				Name:  "destroy",
				Usage: "Destroy a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to destroy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return destroyVM(ctx.String("name"))
				},
			},
			{
				Name:  "destroy-all",
				Usage: "Destroy all VMs",
				Action: func(ctx *cli.Context) error {
					return destroyAllVMs()
				},
			},
			{
				Name:  "list-all",
				Usage: "List all VMs",
				Action: func(ctx *cli.Context) error {
					return listAllVMs()
				},
			},
			{
				Name:  "info",
				Usage: "List VM info",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to destroy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return info(ctx.String("name"))
				},
			},
			{
				Name:  "snapshot",
				Usage: "Create a snapshot of a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "id",
						Aliases:  []string{"i"},
						Usage:    "Unique identifier for the snapshot",
						Required: false,
					},
				},
				Action: func(ctx *cli.Context) error {
					return snapshotVM(ctx.String("name"), ctx.String("id"))
				},
			},
			{
				Name:  "restore",
				Usage: "Restore a VM from a snapshot",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name to give to the restored VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "id",
						Aliases:  []string{"i"},
						Usage:    "ID of the snapshot to restore from",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return restoreVM(ctx.String("name"), ctx.String("id"))
				},
			},
			{
				Name:  "pause",
				Usage: "Pause a running VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to pause",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return pauseVM(ctx.String("name"))
				},
			},
			{
				Name:  "resume",
				Usage: "Resume a paused VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to resume",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return resumeVM(ctx.String("name"))
				},
			},
			{
				Name:  "upload",
				Usage: "Upload files to a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "file",
						Aliases:  []string{"f"},
						Usage:    "File(s) to upload in format 'source,destination' (can be specified multiple times)",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return uploadFiles(ctx.String("name"), ctx.StringSlice("file"))
				},
			},
			{
				Name:  "run",
				Usage: "Run a command in a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "cmd",
						Aliases:  []string{"c"},
						Usage:    "Command to run",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return runCommand(ctx.String("name"), ctx.String("cmd"))
				},
			},
			{
				Name:  "download",
				Usage: "Download files from a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "path",
						Aliases:  []string{"p"},
						Usage:    "Path(s) to download (can be specified multiple times)",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return downloadFiles(ctx.String("name"), ctx.StringSlice("path"))
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
