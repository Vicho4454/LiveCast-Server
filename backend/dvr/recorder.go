package dvr

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Recorder struct {
	processes map[string]*exec.Cmd
	mutex     sync.Mutex
	OutputDir string
}

func NewRecorder(outputDir string) *Recorder {
	os.MkdirAll(outputDir, 0755)
	return &Recorder{
		processes: make(map[string]*exec.Cmd),
		OutputDir: outputDir,
	}
}

func (r *Recorder) StartRecording(streamID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.processes[streamID]; exists {
		return fmt.Errorf("ya se está grabando %s", streamID)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s/%s_%s.mov", r.OutputDir, streamID, timestamp)

	// Iniciar FFmpeg copiando el códec original sin pérdida
	cmd := exec.Command("ffmpeg", "-y", "-rtsp_transport", "tcp", "-i", fmt.Sprintf("rtsp://127.0.0.1:8554/%s", streamID), "-c", "copy", "-movflags", "+faststart", filename)

	err := cmd.Start()
	if err != nil {
		return err
	}

	r.processes[streamID] = cmd
	fmt.Printf("[DVR] Grabación iniciada para %s: %s\n", streamID, filename)
	return nil
}

func (r *Recorder) StopRecording(streamID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if cmd, exists := r.processes[streamID]; exists {
		fmt.Printf("[DVR] Solicitando cierre seguro del archivo MOV para %s...\n", streamID)
		cmd.Process.Signal(os.Interrupt)
		go func(c *exec.Cmd, id string) {
			c.Wait()
			fmt.Printf("[DVR] Grabación finalizada y guardada para %s\n", id)
		}(cmd, streamID)
		delete(r.processes, streamID)
	}
}

func (r *Recorder) StopAll() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for id, cmd := range r.processes {
		cmd.Process.Signal(os.Interrupt)
		go cmd.Wait()
		delete(r.processes, id)
	}
}

func (r *Recorder) IsRecording(streamID string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	_, exists := r.processes[streamID]
	return exists
}
