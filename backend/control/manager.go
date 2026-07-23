package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Permitir conexiones desde la app móvil o web
	},
}

// Manager administra las conexiones WebSocket de control de cámara
type Manager struct {
	connections map[string]*websocket.Conn
	mutex       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*websocket.Conn),
	}
}

// HandleWebSocket actualiza la conexión HTTP a WebSocket y la registra
func (m *Manager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	streamID := r.URL.Query().Get("id")
	if streamID == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}

	// Limpiar nombre por si envían "live/camX" en vez de "camX"
	parts := strings.Split(streamID, "/")
	shortID := parts[len(parts)-1]

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading websocket:", err)
		return
	}

	m.mutex.Lock()
	// Cerrar conexión anterior si existía para ese ID
	if oldConn, exists := m.connections[shortID]; exists {
		oldConn.Close()
	}
	m.connections[shortID] = conn
	m.mutex.Unlock()

	fmt.Println("📲 Cliente de control conectado para:", shortID)

	// Bucle de lectura para mantener viva la conexión y detectar desconexiones
	go func() {
		defer func() {
			m.mutex.Lock()
			if currentConn, exists := m.connections[shortID]; exists && currentConn == conn {
				delete(m.connections, shortID)
			}
			m.mutex.Unlock()
			conn.Close()
			fmt.Println("🔌 Cliente de control desconectado para:", shortID)
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// Manejar sincronización de reloj (NTP sobre WebSockets)
			var data map[string]interface{}
			if err := json.Unmarshal(msg, &data); err == nil {
				if action, ok := data["action"].(string); ok && action == "time_sync" {
					resp := map[string]interface{}{
						"action":         "time_sync_reply",
						"server_time_ms": time.Now().UnixMilli(),
					}
					// Si el cliente envió su propio timestamp para calcular el RTT, lo devolvemos
					if clientTime, ok := data["client_time_ms"]; ok {
						resp["client_time_ms"] = clientTime
					}

					respJSON, _ := json.Marshal(resp)
					
					m.mutex.Lock()
					conn.WriteMessage(websocket.TextMessage, respJSON)
					m.mutex.Unlock()
				}
			}
		}
	}()
}

// Command representa una acción para la cámara
type Command struct {
	Action string      `json:"action"`
	Value  interface{} `json:"value"`
}

// SendCommand envía un comando a un dispositivo específico
func (m *Manager) SendCommand(streamID string, cmd Command) error {
	parts := strings.Split(streamID, "/")
	shortID := parts[len(parts)-1]

	m.mutex.RLock()
	conn, exists := m.connections[shortID]
	m.mutex.RUnlock()

	if !exists {
		fmt.Println("No connection found for:", shortID, "Connections:", m.connections); return fmt.Errorf("no hay conexión de control activa para %s", shortID)
	}

	msg, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	
	fmt.Printf("[WebSocket] Enviando comando a %s: %s\n", shortID, string(msg))

	m.mutex.Lock()
	err = conn.WriteMessage(websocket.TextMessage, msg)
	m.mutex.Unlock()

	return err
}
