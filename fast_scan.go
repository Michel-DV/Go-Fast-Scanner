package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	Version        = "2.0.0"
	defaultPorts   = "1-1024"
	defaultWorkers = 100
	maxWorkers     = 1024
	defaultTimeout = 500 * time.Millisecond
)

type config struct {
	target  string
	ports   string
	workers int
	timeout time.Duration
	json    bool
	version bool
}

type openPort struct {
	Port    int    `json:"port"`
	Service string `json:"service,omitempty"`
}

type scanReport struct {
	Target       string     `json:"target"`
	PortsScanned int        `json:"ports_scanned"`
	Workers      int        `json:"workers"`
	TimeoutMS    int64      `json:"timeout_ms"`
	OpenPorts    []openPort `json:"open_ports"`
	DurationMS   int64      `json:"duration_ms"`
	Interrupted  bool       `json:"interrupted"`
}

type scanResult struct {
	port int
	open bool
}

var commonServices = map[int]string{
	21:   "ftp",
	22:   "ssh",
	23:   "telnet",
	25:   "smtp",
	53:   "domain",
	80:   "http",
	110:  "pop3",
	143:  "imap",
	443:  "https",
	445:  "smb",
	3306: "mysql",
	3389: "rdp",
	5432: "postgresql",
	6379: "redis",
	8080: "http-alt",
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cfg, err := parseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	if cfg.version {
		fmt.Printf("Go-Fast-Scanner v%s\n", Version)
		return 0
	}

	target, err := normalizeTarget(cfg.target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	ports, err := parsePorts(cfg.ports)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid ports: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !cfg.json {
		printScanHeader(target, cfg.ports, len(ports), cfg.workers, cfg.timeout)
	}

	started := time.Now()
	openPorts, scanned := scan(ctx, target, ports, cfg.workers, cfg.timeout)
	duration := time.Since(started)
	interrupted := ctx.Err() != nil

	report := scanReport{
		Target:       target,
		PortsScanned: scanned,
		Workers:      cfg.workers,
		TimeoutMS:    cfg.timeout.Milliseconds(),
		OpenPorts:    openPorts,
		DurationMS:   duration.Milliseconds(),
		Interrupted:  interrupted,
	}

	if cfg.json {
		if err := writeJSON(report); err != nil {
			fmt.Fprintf(os.Stderr, "error: could not encode JSON: %v\n", err)
			return 1
		}
	} else {
		printHumanReport(report, duration, len(ports))
	}

	if interrupted {
		fmt.Fprintln(os.Stderr, "[!] Scan interrupted")
		return 130
	}
	return 0
}

func parseFlags(args []string) (config, error) {
	cfg := config{}
	fs := flag.NewFlagSet("go-fast-scanner", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&cfg.target, "host", "", "target hostname or IP address")
	fs.StringVar(&cfg.ports, "ports", defaultPorts, "ports to scan (e.g. 22,80,443,8000-8100)")
	fs.IntVar(&cfg.workers, "workers", defaultWorkers, "number of concurrent workers")
	fs.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "per-connection timeout (e.g. 250ms, 1s)")
	fs.BoolVar(&cfg.json, "json", false, "emit machine-readable JSON")
	fs.BoolVar(&cfg.version, "version", false, "print version and exit")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Go-Fast-Scanner v%s - concurrent TCP connect scanner\n\n", Version)
		fmt.Fprintf(fs.Output(), "Usage:\n  %s -host <hostname|ip> [options]\n\nOptions:\n", fs.Name())
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output(), "\nExamples:")
		fmt.Fprintf(fs.Output(), "  %s -host 127.0.0.1\n", fs.Name())
		fmt.Fprintf(fs.Output(), "  %s -host localhost -ports 22,80,443,8000-8100 -workers 200 -timeout 300ms\n", fs.Name())
		fmt.Fprintf(fs.Output(), "  %s -host ::1 -ports 1-1024 -json\n", fs.Name())
	}

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if cfg.version {
		return cfg, nil
	}
	if strings.TrimSpace(cfg.target) == "" {
		return cfg, errors.New("-host is required")
	}
	if cfg.workers < 1 || cfg.workers > maxWorkers {
		return cfg, fmt.Errorf("-workers must be between 1 and %d", maxWorkers)
	}
	if cfg.timeout <= 0 {
		return cfg, errors.New("-timeout must be greater than zero")
	}
	return cfg, nil
}

func normalizeTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("target cannot be empty")
	}
	if strings.Contains(target, "://") {
		return "", errors.New("-host expects a hostname or IP address, not a URL")
	}
	if strings.ContainsAny(target, "/\\") {
		return "", errors.New("-host must not contain a path")
	}
	if strings.HasPrefix(target, "[") && strings.HasSuffix(target, "]") {
		target = strings.TrimSuffix(strings.TrimPrefix(target, "["), "]")
	}
	if strings.Count(target, ":") == 1 && net.ParseIP(target) == nil {
		return "", errors.New("-host must not include a port; use -ports instead")
	}
	return target, nil
}

func parsePorts(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, errors.New("port specification is empty")
	}

	unique := make(map[int]struct{})
	for _, token := range strings.Split(spec, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, errors.New("empty port entry")
		}

		if strings.Contains(token, "-") {
			if strings.Count(token, "-") != 1 {
				return nil, fmt.Errorf("malformed range %q", token)
			}
			parts := strings.SplitN(token, "-", 2)
			start, err := parsePort(parts[0])
			if err != nil {
				return nil, fmt.Errorf("range %q: %w", token, err)
			}
			end, err := parsePort(parts[1])
			if err != nil {
				return nil, fmt.Errorf("range %q: %w", token, err)
			}
			if start > end {
				return nil, fmt.Errorf("range %q is reversed", token)
			}
			for port := start; port <= end; port++ {
				unique[port] = struct{}{}
			}
			continue
		}

		port, err := parsePort(token)
		if err != nil {
			return nil, err
		}
		unique[port] = struct{}{}
	}

	ports := make([]int, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func parsePort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("port is empty")
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid port", value)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d is outside 1-65535", port)
	}
	return port, nil
}

func scan(ctx context.Context, target string, ports []int, workers int, timeout time.Duration) ([]openPort, int) {
	jobs := make(chan int)
	results := make(chan scanResult)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go scanWorker(ctx, target, timeout, jobs, results, &wg)
	}

	go func() {
		defer close(jobs)
		for _, port := range ports {
			select {
			case <-ctx.Done():
				return
			case jobs <- port:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	openPorts := make([]openPort, 0)
	scanned := 0
	for result := range results {
		scanned++
		if result.open {
			openPorts = append(openPorts, openPort{
				Port:    result.port,
				Service: serviceName(result.port),
			})
		}
	}

	sort.Slice(openPorts, func(i, j int) bool {
		return openPorts[i].Port < openPorts[j].Port
	})
	return openPorts, scanned
}

func scanWorker(
	ctx context.Context,
	target string,
	timeout time.Duration,
	jobs <-chan int,
	results chan<- scanResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	dialer := net.Dialer{Timeout: timeout}

	for {
		select {
		case <-ctx.Done():
			return
		case port, ok := <-jobs:
			if !ok {
				return
			}

			address := net.JoinHostPort(target, strconv.Itoa(port))
			conn, err := dialer.DialContext(ctx, "tcp", address)
			isOpen := err == nil
			if conn != nil {
				_ = conn.Close()
			}

			select {
			case results <- scanResult{port: port, open: isOpen}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func serviceName(port int) string {
	return commonServices[port]
}

func printScanHeader(target, portSpec string, portCount, workers int, timeout time.Duration) {
	fmt.Printf("[*] Target:  %s\n", target)
	fmt.Printf("[*] Ports:   %s (%d unique)\n", portSpec, portCount)
	fmt.Printf("[*] Workers: %d\n", workers)
	fmt.Printf("[*] Timeout: %s\n\n", timeout)
}

func printHumanReport(report scanReport, duration time.Duration, requested int) {
	for _, result := range report.OpenPorts {
		if result.Service != "" {
			fmt.Printf("[+] %d/tcp open  %s\n", result.Port, result.Service)
		} else {
			fmt.Printf("[+] %d/tcp open\n", result.Port)
		}
	}

	if len(report.OpenPorts) > 0 {
		fmt.Println()
	}
	fmt.Printf("[*] Completed in %s\n", duration.Round(time.Millisecond))
	fmt.Printf("[*] Ports scanned: %d/%d\n", report.PortsScanned, requested)
	fmt.Printf("[*] %d open port(s) found\n", len(report.OpenPorts))
}

func writeJSON(report scanReport) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
