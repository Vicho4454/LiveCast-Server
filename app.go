package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"net"
	"net/http"
	"strings"
	"runtime"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/bluenviron/mediamtx"
	"SERVLOC-TEST/backend/decoder"
	"SERVLOC-TEST/backend/ndi"
	"SERVLOC-TEST/backend/preview"
	"SERVLOC-TEST/backend/control"
	"SERVLOC-TEST/backend/tally"
	"SERVLOC-TEST/backend/mdns"
	"SERVLOC-TEST/backend/dvr"
	"path/filepath"
	_ "embed"
)

//go:embed mediamtx.yml
var mediamtxConfig []byte

// DeviceTelemetry maps the incoming telemetry JSON payload
type DeviceTelemetry struct {
	StreamID     string `json:"streamId"`
	BatteryLevel int    `json:"batteryLevel"`
	IsCharging   bool   `json:"isCharging"`
	LastUpdate   time.Time `json:"-"`
	Bitrate      float64 `json:"bitrate"`
}

type StreamStat struct {
	LastBytes int64
	LastTime  time.Time
	Bitrate   float64
}

// App struct
type App struct {
	ctx           context.Context
	m             *mediamtx.Server
	activeNDI     map[string]*ndi.Encoder
	broadcasters  map[string]*preview.Broadcaster
	controlMgr    *control.Manager
	ndiMutex      sync.Mutex
	ndiIsInit     bool
	telemetry     map[string]DeviceTelemetry
	telemetryMut  sync.RWMutex
	httpServer    *http.Server
	mdnsServer    *mdns.Server
	dvrRecorder   *dvr.Recorder
	streamStats   map[string]*StreamStat
}

func getRecordingsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "LiveCast_Recordings"
	}
	return filepath.Join(home, "Desktop", "LiveCast_Recordings")
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		activeNDI:    make(map[string]*ndi.Encoder),
		broadcasters: make(map[string]*preview.Broadcaster),
		controlMgr:   control.NewManager(),
		telemetry:    make(map[string]DeviceTelemetry),
		mdnsServer:   mdns.NewServer(),
		streamStats:  make(map[string]*StreamStat),
	}
}

// shutdown is called at application termination
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("Ejecutando Graceful Shutdown: Cerrando RTMP, NDI, RTSP y liberando puertos...")
	if a.dvrRecorder != nil {
		a.dvrRecorder.StopAll()
	}
	if a.mdnsServer != nil {
		a.mdnsServer.Stop()
	}
	if a.httpServer != nil {
		a.httpServer.Shutdown(context.Background())
	}
	if a.m != nil {
		a.m.Close()
	}
	
	a.ndiMutex.Lock()
	for _, enc := range a.activeNDI {
		enc.Close()
	}
	for _, bc := range a.broadcasters {
		bc.Stop()
	}
	a.ndiMutex.Unlock()

	if a.ndiIsInit {
		ndi.Destroy()
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	fmt.Println("SERVLOC-TEST Backend Inicializado")
	
	// Iniciar Poller de Tally
	tally.InitPoller(a.controlMgr)

	errMDNS := a.mdnsServer.Start(8080)
	if errMDNS != nil {
		fmt.Println("[mDNS] Error al iniciar servicio mDNS:", errMDNS)
	}

	a.dvrRecorder = dvr.NewRecorder(getRecordingsDir())

	// Capturar stdout para la consola del frontend
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "backend-log", line)
			}
		}
	}()

	// Hack for macOS 14+: Force Local Network permission prompt by initiating an outbound local connection
	go func() {
		conn, _ := net.DialTimeout("udp", "255.255.255.255:12345", 1*time.Second)
		if conn != nil {
			conn.Close()
		}
	}()

	err := ndi.Init()
	if err == nil {
		a.ndiIsInit = true
	} else {
		fmt.Println("Advertencia: No se pudo iniciar SDK de NDI:", err)
	}

	// Configurar MediaMTX con el archivo embebido
	configPath := filepath.Join(os.TempDir(), "livecast_mediamtx.yml")
	err = os.WriteFile(configPath, mediamtxConfig, 0644)
	if err != nil {
		fmt.Println("Error escribiendo archivo de configuración de MediaMTX:", err)
	}

	// Iniciar MediaMTX
	go func() {
		m, ok := mediamtx.Start(configPath)
		if !ok {
			fmt.Println("Error iniciando MediaMTX")
			return
		}
		a.m = m
		a.m.Wait()
	}()

	// Iniciar Servidor HTTP para Telemetría y Panel Remoto (Puerto 3000)
	go a.startHTTPServer()
}

func (a *App) startHTTPServer() {
	mux := http.NewServeMux()

	// 1. Endpoint POST para recibir telemetría
	mux.HandleFunc("/api/telemetry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var t DeviceTelemetry
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		t.LastUpdate = time.Now()
		
		a.telemetryMut.Lock()
		a.telemetry[t.StreamID] = t
		a.telemetryMut.Unlock()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 2. Endpoint GET para exponer los datos (usado por el panel web)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(a.GetTelemetry())
	})

	// 3. Endpoint MJPEG para previsualización en vivo (Modo Estudio)
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		streamID := r.URL.Query().Get("id")
		if streamID == "" {
			http.Error(w, "Missing id", http.StatusBadRequest)
			return
		}

		a.ndiMutex.Lock()
		broadcaster := a.broadcasters[streamID]
		if broadcaster == nil {
			parts := strings.Split(streamID, "/")
			shortID := parts[len(parts)-1]
			broadcaster = a.broadcasters[shortID]
		}
		a.ndiMutex.Unlock()

		if broadcaster == nil {
			http.Error(w, "Stream not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		w.Header().Set("Cache-Control", "no-cache, private")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case frame := <-ch:
				_, err := w.Write([]byte(fmt.Sprintf("--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))))
				if err != nil {
					return
				}
				_, err = w.Write(frame)
				if err != nil {
					return
				}
				_, err = w.Write([]byte("\r\n"))
				if err != nil {
					return
				}
			}
		}
	})

	// 4. Endpoint de frame individual (para polling rápido en Safari/WKWebView)
	mux.HandleFunc("/api/frame", func(w http.ResponseWriter, r *http.Request) {
		streamID := r.URL.Query().Get("id")
		a.ndiMutex.Lock()
		broadcaster := a.broadcasters[streamID]
		if broadcaster == nil {
			parts := strings.Split(streamID, "/")
			broadcaster = a.broadcasters[parts[len(parts)-1]]
		}
		a.ndiMutex.Unlock()

		if broadcaster == nil {
			http.Error(w, "Not found", 404)
			return
		}

		ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(ch)

		select {
		case <-r.Context().Done():
			return
		case frame := <-ch:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(frame)
		case <-time.After(2 * time.Second):
			http.Error(w, "Timeout", 504)
		}
	})

	// 5. Endpoint WebSocket para recibir conexión de control desde el teléfono
	mux.HandleFunc("/api/control", a.controlMgr.HandleWebSocket)

	// 6. Endpoint POST para que el Dashboard envíe comandos
	mux.HandleFunc("/api/camera/control", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var cmd control.Command
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			http.Error(w, "Invalid command", http.StatusBadRequest)
			return
		}
		
		streamID := r.URL.Query().Get("id")
		err := a.controlMgr.SendCommand(streamID, cmd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.WriteHeader(http.StatusOK)
	})

	// 7. Endpoints de Tally
	mux.HandleFunc("/api/tally/vmix", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, DELETE")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodDelete {
			tally.Stop()
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		if payload.IP == "" {
			tally.Stop()
		} else {
			tally.Start(payload.IP)
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/tally/vmix/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		
		ip := tally.Status()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": ip != "",
			"ip":        ip,
		})
	})

	// 8. Endpoints de DVR
	mux.HandleFunc("/api/dvr", a.handleDVR)

	// 9. Servir el Frontend empaquetado (Vue/Wails)
	frontendFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		fmt.Println("No se pudo cargar el frontendFS estático:", err)
	} else {
		mux.Handle("/", http.FileServer(http.FS(frontendFS)))
	}

	a.httpServer = &http.Server{
		Addr:    ":3000",
		Handler: mux,
	}

	fmt.Println("Servidor Web/Telemetría escuchando en http://localhost:3000")
	if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("Error en servidor HTTP:", err)
	}
}

// GetTelemetry returns system and server stats to the frontend
func (a *App) GetTelemetry() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	sessions := []map[string]interface{}{}
	currentStreams := make(map[string]bool)

	// Fetch API from MediaMTX
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://localhost:9997/v3/paths/list")
	if err == nil {
		defer resp.Body.Close()
		var result struct {
			Items []struct {
				Name          string `json:"name"`
				Ready         bool   `json:"ready"`
				BytesReceived int64  `json:"bytesReceived"`
			} `json:"items"`
		}
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			a.telemetryMut.RLock()
			now := time.Now()
			for _, item := range result.Items {
				if item.Ready {
					currentStreams[item.Name] = true

					// Obtenemos telemetría si existe (por nombre completo o nombre corto)
					t, ok := a.telemetry[item.Name]
					if !ok {
						parts := strings.Split(item.Name, "/")
						shortID := parts[len(parts)-1]
						t = a.telemetry[shortID]
					}

					// Calculate actual bitrate
					stat, exists := a.streamStats[item.Name]
					if !exists {
						stat = &StreamStat{LastBytes: item.BytesReceived, LastTime: now, Bitrate: 0}
						a.streamStats[item.Name] = stat
					} else {
						diffTime := now.Sub(stat.LastTime).Seconds()
						if diffTime >= 1.0 { // Update every 1s+
							diffBytes := item.BytesReceived - stat.LastBytes
							if diffBytes < 0 {
								diffBytes = 0
							}
							stat.Bitrate = float64(diffBytes) * 8 / 1000000.0 / diffTime
							stat.LastBytes = item.BytesReceived
							stat.LastTime = now
						}
					}

					sessions = append(sessions, map[string]interface{}{
						"id":      item.Name,
						"bitrate": stat.Bitrate,
						"ndiName": "LiveCast-" + item.Name,
						"batteryLevel": t.BatteryLevel,
						"isCharging":   t.IsCharging,
						"hasTelemetry": !t.LastUpdate.IsZero() && time.Since(t.LastUpdate) < 15*time.Second,
					})

					// Si NDI está iniciado y no existe el encoder para este stream, lo creamos
					a.ndiMutex.Lock()
					if a.ndiIsInit && a.activeNDI[item.Name] == nil {
						fmt.Println("Activando pipeline decodificador para:", item.Name)
						
						// Iniciar Broadcaster para MJPEG
						broadcaster := preview.NewBroadcaster()
						
						// Determinar el shortID para el endpoint HTTP
						parts := strings.Split(item.Name, "/")
						shortID := parts[len(parts)-1]
						a.broadcasters[shortID] = broadcaster

						enc, err := ndi.NewEncoder("LiveCast-" + item.Name)
						if err == nil {
							a.activeNDI[item.Name] = enc
							pipeline := decoder.NewPipeline(item.Name, enc)
							pipeline.OnFrame = broadcaster.PushFrame
							
							go func() {
								// We wait 1 second to ensure the RTSP path is fully published
								time.Sleep(1 * time.Second)
								err := pipeline.Start()
								if err != nil {
									fmt.Println("Error en pipeline:", err)
								}
							}()
						} else {
							fmt.Println("Error creando encoder NDI:", err)
						}
					}
					a.ndiMutex.Unlock()
				}
			}
			a.telemetryMut.RUnlock()
		}
	}

	// Cleanup encoders for disconnected streams
	a.ndiMutex.Lock()
	for name, enc := range a.activeNDI {
		if !currentStreams[name] {
			fmt.Println("Cerrando salida NDI para:", name)
			enc.Close()
			delete(a.activeNDI, name)

			parts := strings.Split(name, "/")
			shortID := parts[len(parts)-1]
			if bc, ok := a.broadcasters[shortID]; ok {
				bc.Stop()
				delete(a.broadcasters, shortID)
			}
		}
	}
	a.ndiMutex.Unlock()

	return map[string]interface{}{
		"cpuUsage":   1.2, // mock
		"ramUsage":   m.Alloc / 1024 / 1024,
		"sessions":   sessions,
		"version":    "1.0.0",
		"ndiEnabled": a.ndiIsInit,
	}
}

// handleDVR permite iniciar o detener la grabación de una cámara
func (a *App) handleDVR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		return
	}

	streamID := r.URL.Query().Get("id")
	if streamID == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		isRecording := a.dvrRecorder.IsRecording(streamID)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"recording": %v}`, isRecording)
		return
	}

	if r.Method == http.MethodPost {
		err := a.dvrRecorder.StartRecording(streamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodDelete {
		a.dvrRecorder.StopRecording(streamID)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
