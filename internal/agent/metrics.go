package agent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type SystemMetrics struct {
	CPUUsage     float64
	RAMUsage     float64
	DiskUsage    float64
	NetworkRX    int64
	NetworkTX    int64
	TotalRAM     int64
	TotalDisk    int64
	CPUModel     string
	CPUCores     int
	OSVersion    string
	KernelVersion string
	DockerStatus  string
	NomadStatus   string
	JudgeStatus   string
	RunningJobs   int
	Hostname      string
	Uptime        int64
}

func CollectMetrics() *SystemMetrics {
	m := &SystemMetrics{
		CPUCores: runtime.NumCPU(),
	}
	m.Hostname, _ = os.Hostname()
	m.CPUUsage = readCPUUsage()
	m.RAMUsage, m.TotalRAM = readRAMUsage()
	m.DiskUsage, m.TotalDisk = readDiskUsage()
	m.NetworkRX, m.NetworkTX = readNetworkStats()
	m.CPUModel = readCPUModel()
	m.KernelVersion = readKernelVersion()
	m.OSVersion = readOSVersion()
	m.Uptime = readUptime()
	m.DockerStatus = serviceStatus("docker")
	m.NomadStatus = serviceStatus("nomad")
	m.JudgeStatus = judgeStatus()
	m.RunningJobs = countRunningContainers()
	return m
}

func readCPUUsage() float64 {
	read := func() (idle, total uint64) {
		f, err := os.Open("/proc/stat")
		if err != nil {
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "cpu ") {
				continue
			}
			fields := strings.Fields(line)
			for i, v := range fields[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				total += n
				if i == 3 {
					idle = n
				}
			}
			return
		}
		return
	}
	idle1, total1 := read()
	time.Sleep(200 * time.Millisecond)
	idle2, total2 := read()
	idleDelta := idle2 - idle1
	totalDelta := total2 - total1
	if totalDelta == 0 {
		return 0
	}
	return (1.0 - float64(idleDelta)/float64(totalDelta)) * 100
}

func readRAMUsage() (usage float64, total int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	var memTotal, memAvailable int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			memTotal = val * 1024
		case "MemAvailable:":
			memAvailable = val * 1024
		}
	}
	if memTotal == 0 {
		return
	}
	total = memTotal
	usage = float64(memTotal-memAvailable) / float64(memTotal) * 100
	return
}

func readDiskUsage() (usage float64, total int64) {
	out, err := exec.Command("df", "-B1", "/").Output()
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return
	}
	size, _ := strconv.ParseInt(fields[1], 10, 64)
	used, _ := strconv.ParseInt(fields[2], 10, 64)
	total = size
	if size > 0 {
		usage = float64(used) / float64(size) * 100
	}
	return
}

func readNetworkStats() (rx, tx int64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "lo:") || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseInt(fields[0], 10, 64)
		t, _ := strconv.ParseInt(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return
}

func readCPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func readKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readOSVersion() string {
	out, err := exec.Command("lsb_release", "-ds").Output()
	if err != nil {
		out2, err2 := os.ReadFile("/etc/os-release")
		if err2 != nil {
			return ""
		}
		for _, line := range strings.Split(string(out2), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readUptime() int64 {
	out, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fields[0], 64)
	return int64(f)
}

func serviceStatus(name string) string {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err != nil {
		return "stopped"
	}
	s := strings.TrimSpace(string(out))
	if s == "active" {
		return "running"
	}
	return s
}

func judgeStatus() string {
	out, err := exec.Command("docker", "ps", "--filter", "name=judge", "--format", "{{.Status}}").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "stopped"
	}
	return "running"
}

func countRunningContainers() int {
	out, err := exec.Command("docker", "ps", "-q").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

func prometheusMetrics(m *SystemMetrics) string {
	return fmt.Sprintf(`# HELP worker_cpu_usage_percent CPU usage percentage
# TYPE worker_cpu_usage_percent gauge
worker_cpu_usage_percent %.2f

# HELP worker_ram_usage_percent RAM usage percentage
# TYPE worker_ram_usage_percent gauge
worker_ram_usage_percent %.2f

# HELP worker_disk_usage_percent Disk usage percentage
# TYPE worker_disk_usage_percent gauge
worker_disk_usage_percent %.2f

# HELP worker_running_jobs Number of running jobs
# TYPE worker_running_jobs gauge
worker_running_jobs %d

# HELP worker_network_rx_bytes Network received bytes
# TYPE worker_network_rx_bytes counter
worker_network_rx_bytes %d

# HELP worker_network_tx_bytes Network transmitted bytes
# TYPE worker_network_tx_bytes counter
worker_network_tx_bytes %d
`,
		m.CPUUsage, m.RAMUsage, m.DiskUsage,
		m.RunningJobs, m.NetworkRX, m.NetworkTX,
	)
}
