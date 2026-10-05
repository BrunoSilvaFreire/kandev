package usage

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	antigravityProvider       = "antigravity"
	antigravityAccountLocator = "local-language-server"
	agyLanguageServerName     = "language_server"
	agyCSRFTokenFlag          = "--csrf_token"
	agyUserStatusPath         = "/exa.language_server_pb.LanguageServerService/GetUserStatus"
	agyRequestTimeout         = 3 * time.Second
	agySchemeHTTP             = "http"
	agySchemeHTTPS            = "https"
)

// ErrSourceNotRunning is returned by the Antigravity client when no running
// language server can be found. The page renders it as an unavailable source
// rather than a page error.
var ErrSourceNotRunning = errors.New("antigravity language server is not running")

// AntigravityCacheKey returns the live cache key for the local Antigravity
// language-server credential. It matches the key registered by the usage
// adapter so live fetches and recorded history share one account.
func AntigravityCacheKey() string {
	return CacheKey(antigravityProvider, antigravityAccountLocator)
}

// agyProcess is one discovered language server: its pid and CSRF token.
type agyProcess struct {
	pid  int
	csrf string
}

// AntigravityUsageClient reads utilization from the running Antigravity
// language server over loopback. Discovery and HTTP are injectable so tests
// never need a real process; the CSRF token is never logged.
type AntigravityUsageClient struct {
	listProcesses func() ([]agyProcess, error)
	listenPorts   func(pid int) ([]int, error)
	httpDo        func(ctx context.Context, method, url string, headers map[string]string, body []byte, insecureTLS bool) (int, []byte, error)
}

// NewAntigravityUsageClient creates a client wired to the real host probes.
func NewAntigravityUsageClient() *AntigravityUsageClient {
	return &AntigravityUsageClient{
		listProcesses: defaultAntigravityProcesses,
		listenPorts:   defaultListenPorts,
		httpDo:        defaultAntigravityHTTPDo,
	}
}

// FetchUsage implements ProviderUsageClient.
func (c *AntigravityUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	procs, err := c.listProcesses()
	if err != nil {
		return nil, fmt.Errorf("antigravity usage: discover: %w", err)
	}
	if len(procs) == 0 {
		return nil, ErrSourceNotRunning
	}
	var lastErr error
	for _, proc := range procs {
		usage, err := c.fetchFromProcess(ctx, proc)
		if err == nil {
			return usage, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, ErrSourceNotRunning
	}
	return nil, lastErr
}

func (c *AntigravityUsageClient) fetchFromProcess(ctx context.Context, proc agyProcess) (*ProviderUsage, error) {
	ports, err := c.listenPorts(proc.pid)
	if err != nil {
		return nil, fmt.Errorf("antigravity usage: enumerate ports: %w", err)
	}
	var lastErr error
	for _, port := range ports {
		for _, scheme := range []string{agySchemeHTTP, agySchemeHTTPS} {
			usage, err := c.fetchFromPort(ctx, scheme, port, proc.csrf)
			if err == nil {
				return usage, nil
			}
			lastErr = err
		}
	}
	return nil, lastErr
}

func (c *AntigravityUsageClient) fetchFromPort(ctx context.Context, scheme string, port int, csrf string) (*ProviderUsage, error) {
	url := fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, port, agyUserStatusPath)
	headers := map[string]string{
		"Content-Type":             "application/json",
		"Connect-Protocol-Version": "1",
		"X-Codeium-Csrf-Token":     csrf,
	}
	status, body, err := c.httpDo(ctx, http.MethodPost, url, headers, []byte("{}"), scheme == agySchemeHTTPS)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("antigravity usage: status %d", status)
	}
	return parseAntigravityStatus(body)
}

type agyQuotaInfo struct {
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
}

type agyModelConfig struct {
	Label       string        `json:"label"`
	QuotaInfo   *agyQuotaInfo `json:"quotaInfo"`
	IsExhausted bool          `json:"isExhausted"`
}

type agyUserStatusResponse struct {
	UserStatus struct {
		CascadeModelConfigData struct {
			ClientModelConfigs []agyModelConfig `json:"clientModelConfigs"`
		} `json:"cascadeModelConfigData"`
		PlanStatus struct {
			PlanInfo struct {
				TeamsTier string `json:"teamsTier"`
			} `json:"planInfo"`
		} `json:"planStatus"`
	} `json:"userStatus"`
}

// parseAntigravityStatus is pure so tests use a trimmed fixture.
func parseAntigravityStatus(body []byte) (*ProviderUsage, error) {
	var raw agyUserStatusResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("antigravity usage: decode: %w", err)
	}
	return &ProviderUsage{
		Provider:  antigravityProvider,
		Plan:      raw.UserStatus.PlanStatus.PlanInfo.TeamsTier,
		Windows:   antigravityWindows(raw.UserStatus.CascadeModelConfigData.ClientModelConfigs),
		FetchedAt: time.Now(),
	}, nil
}

// antigravityGroupLabel collapses model labels into the quota groups the
// provider tracks. Anything unrecognized keeps its own label.
func antigravityGroupLabel(label string) string {
	switch {
	case strings.HasPrefix(label, "Claude"):
		return "Claude models"
	case strings.HasPrefix(label, "Gemini") && strings.Contains(label, "Pro"):
		return "Gemini Pro"
	case strings.HasPrefix(label, "Gemini") && strings.Contains(label, "Flash"):
		return "Gemini Flash"
	case strings.HasPrefix(label, "GPT-OSS"):
		return "GPT-OSS"
	default:
		return label
	}
}

// agyFraction returns the group's remaining fraction, treating an exhausted
// entry with no explicit fraction as zero.
func agyFraction(c agyModelConfig) *float64 {
	if c.QuotaInfo != nil && c.QuotaInfo.RemainingFraction != nil {
		f := *c.QuotaInfo.RemainingFraction
		return &f
	}
	if c.IsExhausted {
		zero := 0.0
		return &zero
	}
	return nil
}

// antigravityWindows is pure: one window per quota group, utilization from the
// group's lowest remaining fraction and reset from the group's earliest reset.
func antigravityWindows(configs []agyModelConfig) []UtilizationWindow {
	type group struct {
		min   *float64
		reset time.Time
	}
	groups := make(map[string]*group, len(configs))
	order := make([]string, 0, len(configs))
	for _, cfg := range configs {
		label := antigravityGroupLabel(cfg.Label)
		g, ok := groups[label]
		if !ok {
			g = &group{}
			groups[label] = g
			order = append(order, label)
		}
		if frac := agyFraction(cfg); frac != nil && (g.min == nil || *frac < *g.min) {
			g.min = frac
		}
		if cfg.QuotaInfo != nil && cfg.QuotaInfo.ResetTime != "" {
			if t, err := time.Parse(time.RFC3339, cfg.QuotaInfo.ResetTime); err == nil {
				if g.reset.IsZero() || t.Before(g.reset) {
					g.reset = t
				}
			}
		}
	}
	windows := make([]UtilizationWindow, 0, len(order))
	for _, label := range order {
		g := groups[label]
		// A group with no fraction data has nothing authoritative to report;
		// emitting it as 0% would read as healthy rather than unknown.
		if g.min == nil {
			continue
		}
		windows = append(windows, UtilizationWindow{
			Label:          label,
			UtilizationPct: 100 * (1 - *g.min),
			ResetAt:        g.reset,
		})
	}
	return windows
}

// defaultAntigravityHTTPDo performs one loopback request. Redirects are never
// followed and TLS verification is skipped only for the 127.0.0.1 address the
// caller constructed.
func defaultAntigravityHTTPDo(ctx context.Context, method, url string, headers map[string]string, body []byte, insecureTLS bool) (int, []byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // loopback only
	}
	client := &http.Client{
		Timeout:   agyRequestTimeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func defaultAntigravityProcesses() ([]agyProcess, error) {
	switch runtime.GOOS {
	case "linux":
		return linuxAntigravityProcesses()
	case "darwin":
		return darwinAntigravityProcesses()
	default:
		return nil, nil
	}
}

func defaultListenPorts(pid int) ([]int, error) {
	switch runtime.GOOS {
	case "linux":
		return linuxListenPorts(pid)
	case "darwin":
		return darwinListenPorts(pid)
	default:
		return nil, nil
	}
}

func isAntigravityLanguageServer(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if !strings.Contains(filepath.Base(args[0]), agyLanguageServerName) {
		return false
	}
	if flagValue(args, agyCSRFTokenFlag) == "" {
		return false
	}
	for _, arg := range args {
		if strings.Contains(arg, "antigravity") {
			return true
		}
	}
	return false
}

// flagValue returns the value of a "--flag value" or "--flag=value" argument.
func flagValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, flag+"=") {
			return strings.TrimPrefix(arg, flag+"=")
		}
	}
	return ""
}

func linuxAntigravityProcesses() ([]agyProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var procs []agyProcess
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		args := readCmdline(pid)
		if !isAntigravityLanguageServer(args) {
			continue
		}
		procs = append(procs, agyProcess{pid: pid, csrf: flagValue(args, agyCSRFTokenFlag)})
	}
	return procs, nil
}

func readCmdline(pid int) []string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return nil
	}
	parts := strings.Split(string(data), "\x00")
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			args = append(args, part)
		}
	}
	return args
}

func darwinAntigravityProcesses() ([]agyProcess, error) {
	out, err := exec.Command("ps", "-axo", "pid=,args=").Output()
	if err != nil {
		return nil, err
	}
	var procs []agyProcess
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		args := strings.Fields(fields[1])
		if !isAntigravityLanguageServer(args) {
			continue
		}
		procs = append(procs, agyProcess{pid: pid, csrf: flagValue(args, agyCSRFTokenFlag)})
	}
	return procs, nil
}

func linuxListenPorts(pid int) ([]int, error) {
	inodes, err := linuxSocketInodes(pid)
	if err != nil {
		return nil, err
	}
	ports := make([]int, 0, len(inodes))
	seen := make(map[int]bool, len(inodes))
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		tablePorts, err := listenPortsInTable(table, inodes)
		if err != nil {
			return nil, err
		}
		for _, port := range tablePorts {
			if !seen[port] {
				seen[port] = true
				ports = append(ports, port)
			}
		}
	}
	return ports, nil
}

func linuxSocketInodes(pid int) (map[string]bool, error) {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return nil, err
	}
	inodes := make(map[string]bool)
	for _, entry := range entries {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, entry.Name()))
		if err != nil {
			continue
		}
		if inode, ok := socketInode(target); ok {
			inodes[inode] = true
		}
	}
	return inodes, nil
}

func socketInode(link string) (string, bool) {
	if !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), true
}

// listenPortsInTable returns listening ports whose socket inode is in the
// wanted set. Fields are: index, local_address, rem_address, state, ...,
// inode (index 9).
func listenPortsInTable(path string, wanted map[string]bool) ([]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ports []int
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		if !wanted[fields[9]] {
			continue
		}
		colon := strings.LastIndex(fields[1], ":")
		if colon < 0 {
			continue
		}
		port, err := strconv.ParseInt(fields[1][colon+1:], 16, 32)
		if err != nil {
			continue
		}
		ports = append(ports, int(port))
	}
	return ports, nil
}

func darwinListenPorts(pid int) ([]int, error) {
	out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil, nil
	}
	var ports []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[len(fields)-1] != "(LISTEN)" {
			continue
		}
		name := fields[len(fields)-2]
		colon := strings.LastIndex(name, ":")
		if colon < 0 {
			continue
		}
		port, err := strconv.Atoi(name[colon+1:])
		if err != nil {
			continue
		}
		ports = append(ports, port)
	}
	return ports, nil
}
