package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/abshkbh/chv-starter-pack/out/gen/chvapi"
	"github.com/abshkbh/chv-starter-pack/out/gen/serverapi"
	"github.com/abshkbh/chv-starter-pack/pkg/cmdserver"
	"github.com/abshkbh/chv-starter-pack/pkg/config"
	"github.com/abshkbh/chv-starter-pack/pkg/server/cidallocator"
	"github.com/abshkbh/chv-starter-pack/pkg/server/fountain"
	"github.com/abshkbh/chv-starter-pack/pkg/server/ipallocator"
	"github.com/abshkbh/chv-starter-pack/pkg/server/portallocator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gvisor.dev/gvisor/pkg/cleanup"
)

// vmStatus represents the status of a VM.
type vmStatus int

const (
	vmStatusCreated vmStatus = iota
	vmStatusRunning
	vmStatusStopped
	vmStatusPaused
)

func (status vmStatus) String() string {
	switch status {
	case vmStatusCreated:
		return "CREATED"
	case vmStatusRunning:
		return "RUNNING"
	case vmStatusStopped:
		return "STOPPED"
	case vmStatusPaused:
		return "PAUSED"
	default:
		return "UNKNOWN"
	}
}

const (
	// Case sensitive.
	serialPortMode = "Tty"
	// Case sensitive.
	consolePortMode = "Off"

	numNetDeviceQueues      = 2
	netDeviceQueueSizeBytes = 256
	netDeviceId             = "_net0"
	reapVmTimeout           = 20 * time.Second

	portAllocatorLowPort  = 3000
	portAllocatorHighPort = 6000

	cidAllocatorLow  = 3
	cidAllocatorHigh = 1000 // Or whatever upper limit makes sense

	statefulDiskFilename      = "stateful.img"
	minGuestMemoryMB          = 1024
	maxGuestMemoryMB          = 32768
	defaultGuestMemPercentage = 50
)

type portForward struct {
	hostPort    int32
	guestPort   int32
	description string
}

func String(s string) *string {
	return &s
}

func Int32(i int32) *int32 {
	return &i
}

func Bool(b bool) *bool {
	return &b
}

type vm struct {
	lock          sync.RWMutex
	name          string
	stateDirPath  string
	apiSocketPath string
	apiClient     *chvapi.APIClient
	process       *os.Process
	ip            *net.IPNet
	tapDevice     *fountain.TapDevice
	status        vmStatus
	portForwards  []portForward
	// This is actually a unix domain socket path that maps to all vsock server
	// running inside the VM. A "CONNECT <port>" command sent on this socket
	// will be forwarded to the vsock server listening on the given port inside
	// the VM. This is as per the cloud-hypervisor vsock implementation.
	vsockPath        string
	cid              uint32
	statefulDiskPath string
}

// calculateVCPUCount returns an appropriate number of vCPUs based on host's CPU count.
// It ensures the VM has enough CPU resources while not overcommitting the host.
func calculateVCPUCount() int32 {
	hostCPUs := int32(runtime.NumCPU())
	minVCPUs := int32(1)
	maxVCPUs := int32(8)
	suggestedVCPUs := hostCPUs / 2

	if suggestedVCPUs < minVCPUs {
		return minVCPUs
	}
	if suggestedVCPUs > maxVCPUs {
		return maxVCPUs
	}
	return suggestedVCPUs
}

// calculateGuestMemorySizeInMB calculates the appropriate memory size for the guest.
func calculateGuestMemorySizeInMB(memoryPercentage int32) (int32, error) {
	if memoryPercentage <= 0 || memoryPercentage > 100 {
		memoryPercentage = defaultGuestMemPercentage
		log.Warnf(
			"Invalid memory percentage provided: %d, using default of %d%%",
			memoryPercentage,
			defaultGuestMemPercentage,
		)
	}

	var totalMemoryKB int64
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Warn("Could not determine host memory size, using default of 4096 MB")
		return minGuestMemoryMB, nil
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					totalMemoryKB = memKB
					break
				}
			}
		}
	}
	if totalMemoryKB <= 0 {
		return 0, fmt.Errorf("could not determine host memory size")
	}
	log.Infof("Total host memory: %d MB", totalMemoryKB/1024)

	suggestedMemoryKB := (totalMemoryKB * int64(memoryPercentage)) / 100
	if suggestedMemoryKB < minGuestMemoryMB*1024 {
		return 0, fmt.Errorf(
			"host memory allocation too small. suggested memory: %d MB (at %d%%) total memory: %d MB",
			suggestedMemoryKB/1024,
			memoryPercentage,
			totalMemoryKB/1024,
		)
	}
	if suggestedMemoryKB > maxGuestMemoryMB*1024 {
		return maxGuestMemoryMB, nil
	}
	return int32(suggestedMemoryKB / 1024), nil
}

func getKernelCmdLine(gatewayIP string, guestIP string) string {
	return fmt.Sprintf(
		"console=ttyS0 gateway_ip=\"%s\" guest_ip=\"%s\"",
		gatewayIP,
		guestIP,
	)
}

// bridgeExists checks if a bridge with the given name exists.
func bridgeExists(bridgeName string) (bool, error) {
	cmd := exec.Command("ip", "link", "show", "type", "bridge")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("error executing command: %v", err)
	}

	bridges := strings.Split(string(output), "\n")

	for _, bridge := range bridges {
		if strings.Contains(bridge, bridgeName+":") {
			return true, nil
		}
	}

	return false, nil
}

// setupPortForwardsToVM forwards the given port forwards to the VM.
func (s *Server) setupPortForwardsToVM(vmIP string, guestPorts []config.PortForwardConfig) ([]portForward, error) {
	portForwards := make([]portForward, 0, len(guestPorts))
	for _, guestPortConfig := range guestPorts {
		guestPort, err := strconv.ParseInt(guestPortConfig.Port, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid guest port %s: %w", guestPortConfig.Port, err)
		}

		hostPort, err := s.portAllocator.AllocatePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}

		portForwards = append(portForwards, portForward{
			hostPort:    hostPort,
			guestPort:   int32(guestPort),
			description: guestPortConfig.Description,
		})

		log.Infof(
			"Setting up port forward %d -> %s:%d (%s)",
			hostPort,
			vmIP,
			guestPort,
			guestPortConfig.Description,
		)
		cmd := exec.Command(
			"iptables",
			"-t",
			"nat",
			"-A",
			"PREROUTING",
			"-p",
			"tcp",
			"--dport",
			strconv.Itoa(int(hostPort)),
			"-j",
			"DNAT",
			"--to-destination",
			fmt.Sprintf("%s:%d", vmIP, guestPort),
		)

		err = cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("error forwarding port %d->%s:%d: %w", hostPort, vmIP, guestPort, err)
		}
	}
	return portForwards, nil
}

func cleanupAllIPTablesRulesForIP(ip string) error {
	log.Infof("deleting all iptables rules for IP: %s", ip)
	// First, list all rules in the NAT table PREROUTING chain.
	cmd := exec.Command("iptables", "-t", "nat", "-L", "PREROUTING", "-n", "--line-numbers")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list iptables rules: %w", err)
	}

	// Parse the output to find rule numbers that match our IP.
	lines := strings.Split(string(output), "\n")
	var ruleNumbers []int

	// Skip the first two lines (headers).
	for i := 2; i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "to:"+ip+":") {
			log.Infof("deleting rule: %s", line)
			// Extract the rule number (first field).
			fields := strings.Fields(line)
			if len(fields) > 0 {
				ruleNum, err := strconv.Atoi(fields[0])
				if err == nil {
					ruleNumbers = append(ruleNumbers, ruleNum)
				}
			}
		}
	}

	// Delete rules in reverse order to minimize the number of rules that need to be deleted.
	sort.Sort(sort.Reverse(sort.IntSlice(ruleNumbers)))

	var finalErr error
	for _, ruleNum := range ruleNumbers {
		cmd := exec.Command(
			"iptables",
			"-t",
			"nat",
			"-D",
			"PREROUTING",
			strconv.Itoa(ruleNum),
		)

		if err := cmd.Run(); err != nil {
			log.Warnf("error deleting iptables rule %d for IP %s: %v", ruleNum, ip, err)
			finalErr = errors.Join(
				finalErr,
				fmt.Errorf("failed to delete rule %d: %w", ruleNum, err),
			)
		}
	}
	return finalErr
}

func cleanupTapDevices() error {
	// List all network interfaces.
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to list interfaces: %v", err)
	}

	for _, iface := range interfaces {
		// Check if interface name starts with "tap".
		if strings.HasPrefix(iface.Name, "tap") {
			if err := exec.Command("ip", "link", "delete", iface.Name).Run(); err != nil {
				log.Warnf("failed to delete tap device %s: %v", iface.Name, err)
			}
			log.Infof("deleted tap device: %s", iface.Name)
		}
	}
	return nil
}

func cleanupBridge() error {
	// Check if br0 exists.
	_, err := exec.Command("ip", "link", "show", "br0").CombinedOutput()
	if err != nil {
		// Bridge doesn't exist, nothing to do.
		return nil
	}

	// Bridge exists, delete it
	if err := exec.Command("ip", "link", "delete", "br0").Run(); err != nil {
		return fmt.Errorf("failed to delete bridge br0: %v", err)
	}
	log.Info("deleted bridge: br0")
	return nil
}

// setupBridgeAndFirewall sets up a bridge and firewall rules for the given bridge name, IP address, and subnet.
func setupBridgeAndFirewall(
	backupFile string,
	bridgeName string,
	bridgeIP string,
	bridgeSubnet string,
) error {
	output, err := exec.Command("iptables-save").Output()
	if err != nil {
		return fmt.Errorf("failed to run iptables-save: %w", err)
	}

	err = os.WriteFile(backupFile, output, 0644)
	if err != nil {
		return fmt.Errorf("failed to save iptables-save to: %v: %w", backupFile, err)
	}

	// Get default network interface
	output, err = exec.Command("sh", "-c", "ip r | grep default | awk '{print $5}'").Output()
	if err != nil {
		return fmt.Errorf("failed to get default network interface: %w", err)
	}
	hostDefaultNetworkInterface := strings.TrimSpace(string(output))

	exists, err := bridgeExists(bridgeName)
	if err != nil {
		return fmt.Errorf("failed to detect if bridge exists: %w", err)
	}

	if exists {
		log.Info("networking already setup")
		return nil
	}

	// Setup bridge and firewall rules
	commands := []struct {
		name string
		args []string
	}{
		{"ip", []string{"l", "add", bridgeName, "type", "bridge"}},
		{"ip", []string{"l", "set", bridgeName, "up"}},
		{"ip", []string{"a", "add", bridgeIP, "dev", bridgeName, "scope", "host"}},
		{"iptables", []string{"-t", "nat", "-A", "POSTROUTING", "-s", bridgeSubnet, "-o", hostDefaultNetworkInterface, "-j", "MASQUERADE"}},
		{"sysctl", []string{"-w", fmt.Sprintf("net.ipv4.conf.%s.forwarding=1", hostDefaultNetworkInterface)}},
		{"sysctl", []string{"-w", fmt.Sprintf("net.ipv4.conf.%s.forwarding=1", bridgeName)}},
		{"iptables", []string{"-t", "filter", "-I", "FORWARD", "-s", bridgeSubnet, "-j", "ACCEPT"}},
		{"iptables", []string{"-t", "filter", "-I", "FORWARD", "-d", bridgeSubnet, "-j", "ACCEPT"}},
	}

	for _, cmd := range commands {
		if err := exec.Command(cmd.name, cmd.args...).Run(); err != nil {
			return fmt.Errorf("failed to execute command '%s %s': %w", cmd.name, strings.Join(cmd.args, " "), err)
		}
	}

	return nil
}

func getVmStateDirPath(stateDir string, vmName string) string {
	return path.Join(stateDir, vmName)
}

func getVmSocketPath(vmStateDir string, vmName string) string {
	return path.Join(vmStateDir, vmName+".sock")
}

func unixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: time.Second * 30,
	}
}

func createApiClient(apiSocketPath string) *chvapi.APIClient {
	configuration := chvapi.NewConfiguration()
	configuration.HTTPClient = unixSocketClient(apiSocketPath)
	configuration.Servers = chvapi.ServerConfigurations{
		{
			URL: "http://localhost/api/v1",
		},
	}
	return chvapi.NewAPIClient(configuration)
}

func waitForServer(ctx context.Context, apiClient *chvapi.APIClient, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				resp, r, err := apiClient.DefaultAPI.VmmPingGet(ctx).Execute()
				if err == nil {
					log.WithFields(log.Fields{
						"buildVersion": *resp.BuildVersion,
						"statusCode":   r.StatusCode,
					}).Info("cloud-hypervisor server up")
					errCh <- nil
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	return <-errCh
}

func reapProcess(process *os.Process, logger *log.Entry, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		log.Info("waiting for VM process to exit")
		_, err := process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		logger.Infof("VM process exited via wait")
		return err
	case <-time.After(timeout):
		logger.Warnf("Timeout waiting for VM process to exit")
	}

	// Attempt to kill the process if it's still running. This should also
	// trigger the wait in the goroutine preventing it's leak.
	err := process.Kill()
	if err != nil {
		return fmt.Errorf("failed to kill VM process: %v", err)
	}
	return fmt.Errorf("VM process was force killed after timeout")
}

// convertPortForward converts the port forwards from the config to the API format.
func convertPortForward(pfs []portForward) []serverapi.PortForward {
	result := make([]serverapi.PortForward, 0, len(pfs))
	for _, pf := range pfs {
		result = append(result, serverapi.PortForward{
			HostPort:    serverapi.PtrString(strconv.Itoa(int(pf.hostPort))),
			GuestPort:   serverapi.PtrString(strconv.Itoa(int(pf.guestPort))),
			Description: serverapi.PtrString(pf.description),
		})
	}
	return result
}

type NetworkConfig struct {
	Tap string `json:"tap"`
}

type PayloadConfig struct {
	Firmware  *string `json:"firmware"`
	Kernel    *string `json:"kernel"`
	Cmdline   *string `json:"cmdline"`
	Initramfs *string `json:"initramfs"`
}

type VMConfig struct {
	Net     *[]NetworkConfig `json:"net"`
	Payload PayloadConfig    `json:"payload"`
}

func extractGuestIPFromCmdline(cmdline string) (*net.IPNet, error) {
	// Look for guest_ip="<ip>" in the cmdline.
	re := regexp.MustCompile(`guest_ip="([^"]+)"`)
	matches := re.FindStringSubmatch(cmdline)
	if len(matches) < 2 {
		return nil, fmt.Errorf("guest_ip not found in cmdline")
	}

	// matches[1] contains the IP address with CIDR.
	ip, ipNet, err := net.ParseCIDR(matches[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse CIDR %q: %w", matches[1], err)
	}
	ipNet.IP = ip
	return ipNet, nil
}

// Returns the tap device name and the guest IP address from the snapshot config.
func parseNetworkDataFromSnapshotConfig(configPath string) (string, *net.IPNet, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config VMConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if config.Net == nil || len(*config.Net) == 0 {
		return "", nil, fmt.Errorf("no network configuration found")
	}

	if config.Payload.Cmdline == nil {
		return "", nil, fmt.Errorf("no cmdline found")
	}

	guestIP, err := extractGuestIPFromCmdline(*config.Payload.Cmdline)
	if err != nil {
		return "", nil, fmt.Errorf("failed to extract guest IP from cmdline: %w", err)
	}
	return (*config.Net)[0].Tap, guestIP, nil
}

// getIPPrefix returns the IP prefix from the given CIDR taking into account the mask.
func getIPPrefix(cidr string) (string, error) {
	// Parse CIDR
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("failed to parse CIDR: %w", err)
	}

	// Get the ones in the mask to determine how many octets to keep
	ones, _ := ipNet.Mask.Size()

	// Calculate how many complete octets we need
	completeOctets := ones / 8

	// Split IP into octets
	octets := strings.Split(ipNet.IP.String(), ".")

	// Take only the number of octets determined by the mask
	if completeOctets > 0 && completeOctets <= len(octets) {
		return strings.Join(octets[:completeOctets], "."), nil
	}

	return "", fmt.Errorf("invalid mask size: %d", ones)
}

func createStatefulDisk(path string, sizeInMB int32) error {
	log.Infof("Creating stateful disk at %s with size %dMB", path, sizeInMB)
	// A sparse file is created as we want to pack as many sandboxes on a server, by growing as
	// needed.
	cmd := exec.Command(
		"truncate",
		"-s",
		fmt.Sprintf("%dM", sizeInMB),
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create stateful disk: %w out: %s", err, string(out))
	}

	cmd = exec.Command("mkfs.ext4", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to format stateful disk with ext4: %w out: %s", err, string(out))
	}
	return nil
}

func NewServer(config config.ServerConfig) (*Server, error) {
	// Cleanup any existing resources.
	if err := cleanupTapDevices(); err != nil {
		return nil, fmt.Errorf("failed to cleanup tap devices: %w", err)
	}

	if err := cleanupBridge(); err != nil {
		return nil, fmt.Errorf("failed to cleanup bridge: %w", err)
	}

	ipPrefix, err := getIPPrefix(config.BridgeSubnet)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP prefix: %w", err)
	}

	log.Infof("Cleaning up iptables rules for IP prefix: %s", ipPrefix)
	if err := cleanupAllIPTablesRulesForIP(ipPrefix); err != nil {
		return nil, fmt.Errorf("failed to cleanup iptables rules: %w", err)
	}

	if err := os.MkdirAll(config.StateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create vm state dir: %v err: %w", config.StateDir, err)
	}

	ipBackupFile := fmt.Sprintf("/tmp/iptables-backup-%s.rules", time.Now().Format(time.UnixDate))
	if err := setupBridgeAndFirewall(
		ipBackupFile,
		config.BridgeName,
		config.BridgeIP,
		config.BridgeSubnet,
	); err != nil {
		return nil, fmt.Errorf("failed to setup networking on the host: %w", err)
	}

	ipAllocator, err := ipallocator.NewIPAllocator(config.BridgeSubnet)
	if err != nil {
		return nil, fmt.Errorf("failed to create ip allocator: %w", err)
	}

	portAllocator, err := portallocator.NewPortAllocator(
		portAllocatorLowPort,
		portAllocatorHighPort,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create port allocator: %w", err)
	}

	cidAllocator, err := cidallocator.NewCIDAllocator(cidAllocatorLow, cidAllocatorHigh)
	if err != nil {
		return nil, fmt.Errorf("failed to create CID allocator: %w", err)
	}

	log.Infof("Server config: %+v", config)
	return &Server{
		vms:           make(map[string]*vm),
		fountain:      fountain.NewFountain(config.BridgeName),
		ipAllocator:   ipAllocator,
		portAllocator: portAllocator,
		cidAllocator:  cidAllocator,
		config:        config,
	}, nil
}

func (s *Server) getVMAtomic(vmName string) *vm {
	s.lock.RLock()
	defer s.lock.RUnlock()

	vm, exists := s.vms[vmName]
	if !exists {
		return nil
	}
	return vm
}

func (s *Server) createVM(
	ctx context.Context,
	vmName string,
	kernelPath string,
	initramfsPath string,
	rootfsPath string,
	forRestore bool,
) (*vm, error) {
	cleanup := cleanup.Make(func() {
		log.WithFields(
			log.Fields{
				"vmname": vmName,
				"action": "cleanup",
				"api":    "createVM",
			},
		).Info("clean up done")
	})

	defer func() {
		// Won't do anything if no error since we call `Release` it at the end.
		cleanup.Clean()
	}()

	vmStateDir := getVmStateDirPath(s.config.StateDir, vmName)
	err := os.MkdirAll(vmStateDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create vm state dir: %w", err)
	}
	cleanup.Add(func() {
		if err := os.RemoveAll(vmStateDir); err != nil {
			log.WithError(err).Errorf("failed to remove vm state dir: %s", vmStateDir)
		}
	})
	log.Infof("CREATED: %v", vmStateDir)

	// This will be cleaned up by the clean up function above nuking the directory.
	apiSocketPath := getVmSocketPath(vmStateDir, vmName)
	apiClient := createApiClient(apiSocketPath)

	// This will be cleaned up by the clean up function above nuking the directory.
	logFilePath := path.Join(vmStateDir, "log")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	cmd := exec.Command(s.config.ChvBinPath, "--api-socket", apiSocketPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Add VMs to a separate process group. Otherwise Ctrl-C goes to the VMs
	// without us handling it. Now we can handle it and gracefully shut down
	// each VM.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("error spawning vm: %w", err)
	}
	cleanup.Add(func() {
		log.WithFields(log.Fields{"vmname": vmName, "action": "cleanup", "api": "createVM"}).Info("reap VMM process")
		reapProcess(cmd.Process, log.WithField("vmname", vmName), reapVmTimeout)
	})

	err = waitForServer(ctx, apiClient, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error waiting for vm: %w", err)
	}
	cleanup.Add(func() {
		log.WithFields(log.Fields{"vmname": vmName, "action": "cleanup", "api": "createVM"}).Info("kill VMM process")
		if err := cmd.Process.Kill(); err != nil {
			log.WithField("vmname", vmName).Errorf("Error killing vm: %v", err)
		}
	})
	log.WithField("vmname", vmName).Infof("VM started Pid:%d", cmd.Process.Pid)

	var guestIP *net.IPNet
	var tapDevice *fountain.TapDevice
	var portForwards []portForward
	var vsockPath string
	var cid uint32
	var statefulDiskPath string
	// We only need to setup the network and call the chv create VM API if we are not restoring
	// from a snapshot.
	if !forRestore {
		var err error
		tapDevice, err = s.fountain.CreateTapDevice()
		if err != nil {
			return nil, fmt.Errorf("failed to create tap device: %w", err)
		}
		cleanup.Add(func() {
			if err := s.fountain.DestroyTapDevice(tapDevice); err != nil {
				log.WithError(err).Errorf("failed to delete tap device: %s", tapDevice)
			}
		})

		guestIP, err = s.ipAllocator.AllocateIP()
		if err != nil {
			return nil, fmt.Errorf("error allocating guest ip: %w", err)
		}
		log.Infof("Allocated IP: %v", guestIP)
		cleanup.Add(func() {
			log.WithFields(log.Fields{"vmname": vmName, "action": "cleanup", "api": "createVM", "ip": guestIP.String()}).Info("freeing IP")
			s.ipAllocator.FreeIP(guestIP.IP)
		})

		portForwards, err = s.setupPortForwardsToVM(guestIP.IP.String(), s.config.PortForwards)
		if err != nil {
			cleanupAllIPTablesRulesForIP(guestIP.IP.String())
			return nil, fmt.Errorf("failed to forward ports to VM: %w", err)
		}
		cleanup.Add(func() {
			log.WithFields(
				log.Fields{
					"vmname": vmName,
					"action": "cleanup",
					"api":    "createVM",
					"ip":     guestIP.String(),
				},
			).Info("deleting port forwards")
			cleanupAllIPTablesRulesForIP(guestIP.IP.String())
		})

		vsockPath = path.Join(vmStateDir, "vsock.sock")
		cid, err = s.cidAllocator.AllocateCID()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate CID: %w", err)
		}
		cleanup.Add(func() {
			if err := s.cidAllocator.FreeCID(cid); err != nil {
				log.WithError(err).Errorf("failed to free CID: %d", cid)
			}
		})

		statefulDiskPath = path.Join(vmStateDir, statefulDiskFilename)
		err = createStatefulDisk(statefulDiskPath, s.config.StatefulSizeInMB)
		if err != nil {
			return nil, fmt.Errorf("failed to create stateful disk: %w", err)
		}
		cleanup.Add(func() {
			if err := os.Remove(statefulDiskPath); err != nil {
				log.WithError(err).Errorf("failed to remove stateful disk: %s", statefulDiskPath)
			}
		})

		vcpus := calculateVCPUCount()
		// Match virtio-blk queues to vCPUs.
		numBlockDeviceQueues := vcpus
		memorySizeMB, err := calculateGuestMemorySizeInMB(s.config.GuestMemPercentage)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate guest memory size: %w", err)
		}
		log.Infof("Calculated vCPUs: %d, memory size: %d MB", vcpus, memorySizeMB)
		vmConfig := chvapi.VmConfig{
			Payload: chvapi.PayloadConfig{
				Kernel:    String(kernelPath),
				Cmdline:   String(getKernelCmdLine(s.config.BridgeIP, guestIP.String())),
				Initramfs: String(initramfsPath),
			},
			Disks: []chvapi.DiskConfig{
				{Path: rootfsPath, Readonly: Bool(true), NumQueues: &numBlockDeviceQueues},
				{Path: statefulDiskPath, NumQueues: &numBlockDeviceQueues},
			},
			Cpus:    &chvapi.CpusConfig{BootVcpus: vcpus, MaxVcpus: vcpus},
			Memory:  &chvapi.MemoryConfig{Size: int64(memorySizeMB * 1024 * 1024)},
			Serial:  chvapi.NewConsoleConfig(serialPortMode),
			Console: chvapi.NewConsoleConfig(consolePortMode),
			Net: []chvapi.NetConfig{
				{Tap: String(tapDevice.Name), NumQueues: Int32(numNetDeviceQueues), QueueSize: Int32(netDeviceQueueSizeBytes), Id: String(netDeviceId)},
			},
			Vsock: &chvapi.VsockConfig{Cid: int64(cid), Socket: vsockPath},
		}
		log.Info("Calling CreateVM")
		req := apiClient.DefaultAPI.CreateVM(ctx)
		req = req.VmConfig(vmConfig)

		resp, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to start VM: %w", err)
		}
		if resp.StatusCode != 204 {
			return nil, fmt.Errorf("failed to start VM. bad status: %v", resp)
		}
	}

	vm := &vm{
		name:             vmName,
		stateDirPath:     vmStateDir,
		apiSocketPath:    apiSocketPath,
		apiClient:        apiClient,
		process:          cmd.Process,
		ip:               guestIP,
		tapDevice:        tapDevice,
		status:           vmStatusRunning,
		portForwards:     portForwards,
		vsockPath:        vsockPath,
		cid:              cid,
		statefulDiskPath: statefulDiskPath,
	}
	log.Infof("Successfully created VM: %s", vmName)

	s.lock.Lock()
	s.vms[vmName] = vm
	s.lock.Unlock()

	cleanup.Release()
	return vm, nil
}

func (v *vm) boot(
	ctx context.Context,
) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	resp, err := v.apiClient.DefaultAPI.BootVM(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to boot VM resp.Body: %v: %w", resp.Body, err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to boot VM. bad status: %v", resp)
	}

	log.Infof("Successfully booted VM: %s", v.name)
	v.status = vmStatusRunning
	return nil
}

func (v *vm) resume(
	ctx context.Context,
) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	resp, err := v.apiClient.DefaultAPI.ResumeVM(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to resume VM resp.Body: %v: %w", resp.Body, err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to resume VM. bad status: %v", resp)
	}

	log.Infof("Successfully resumed VM: %s", v.name)
	v.status = vmStatusRunning
	return nil
}

func (v *vm) restore(
	ctx context.Context,
	snapshotPath string,
) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	// The snapshot path is a "file://" URL.
	req := v.apiClient.DefaultAPI.VmRestorePut(ctx)
	req = req.RestoreConfig(chvapi.RestoreConfig{
		SourceUrl: fmt.Sprintf("file://%s", snapshotPath),
	})

	resp, err := req.Execute()
	if err != nil {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to restore from snapshot: %d: %s: %w", resp.StatusCode, string(body), err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to restore from snapshot. bad status: %v", resp)
	}
	return nil
}

func (v *vm) destroy(
	ctx context.Context,
) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	logger := log.WithField("vmName", v.name)

	// Shutdown for a graceful exit before full deletion. Don't error out if this fails as we still
	// want to try a deletion after this.
	shutdownReq := v.apiClient.DefaultAPI.ShutdownVM(ctx)
	resp, err := shutdownReq.Execute()
	if err != nil {
		logger.Warnf("failed to shutdown VM before deleting: %v", err)
	} else if resp.StatusCode >= 300 {
		logger.Warnf("failed to shutdown VM before deleting. bad status: %v", resp)
	}

	deleteReq := v.apiClient.DefaultAPI.DeleteVM(ctx)
	resp, err = deleteReq.Execute()
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to delete VM: %v", err))
	}

	if resp.StatusCode >= 300 {
		return status.Error(codes.Internal, fmt.Sprintf("failed to stop VM. bad status: %v", resp))
	}

	shutdownVMMReq := v.apiClient.DefaultAPI.ShutdownVMM(ctx)
	resp, err = shutdownVMMReq.Execute()
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to shutdown VMM: %v", err))
	}

	if resp.StatusCode >= 300 {
		return status.Error(codes.Internal, fmt.Sprintf("failed to shutdown VMM. bad status: %v", resp))
	}

	// At this point `v.process` is guaranteed to be non-nil.
	err = reapProcess(v.process, logger, reapVmTimeout)
	if err != nil {
		logger.Warnf("failed to reap VM process: %v", err)
	}

	// This should be done at the very end in case we need to communicate with the VM during cleanup.
	log.Infof("Deleting iptables rules for IP: %s", v.ip.String())
	err = cleanupAllIPTablesRulesForIP(v.ip.IP.String())
	if err != nil {
		logger.Warnf("failed to delete iptables rules: %v", err)
	}

	// Once deleted remove its directory and remove it from the internal store of VMs.
	err = os.RemoveAll(v.stateDirPath)
	if err != nil {
		log.Warnf("Failed to delete directory %s: %v", v.stateDirPath, err)
	}
	return nil
}

func (v *vm) pause(
	ctx context.Context,
) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	resp, err := v.apiClient.DefaultAPI.PauseVM(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to pause VM resp.Body: %v: %w", resp.Body, err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to pause VM. bad status: %v", resp)
	}

	log.Infof("Successfully paused VM: %s", v.name)
	v.status = vmStatusPaused
	return nil
}

type Server struct {
	lock          sync.RWMutex
	vms           map[string]*vm
	fountain      *fountain.Fountain
	ipAllocator   *ipallocator.IPAllocator
	portAllocator *portallocator.PortAllocator
	cidAllocator  *cidallocator.CIDAllocator
	config        config.ServerConfig
}

func (s *Server) StartVM(ctx context.Context, req *serverapi.StartVMRequest) (*serverapi.StartVMResponse, error) {
	vmName := req.GetVmName()
	if vmName == "" {
		return nil, fmt.Errorf("vmName is required")
	}

	if snapshotPath := req.GetSnapshotPath(); snapshotPath != "" {
		vm, err := s.restoreVM(ctx, vmName, snapshotPath)
		if err != nil {
			return nil, fmt.Errorf("failed to restore VM from snapshot: %w", err)
		}

		return &serverapi.StartVMResponse{
			VmName:        serverapi.PtrString(vmName),
			Ip:            serverapi.PtrString(vm.ip.String()),
			Status:        serverapi.PtrString(vm.status.String()),
			TapDeviceName: serverapi.PtrString(vm.tapDevice.Name),
			PortForwards:  convertPortForward(vm.portForwards),
		}, nil
	}

	kernelPath := req.GetKernel()
	rootfsPath := req.GetRootfs()
	initramfsPath := req.GetInitramfs()
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to start VM")

	// If not specified, set kernel and rootfs to defaults.
	if kernelPath == "" {
		kernelPath = s.config.KernelPath
	}

	if rootfsPath == "" {
		rootfsPath = s.config.RootfsPath
	}

	if initramfsPath == "" {
		initramfsPath = s.config.InitramfsPath
	}

	vm := s.getVMAtomic(vmName)
	if vm != nil {
		err := vm.boot(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to boot existing VM: %v", err)
		}
	} else {
		cleanup := cleanup.Make(func() {
			logger.Info("start VM clean up done")
		})
		defer func() {
			// Won't do anything if no error since we call `Release` it at the end.
			cleanup.Clean()
		}()

		var err error
		vm, err = s.createVM(ctx, vmName, kernelPath, initramfsPath, rootfsPath, false)
		if err != nil {
			logger.Errorf("failed to create VM: %v", err)
			return nil, err
		}

		cleanup.Add(func() {
			logger.Info("shutting down VM")
			resp, err := vm.apiClient.DefaultAPI.ShutdownVM(ctx).Execute()
			if err != nil {
				logger.WithError(err).Errorf("failed to shutdown VM: %v", err)
			}

			if resp.StatusCode != 204 {
				logger.WithError(err).Errorf("failed to shutdown VM. bad status: %v", resp)
			}
		})

		err = vm.boot(ctx)
		if err != nil {
			logger.Errorf("failed to boot VM: %v", err)
			return nil, err
		}
		cleanup.Release()
	}

	logger.Infof("VM started")
	return &serverapi.StartVMResponse{
		VmName:        serverapi.PtrString(vmName),
		Ip:            serverapi.PtrString(vm.ip.String()),
		Status:        serverapi.PtrString(vm.status.String()),
		TapDeviceName: serverapi.PtrString(vm.tapDevice.Name),
		PortForwards:  convertPortForward(vm.portForwards),
	}, nil
}

func (s *Server) StopVM(ctx context.Context, req *serverapi.VMRequest) (*serverapi.VMResponse, error) {
	vmName := req.GetVmName()
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to stop VM")

	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Errorf(codes.NotFound, "vm %s not found", vmName)
	}

	shutdown_req := vm.apiClient.DefaultAPI.ShutdownVM(ctx)
	resp, err := shutdown_req.Execute()
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to stop VM: %v", err))
	}

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to stop VM. bad status: %v", resp))
	}

	vm.status = vmStatusStopped
	logger.Infof("VM stopped")
	return &serverapi.VMResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) destroyVM(ctx context.Context, vmName string) error {
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to destroy VM")
	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return fmt.Errorf("vm %s not found", vmName)
	}

	err := vm.destroy(ctx)
	if err != nil {
		return fmt.Errorf("failed to destroy vm: %s: %w", vmName, err)
	}

	err = s.fountain.DestroyTapDevice(vm.tapDevice)
	if err != nil {
		return fmt.Errorf("failed to destroy the tap device for vm: %s: %w", vmName, err)
	}

	err = s.ipAllocator.FreeIP(vm.ip.IP)
	if err != nil {
		return fmt.Errorf("failed to free IP: %s: %w", vm.ip.String(), err)
	}

	err = s.cidAllocator.FreeCID(vm.cid)
	if err != nil {
		log.WithError(err).Errorf("failed to free CID: %d", vm.cid)
	}

	s.lock.Lock()
	delete(s.vms, vmName)
	s.lock.Unlock()
	return nil
}

func (s *Server) DestroyVM(ctx context.Context, req *serverapi.VMRequest) (*serverapi.VMResponse, error) {
	vmName := req.GetVmName()
	err := s.destroyVM(ctx, vmName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to destroy vm: %s: %v", vmName, err)
	}

	return &serverapi.VMResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) DestroyAllVMs(ctx context.Context) (*serverapi.DestroyAllVMsResponse, error) {
	log.Infof("received request to destroy all VMs")

	// `destroyVM` grabs locks inside it. Hence easiest to just capture VM names before. If state is
	// changed concurrently before destroying then we will return an error as expected. However,
	// state will never be corrupted.
	s.lock.RLock()
	vmNames := make([]string, 0, len(s.vms))
	for name := range s.vms {
		vmNames = append(vmNames, name)
	}
	s.lock.RUnlock()

	var finalErr error
	for _, vmName := range vmNames {
		// Each invocation grabs the same lock on `s`. No point spawning a goroutine for each VM.
		err := s.destroyVM(ctx, vmName)
		if err != nil {
			log.Warnf("failed to destroy and clean up vm: %s", vmName)
		}
		finalErr = errors.Join(finalErr, err)
	}

	if finalErr != nil {
		return nil, status.Errorf(codes.Internal, "failed to destroy all VMs: %v", finalErr)
	}

	return &serverapi.DestroyAllVMsResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) ListAllVMs(ctx context.Context) (*serverapi.ListAllVMsResponse, error) {
	resp := &serverapi.ListAllVMsResponse{}
	var vms []serverapi.ListAllVMsResponseVmsInner

	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, vm := range s.vms {
		var ipString string
		if vm.ip != nil {
			ipString = vm.ip.String()
		}

		vmInfo := serverapi.ListAllVMsResponseVmsInner{
			VmName:        serverapi.PtrString(vm.name),
			Ip:            serverapi.PtrString(ipString),
			Status:        serverapi.PtrString(vm.status.String()),
			TapDeviceName: serverapi.PtrString(vm.tapDevice.Name),
			PortForwards:  convertPortForward(vm.portForwards),
		}
		vms = append(vms, vmInfo)
	}
	resp.Vms = vms
	return resp, nil
}

func (s *Server) ListVM(ctx context.Context, vmName string) (*serverapi.ListVMResponse, error) {
	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	var ipString string
	if vm.ip != nil {
		ipString = vm.ip.String()
	}

	return &serverapi.ListVMResponse{
		VmName:        serverapi.PtrString(vm.name),
		Ip:            serverapi.PtrString(ipString),
		Status:        serverapi.PtrString(vm.status.String()),
		TapDeviceName: serverapi.PtrString(vm.tapDevice.Name),
		PortForwards:  convertPortForward(vm.portForwards),
	}, nil
}

func (s *Server) SnapshotVM(ctx context.Context, req *serverapi.VMSnapshotRequest) (*serverapi.VMSnapshotResponse, error) {
	vmName := req.GetVmName()
	outputDir := req.GetOutputFile()
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to snapshot VM")

	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	// Create the output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.WithError(err).Error("failed to create snapshot directory")
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Pause the VM first as this is a prerequisite for taking a snapshot as per the CHV API spec.
	pauseReq := vm.apiClient.DefaultAPI.PauseVM(ctx)
	resp, err := pauseReq.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to pause VM: %w", err)
	}
	if resp.StatusCode != 204 {
		return nil, fmt.Errorf("failed to pause VM. bad status: %v", resp)
	}
	logger.Info("VM paused successfully")
	vm.status = vmStatusPaused

	// Ensure we resume the VM even if snapshot fails.
	defer func() {
		resumeReq := vm.apiClient.DefaultAPI.ResumeVM(ctx)
		resp, err := resumeReq.Execute()
		if err != nil {
			logger.Errorf("failed to resume VM: %v", err)
			return
		}
		if resp.StatusCode != 204 {
			logger.Errorf("failed to resume VM. bad status: %v", resp)
			return
		}
		logger.Info("VM resumed successfully")
		vm.status = vmStatusRunning
	}()

	// The API expects a "file://" URL.
	outputUrl := fmt.Sprintf("file://%s", outputDir)
	snapshotConfig := chvapi.VmSnapshotConfig{
		DestinationUrl: &outputUrl,
	}
	logger.WithField("destination", outputDir).Info("initiating VM snapshot")

	snapshotReq := vm.apiClient.DefaultAPI.VmSnapshotPut(ctx)
	snapshotReq = snapshotReq.VmSnapshotConfig(snapshotConfig)
	resp, err = snapshotReq.Execute()
	if err != nil {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create snapshot: %d: %s: %w", resp.StatusCode, string(body), err)
	}

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create snapshot: %d: %s", resp.StatusCode, string(body))
	}

	logger.WithFields(log.Fields{
		"destination": outputDir,
		"statusCode":  resp.StatusCode,
	}).Info("VM snapshot created successfully")
	return &serverapi.VMSnapshotResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) restoreVM(
	ctx context.Context,
	vmName string,
	snapshotPath string,
) (*vm, error) {
	logger := log.WithFields(log.Fields{
		"vmName":       vmName,
		"snapshotPath": snapshotPath,
	})
	logger.Info("received request to restore VM from snapshot")

	cleanup := cleanup.Make(func() {
		logger.Info("restore VM clean up done")
	})

	defer func() {
		// Won't do anything if no error since we call `Release` it at the end.
		cleanup.Clean()
	}()

	oldtapdeviceName, guestIP, err := parseNetworkDataFromSnapshotConfig(snapshotPath + "/config.json")
	if err != nil {
		return nil, fmt.Errorf("failed to get tap device from config: %w", err)
	}
	log.WithFields(log.Fields{
		"tapDevice": oldtapdeviceName,
		"guestIP":   guestIP.IP.String(),
	}).Info("parse network data from snapshot config")

	err = s.ipAllocator.ClaimIP(guestIP.IP)
	if err != nil {
		return nil, fmt.Errorf("failed to claim IP: %w", err)
	}

	newTapDevice, err := s.fountain.CreateTapDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to create tap device: %w", err)
	}
	cleanup.Add(func() {
		log.Errorf("TODO: destroy tap device: %s", newTapDevice.Name)
	})

	vm, err := s.createVM(ctx, vmName, "", "", "", true)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM for restore: %w", err)
	}
	vm.tapDevice = newTapDevice
	vm.ip = guestIP
	// From this point on we need to clean up the VM if the restore fails.
	cleanup.Add(func() {
		err := s.destroyVM(ctx, vmName)
		log.WithError(err).Errorf("failed to destroy VM during restore cleanup")
	})
	log.WithField("guestIP", guestIP.IP.String()).Info("restored VM")

	err = vm.restore(ctx, snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to restore VM: %w", err)
	}

	err = vm.resume(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resume VM: %w", err)
	}

	cleanup.Release()
	return vm, nil
}

func (s *Server) PauseVM(ctx context.Context, req *serverapi.VMRequest) (*serverapi.VMResponse, error) {
	vmName := req.GetVmName()
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to pause VM")

	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	err := vm.pause(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to pause VM: %v", err))
	}

	return &serverapi.VMResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) ResumeVM(ctx context.Context, req *serverapi.VMRequest) (*serverapi.VMResponse, error) {
	vmName := req.GetVmName()
	logger := log.WithField("vmName", vmName)
	logger.Infof("received request to resume VM")

	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	// We are only allowed to resume a paused VM. If this isn't the case the underlying VMM API will
	// return an error we return to the user. Thus, we don't check state here.
	err := vm.resume(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to resume VM: %v", err))
	}

	return &serverapi.VMResponse{
		Success: serverapi.PtrBool(true),
	}, nil
}

func (s *Server) VMCommand(ctx context.Context, vmName string, cmd string) (*serverapi.VmCommandResponse, error) {
	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	url := fmt.Sprintf("http://%s:4031", vm.ip.IP.String())
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return vm.handleRun(ctx, client, url, cmd)
}

func (s *Server) VMFileUpload(ctx context.Context, vmName string, files []serverapi.VmFileUploadRequestFilesInner) (*serverapi.VmFileUploadResponse, error) {
	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	url := fmt.Sprintf("http://%s:4031", vm.ip.IP.String())
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	reqBody := cmdserver.FilesPostRequest{
		Files: make([]cmdserver.FilePostData, len(files)),
	}

	for i, file := range files {
		reqBody.Files[i] = cmdserver.FilePostData{
			Path:    file.GetPath(),
			Content: file.GetContent(),
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/files", bytes.NewReader(body))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Internal, "request failed with status: %d", resp.StatusCode)
	}

	return &serverapi.VmFileUploadResponse{}, nil
}

func (v *vm) handleRun(ctx context.Context, client *http.Client, baseURL string, cmd string) (*serverapi.VmCommandResponse, error) {
	reqBody := struct {
		Cmd string `json:"cmd"`
	}{
		Cmd: cmd,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/cmd", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	var cmdResp cmdserver.RunCmdResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &serverapi.VmCommandResponse{
		Output: serverapi.PtrString(cmdResp.Output),
		Error:  serverapi.PtrString(cmdResp.Error),
	}, nil
}

func (s *Server) VMFileDownload(ctx context.Context, vmName string, paths string) (*serverapi.VmFileDownloadResponse, error) {
	vm := s.getVMAtomic(vmName)
	if vm == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("vm not found: %s", vmName))
	}

	url := fmt.Sprintf("http://%s:4031", vm.ip.IP.String())
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url+"/files?paths="+paths, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Internal, "request failed with status: %d", resp.StatusCode)
	}

	var cmdResp cmdserver.FilesGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to decode response: %v", err)
	}

	apiResp := &serverapi.VmFileDownloadResponse{
		Files: make([]serverapi.VmFileDownloadResponseFilesInner, len(cmdResp.Files)),
	}
	for i, file := range cmdResp.Files {
		apiResp.Files[i] = serverapi.VmFileDownloadResponseFilesInner{
			Path:    serverapi.PtrString(file.Path),
			Content: serverapi.PtrString(file.Content),
			Error:   serverapi.PtrString(file.Error),
		}
	}
	return apiResp, nil
}
