package preview

import (
	"bytes"
	"image"
	"image/jpeg"
	"sync"
	"time"
)

type Broadcaster struct {
	clients    map[chan []byte]bool
	mutex      sync.Mutex
	bufPool    sync.Pool
	frameChan  chan *image.RGBA
	stopChan   chan struct{}
}

func NewBroadcaster() *Broadcaster {
	b := &Broadcaster{
		clients: make(map[chan []byte]bool),
		bufPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		frameChan: make(chan *image.RGBA, 1),
		stopChan:  make(chan struct{}),
	}
	go b.worker()
	return b
}

func (b *Broadcaster) Stop() {
	close(b.stopChan)
}

func (b *Broadcaster) Subscribe() chan []byte {
	ch := make(chan []byte, 2)
	b.mutex.Lock()
	b.clients[ch] = true
	b.mutex.Unlock()
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mutex.Lock()
	delete(b.clients, ch)
	b.mutex.Unlock()
	close(ch)
}

func (b *Broadcaster) PushFrame(data []byte, width, height, stride int) {
	b.mutex.Lock()
	clientsCount := len(b.clients)
	b.mutex.Unlock()

	// 1. Optimización: ¡No procesar si nadie está mirando! (Ahorro de CPU 100% y 0 latencia para NDI)
	if clientsCount == 0 {
		return 
	}

	// 2. Reducción ultrarrápida a 720p y conversión BGRA -> RGBA sincrónica (toma < 1ms)
	targetW, targetH := 1280, 720
	if width <= targetW || height <= targetH {
		targetW, targetH = width, height
	}

	scaleX := float64(width) / float64(targetW)
	scaleY := float64(height) / float64(targetH)

	img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	
	for y := 0; y < targetH; y++ {
		srcY := int(float64(y) * scaleY)
		dstOffset := y * img.Stride
		srcOffset := srcY * stride
		
		for x := 0; x < targetW; x++ {
			srcX := int(float64(x) * scaleX)
			sIdx := srcOffset + srcX*4
			dIdx := dstOffset + x*4
			
			// Transformar BGRA -> RGBA
			img.Pix[dIdx+0] = data[sIdx+2]
			img.Pix[dIdx+1] = data[sIdx+1]
			img.Pix[dIdx+2] = data[sIdx+0]
			img.Pix[dIdx+3] = 255
		}
	}

	// 3. Enviar a hilo secundario sin bloquear el flujo NDI
	select {
	case b.frameChan <- img:
	default:
		// Drop si el worker aún está ocupado (garantiza cero latencia)
	}
}

func (b *Broadcaster) worker() {
	// Limitar a ~15-20 FPS para la web
	ticker := time.NewTicker(1000 / 20 * time.Millisecond)
	defer ticker.Stop()

	var lastImg *image.RGBA

	for {
		select {
		case <-b.stopChan:
			return
		case img := <-b.frameChan:
			lastImg = img
		case <-ticker.C:
			if lastImg == nil {
				continue
			}
			
			// Compresión JPEG asincrónica (fuera del camino crítico de NDI)
			buf := b.bufPool.Get().(*bytes.Buffer)
			buf.Reset()
			jpeg.Encode(buf, lastImg, &jpeg.Options{Quality: 50})
			
			frameBytes := make([]byte, buf.Len())
			copy(frameBytes, buf.Bytes())
			b.bufPool.Put(buf)

			lastImg = nil // consumido

			b.mutex.Lock()
			for ch := range b.clients {
				select {
				case ch <- frameBytes:
				default:
				}
			}
			b.mutex.Unlock()
		}
	}
}
