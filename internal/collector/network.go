package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// NetworkCollector collects network-related diagnostic data
type NetworkCollector struct{}

// NewNetworkCollector creates a new network collector
func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{}
}

// Name returns the collector's unique identifier
func (n *NetworkCollector) Name() string {
	return "network"
}

// Collect performs network data collection
func (n *NetworkCollector) Collect(ctx context.Context, params map[string]interface{}) (*Result, error) {
	data := &Data{
		Type:      "network",
		Timestamp: time.Now(),
		Source:    "node-local",
		Data:      make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}

	// Collect network interfaces
	if interfaces, err := n.collectNetworkInterfaces(); err == nil {
		data.Data["interfaces"] = interfaces
	}

	// Collect TCP statistics
	if tcpStats, err := n.collectTCPStats(); err == nil {
		data.Data["tcp_stats"] = tcpStats
	}

	// Collect network errors
	if errors, err := n.collectNetworkErrors(); err == nil {
		data.Data["errors"] = errors
	}

	// Collect routing table
	if routes, err := n.collectRoutingTable(); err == nil {
		data.Data["routes"] = routes
	}

	return &Result{
		Data:    data,
		Error:   nil,
		Skipped: false,
		Reason:  "",
	}, nil
}

// collectNetworkInterfaces collects network interface information
func (n *NetworkCollector) collectNetworkInterfaces() (map[string]interface{}, error) {
	interfaces := make(map[string]interface{})

	// Read interface statistics
	stats, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/dev: %w", err)
	}
	defer stats.Close()

	scanner := bufio.NewScanner(stats)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum <= 2 {
			continue // Skip header lines
		}

		fields := strings.Fields(line)
		if len(fields) >= 17 {
			iface := strings.TrimSuffix(fields[0], ":")

			rxBytes, _ := strconv.ParseInt(fields[1], 10, 64)
			rxPackets, _ := strconv.ParseInt(fields[2], 10, 64)
			rxErrors, _ := strconv.ParseInt(fields[3], 10, 64)
			rxDropped, _ := strconv.ParseInt(fields[4], 10, 64)

			txBytes, _ := strconv.ParseInt(fields[9], 10, 64)
			txPackets, _ := strconv.ParseInt(fields[10], 10, 64)
			txErrors, _ := strconv.ParseInt(fields[11], 10, 64)
			txDropped, _ := strconv.ParseInt(fields[12], 10, 64)

			interfaces[iface] = map[string]interface{}{
				"rx_bytes":   rxBytes,
				"rx_packets": rxPackets,
				"rx_errors":  rxErrors,
				"rx_dropped": rxDropped,
				"tx_bytes":   txBytes,
				"tx_packets": txPackets,
				"tx_errors":  txErrors,
				"tx_dropped": txDropped,
			}
		}
	}

	// Add interface configuration
	for iface := range interfaces {
		if config, err := n.collectInterfaceConfig(iface); err == nil {
			interfaces[iface].(map[string]interface{})["config"] = config
		}
	}

	return interfaces, nil
}

// collectInterfaceConfig collects interface configuration
func (n *NetworkCollector) collectInterfaceConfig(iface string) (map[string]interface{}, error) {
	config := make(map[string]interface{})

	// Read interface flags
	flagsPath := fmt.Sprintf("/sys/class/net/%s/flags", iface)
	if flags, err := os.ReadFile(flagsPath); err == nil {
		if flagsInt, err := strconv.ParseInt(strings.TrimSpace(string(flags)), 0, 64); err == nil {
			config["flags"] = flagsInt
			config["up"] = (flagsInt & 0x1) != 0
			config["broadcast"] = (flagsInt & 0x2) != 0
			config["loopback"] = (flagsInt & 0x8) != 0
		}
	}

	// Read interface MTU
	mtuPath := fmt.Sprintf("/sys/class/net/%s/mtu", iface)
	if mtu, err := os.ReadFile(mtuPath); err == nil {
		if mtuInt, err := strconv.ParseInt(strings.TrimSpace(string(mtu)), 10, 64); err == nil {
			config["mtu"] = mtuInt
		}
	}

	// Read interface MAC address
	addrPath := fmt.Sprintf("/sys/class/net/%s/address", iface)
	if addr, err := os.ReadFile(addrPath); err == nil {
		config["mac_address"] = strings.TrimSpace(string(addr))
	}

	// Read interface speed
	speedPath := fmt.Sprintf("/sys/class/net/%s/speed", iface)
	if speed, err := os.ReadFile(speedPath); err == nil {
		if speedInt, err := strconv.ParseInt(strings.TrimSpace(string(speed)), 10, 64); err == nil {
			config["speed_mbps"] = speedInt
		}
	}

	return config, nil
}

// collectTCPStats collects TCP statistics
func (n *NetworkCollector) collectTCPStats() (map[string]interface{}, error) {
	tcpStats := make(map[string]interface{})

	// Read TCP statistics
	stats, err := os.Open("/proc/net/snmp")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/snmp: %w", err)
	}
	defer stats.Close()

	scanner := bufio.NewScanner(stats)
	var tcpSection bool
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Tcp:") {
			if !tcpSection {
				tcpSection = true
				continue // Skip header
			}

			fields := strings.Fields(line)
			if len(fields) >= 15 {
				tcpStats["active_opens"] = parseInt64(fields[5])
				tcpStats["passive_opens"] = parseInt64(fields[6])
				tcpStats["attempt_fails"] = parseInt64(fields[7])
				tcpStats["established_resets"] = parseInt64(fields[8])
				tcpStats["current_established"] = parseInt64(fields[9])
				tcpStats["in_segments"] = parseInt64(fields[10])
				tcpStats["out_segments"] = parseInt64(fields[11])
				tcpStats["retransmitted_segments"] = parseInt64(fields[12])
				tcpStats["in_errors"] = parseInt64(fields[13])
				tcpStats["out_resets"] = parseInt64(fields[14])
			}
			break
		}
	}

	// Read TCP retransmission statistics
	retrans, err := n.collectTCPRetrans()
	if err == nil {
		tcpStats["retransmission_details"] = retrans
	}

	return tcpStats, nil
}

// collectTCPRetrans collects TCP retransmission details
func (n *NetworkCollector) collectTCPRetrans() (map[string]interface{}, error) {
	retrans := make(map[string]interface{})

	// Read TCP retransmission statistics
	stats, err := os.Open("/proc/net/netstat")
	if err != nil {
		return retrans, nil
	}
	defer stats.Close()

	scanner := bufio.NewScanner(stats)
	var tcpExtSection bool
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "TcpExt:") {
			if !tcpExtSection {
				tcpExtSection = true
				continue // Skip header
			}

			fields := strings.Fields(line)
			if len(fields) >= 30 {
				retrans["syncookies_sent"] = parseInt64(fields[1])
				retrans["syncookies_recv"] = parseInt64(fields[2])
				retrans["syncookies_failed"] = parseInt64(fields[3])
				retrans["embryonic_rsts"] = parseInt64(fields[4])
				retrans["prune_called"] = parseInt64(fields[5])
				retrans["rcv_pruned"] = parseInt64(fields[6])
				retrans["oof_pruned"] = parseInt64(fields[7])
				retrans["out_of_window_icmps"] = parseInt64(fields[8])
				retrans["lock_dropped_icmps"] = parseInt64(fields[9])
				retrans["arp_filter"] = parseInt64(fields[10])
				retrans["time_waited"] = parseInt64(fields[11])
				retrans["time_wait_recycled"] = parseInt64(fields[12])
				retrans["time_wait_killed"] = parseInt64(fields[13])
				retrans["paws_active"] = parseInt64(fields[14])
				retrans["paws_estab"] = parseInt64(fields[15])
				retrans["delayed_ack"] = parseInt64(fields[16])
				retrans["delayed_ack_locked"] = parseInt64(fields[17])
				retrans["delayed_ack_lost"] = parseInt64(fields[18])
				retrans["listen_overflows"] = parseInt64(fields[19])
				retrans["listen_drops"] = parseInt64(fields[20])
				retrans["tcp_prequeued"] = parseInt64(fields[21])
				retrans["tcp_direct_copy_from_backlog"] = parseInt64(fields[22])
				retrans["tcp_direct_copy_from_prequeue"] = parseInt64(fields[23])
				retrans["tcp_prequeue_dropped"] = parseInt64(fields[24])
				retrans["tcp_ofo_queue"] = parseInt64(fields[25])
				retrans["tcp_ofo_dropped"] = parseInt64(fields[26])
				retrans["tcp_ofo_merge"] = parseInt64(fields[27])
				retrans["tcp_challenge_ack"] = parseInt64(fields[28])
				retrans["tcp_syn_challenge"] = parseInt64(fields[29])
			}
			break
		}
	}

	return retrans, nil
}

// collectNetworkErrors collects network error information
func (n *NetworkCollector) collectNetworkErrors() (map[string]interface{}, error) {
	errors := make(map[string]interface{})

	// Read network errors from kernel logs
	logFiles := []string{"/var/log/kern.log", "/var/log/messages"}
	for _, logFile := range logFiles {
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			continue
		}

		file, err := os.Open(logFile)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var networkErrors []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "network error") || strings.Contains(line, "TCP error") ||
				strings.Contains(line, "UDP error") || strings.Contains(line, "ICMP error") ||
				strings.Contains(line, "dropped packet") || strings.Contains(line, "collision") {
				networkErrors = append(networkErrors, line)
			}
		}

		if len(networkErrors) > 0 {
			errors["kernel_errors"] = networkErrors
		}
		break
	}

	// Collect interface errors from /proc/net/dev
	if interfaces, err := n.collectNetworkInterfaces(); err == nil {
		interfaceErrors := make(map[string]interface{})
		for iface, data := range interfaces {
			if ifaceData, ok := data.(map[string]interface{}); ok {
				interfaceErrors[iface] = map[string]interface{}{
					"rx_errors":  ifaceData["rx_errors"],
					"rx_dropped": ifaceData["rx_dropped"],
					"tx_errors":  ifaceData["tx_errors"],
					"tx_dropped": ifaceData["tx_dropped"],
				}
			}
		}
		errors["interface_errors"] = interfaceErrors
	}

	return errors, nil
}

// collectRoutingTable collects routing table information
func (n *NetworkCollector) collectRoutingTable() ([]map[string]interface{}, error) {
	var routes []map[string]interface{}

	// Read routing table
	routeFile, err := os.Open("/proc/net/route")
	if err != nil {
		return routes, nil
	}
	defer routeFile.Close()

	scanner := bufio.NewScanner(routeFile)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		fields := strings.Fields(line)
		if len(fields) >= 8 {
			route := map[string]interface{}{
				"interface":   fields[0],
				"destination": parseHexIP(fields[1]),
				"gateway":     parseHexIP(fields[2]),
				"flags":       parseHexInt(fields[3]),
				"refcnt":      parseInt64(fields[4]),
				"use":         parseInt64(fields[5]),
				"metric":      parseInt64(fields[6]),
				"mask":        parseHexIP(fields[7]),
			}
			routes = append(routes, route)
		}
	}

	return routes, nil
}

// Validate validates the collector's parameters
func (n *NetworkCollector) Validate(params map[string]interface{}) error {
	// Network collector has no required parameters
	return nil
}

// IsLocalOnly indicates if this collector provides node-local data only
func (n *NetworkCollector) IsLocalOnly() bool {
	return true
}

// IsClusterSupplement indicates if this collector provides cluster-level data
func (n *NetworkCollector) IsClusterSupplement() bool {
	return false
}

// Priority returns the collector's priority level
func (n *NetworkCollector) Priority() int {
	return 1
}

// Timeout returns the recommended timeout for this collector
func (n *NetworkCollector) Timeout() time.Duration {
	return 5 * time.Second
}

// Helper functions
func parseHexIP(hexIP string) string {
	if hexIP == "00000000" {
		return "0.0.0.0"
	}

	ip, err := strconv.ParseInt(hexIP, 16, 64)
	if err != nil {
		return hexIP
	}

	return fmt.Sprintf("%d.%d.%d.%d",
		(ip>>0)&0xFF,
		(ip>>8)&0xFF,
		(ip>>16)&0xFF,
		(ip>>24)&0xFF)
}

func parseHexInt(hexStr string) int64 {
	val, _ := strconv.ParseInt(hexStr, 16, 64)
	return val
}
