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
	"runtime"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/bluenviron/mediamtx"
	"SERVLOC-TEST/backend/decoder"
	"SERVLOC-TEST/backend/ndi"
)

// DeviceTelemetry maps the incoming telemetry JSON payload
type DeviceTelemetry struct {
	StreamID     string `json:"streamId"`
	BatteryLevel int    `json:"batteryLevel"`
	IsCharging   bool   `json:"isCharging"`
	LastUpdate   time.Time `json:"-"`
}

// App struct
type App struct {
	ctx           context.Context
	m             *mediamtx.Server
	activeNDI     map[string]*ndi.Encoder
	ndiMutex      sync.Mutex
	ndiIsInit     bool
	telemetry     map[string]DeviceTelemetry
	telemetryMut  sync.RWMutex
	httpServer    *http.Server
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		activeNDI: make(map[string]*ndi.Encoder),
		telemetry: make(map[string]DeviceTelemetry),
	}
}

// shutdown is called at application termination
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("Ejecutando Graceful Shutdown: Cerrando RTMP, NDI, RTSP y liberando puertos...")
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

	// Iniciar MediaMTX
	go func() {
		m, ok := mediamtx.Start()
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
		// Agregamos CORS por si acasi
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(a.GetTelemetry())
	})

	// 3. Servir el Frontend empaquetado (Vue/Wails)
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
				Name  string `json:"name"`
				Ready bool   `json:"ready"`
			} `json:"items"`
		}
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			a.telemetryMut.RLock()
			for _, item := range result.Items {
				if item.Ready {
					currentStreams[item.Name] = true

					// Obtenemos telemetría si existe
					t := a.telemetry[item.Name]

					sessions = append(sessions, map[string]interface{}{
						"id":      item.Name,
						"bitrate": 6.5, // Mocked bitrate to show green health
						"ndiName": "LiveCast-" + item.Name,
						"batteryLevel": t.BatteryLevel,
						"isCharging":   t.IsCharging,
						"hasTelemetry": !t.LastUpdate.IsZero() && time.Since(t.LastUpdate) < 15*time.Second,
					})

					// Si NDI está iniciado y no existe el encoder para este stream, lo creamos
					a.ndiMutex.Lock()
					if a.ndiIsInit && a.activeNDI[item.Name] == nil {
						fmt.Println("Activando pipeline decodificador para:", item.Name)
						enc, err := ndi.NewEncoder("LiveCast-" + item.Name)
						if err == nil {
							a.activeNDI[item.Name] = enc
							pipeline := decoder.NewPipeline(item.Name, enc)
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
