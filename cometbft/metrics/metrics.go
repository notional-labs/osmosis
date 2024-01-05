package metrics

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

var instance *Metrics

type Metrics struct {
	mu sync.RWMutex

	values map[string]map[string]float64
}

func newMetrics() *Metrics {
	return &Metrics{
		values: make(map[string]map[string]float64),
	}
}

func Instance() *Metrics {
	if instance == nil {
		instance = newMetrics()
	}
	return instance
}

func (m *Metrics) RecordValue(ip string, name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.values[ip]; !ok {
		m.values[ip] = make(map[string]float64)
	}

	m.values[ip][name] += value
}

func (m *Metrics) GetValues() map[string]map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ret := make(map[string]map[string]string)
	for name, labels := range m.values {
		ret[name] = make(map[string]string)
		for label, value := range labels {
			if value == 0 {
				continue
			}

			str := strings.TrimRight(
				fmt.Sprintf("%.18f", value),
				"0",
			)

			str = strings.TrimRight(str, ".")

			ret[name][label] = str
		}
	}

	return ret
}

func NetAddrToIp(addr net.Addr) string {
	switch addr := addr.(type) {
	case *net.UDPAddr:
		return addr.IP.String()
	case *net.TCPAddr:
		return addr.IP.String()
	default:
		return "IP_UNKNOWN"
	}
}
