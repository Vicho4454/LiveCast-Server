package mdns

import (
	"fmt"
	"net"

	"github.com/grandcat/zeroconf"
)

type Server struct {
	server *zeroconf.Server
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Start(port int) error {
	// Obtener IP local para el log
	ip, err := getLocalIP()
	if err != nil {
		fmt.Println("[mDNS] Advertencia: No se pudo determinar la IP local:", err)
	} else {
		fmt.Printf("[mDNS] Anunciando servicio _livecast._tcp en la red (IP: %s, Puerto: %d)\n", ip, port)
	}

	// Registrar el servicio mDNS
	server, err := zeroconf.Register(
		"LiveCast Server",       // instance name
		"_livecast._tcp",        // service type
		"local.",                // domain
		port,                    // port
		[]string{"txtv=1", "app=livecast"}, // text records
		nil,                     // network interfaces (nil = all)
	)

	if err != nil {
		return fmt.Errorf("error al registrar mDNS: %v", err)
	}
	s.server = server
	return nil
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.Shutdown()
		fmt.Println("[mDNS] Servicio de autodescubrimiento detenido.")
	}
}

// Helper to get local IP
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no se encontró una IP IPv4 válida")
}
