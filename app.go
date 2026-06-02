package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx"
	"SERVLOC-TEST/backend/decoder"
	"SERVLOC-TEST/backend/ndi"
)

// App struct
type App struct {
	ctx          context.Context
	m            *mediamtx.Server
	activeNDI    map[string]*ndi.Encoder
	ndiMutex     sync.Mutex
	ndiIsInit    bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		activeNDI: make(map[string]*ndi.Encoder),
	}
}

// shutdown is called at application termination
func (a *App) shutdown(ctx context.Context) {
	fmt.Println("Ejecutando Graceful Shutdown: Cerrando RTMP, NDI, RTSP y liberando puertos...")
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
			for _, item := range result.Items {
				if item.Ready {
					currentStreams[item.Name] = true
					sessions = append(sessions, map[string]interface{}{
						"id":      item.Name,
						"bitrate": 6.5, // Mocked bitrate to show green health
						"ndiName": "LiveCast-" + item.Name,
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
