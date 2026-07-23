package tally

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"SERVLOC-TEST/backend/control"
)

// VMix representa el root del XML de vMix
type VMix struct {
	XMLName xml.Name    `xml:"vmix"`
	Active  int         `xml:"active"`
	Preview int         `xml:"preview"`
	Inputs  []VMixInput `xml:"inputs>input"`
}

type VMixInput struct {
	Number int    `xml:"number,attr"`
	Title  string `xml:",chardata"`
}

type Poller struct {
	ip         string
	controlMgr *control.Manager
	stopChan   chan struct{}
	mutex      sync.Mutex
	isRunning  bool
	
	// Para recordar el estado anterior y no enviar comandos repetidos
	lastStates map[string]string // map[cameraID] -> "program", "preview", "off"
}

var instance *Poller

// InitPoller inicializa el singleton
func InitPoller(mgr *control.Manager) {
	instance = &Poller{
		controlMgr: mgr,
		lastStates: make(map[string]string),
	}
}

// Start arranca el polling a una IP de vMix
func Start(ip string) error {
	if instance == nil {
		return fmt.Errorf("Poller not initialized")
	}

	instance.mutex.Lock()
	defer instance.mutex.Unlock()

	if instance.isRunning {
		close(instance.stopChan)
		instance.isRunning = false
	}

	instance.ip = ip
	instance.stopChan = make(chan struct{})
	instance.isRunning = true
	
	fmt.Printf("[Tally] Iniciando conexión a vMix en la IP: %s\n", ip)

	go instance.pollLoop()
	return nil
}

// Stop detiene el polling
func Stop() {
	if instance == nil {
		return
	}
	instance.mutex.Lock()
	defer instance.mutex.Unlock()

	if instance.isRunning {
		close(instance.stopChan)
		instance.isRunning = false
	}
}

func Status() string {
	if instance == nil {
		return ""
	}
	instance.mutex.Lock()
	defer instance.mutex.Unlock()
	if instance.isRunning {
		return instance.ip
	}
	return ""
}

func (p *Poller) pollLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.fetchAndProcess()
		}
	}
}

func (p *Poller) fetchAndProcess() {
	url := fmt.Sprintf("http://%s:8088/api", p.ip)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("[Tally] Error conectando a vMix en %s: %v\n", url, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("[Tally] Error leyendo respuesta de vMix: %v\n", err)
		return
	}

	var state VMix
	if err := xml.Unmarshal(body, &state); err != nil {
		fmt.Printf("[Tally] Error parseando XML de vMix: %v\n", err)
		return
	}

	currentStates := make(map[string]string)

	for _, input := range state.Inputs {
		// El nombre del input debe contener el ID de la cámara (ej "cam1" o "LiveCast - cam1")
		camID := extractCamID(input.Title)
		if camID == "" {
			continue
		}

		status := "off"
		if input.Number == state.Active {
			status = "program"
		} else if input.Number == state.Preview {
			status = "preview"
		}

		// Si vMix tiene la misma cámara en múltiples inputs, prevalece program
		if existing, ok := currentStates[camID]; ok {
			if existing == "program" {
				continue
			}
		}
		currentStates[camID] = status
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 1. Enviar Tally para las cámaras que están en program/preview y cambiaron
	for camID, status := range currentStates {
		if status != "off" {
			if p.lastStates[camID] != status {
				p.lastStates[camID] = status
				fmt.Printf("[Tally] Cambio detectado: '%s' ahora está en '%s'\n", camID, status)
				p.controlMgr.SendCommand(camID, control.Command{Action: "tally", Value: status})
			}
		}
	}

	// 2. Apagar Tally para las cámaras que estaban activas y ahora no están ni en program ni preview
	for camID, lastStatus := range p.lastStates {
		if lastStatus != "off" {
			if status, exists := currentStates[camID]; !exists || status == "off" {
				p.lastStates[camID] = "off"
				fmt.Printf("[Tally] Cambio detectado: '%s' ahora está 'off'\n", camID)
				p.controlMgr.SendCommand(camID, control.Command{Action: "tally", Value: "off"})
			}
		}
	}
}

func extractCamID(title string) string {
	parts := strings.Split(title, "-")
	lastPart := strings.TrimSpace(parts[len(parts)-1])
	return lastPart
}
