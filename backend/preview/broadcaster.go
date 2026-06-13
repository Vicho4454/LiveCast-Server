package preview

import (
	"bytes"
	"image"
	"image/jpeg"
	"sync"
)

type Broadcaster struct {
	clients map[chan []byte]bool
	mutex   sync.Mutex
	bufPool sync.Pool
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[chan []byte]bool),
		bufPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (b *Broadcaster) Subscribe() chan []byte {
	// Buffer de 5 fotogramas para no bloquear la decodificación principal
	ch := make(chan []byte, 5) 
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

	// 1. Optimización: ¡No codificar si nadie está mirando! (Ahorro de CPU 100%)
	if clientsCount == 0 {
		return 
	}

	// 2. Reducción ultrarrápida a 720p y conversión BGRA -> RGBA (Nearest Neighbor)
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
			
			// Transformar BGRA -> RGBA en vuelo
			img.Pix[dIdx+0] = data[sIdx+2] // Rojo
			img.Pix[dIdx+1] = data[sIdx+1] // Verde
			img.Pix[dIdx+2] = data[sIdx+0] // Azul
			img.Pix[dIdx+3] = 255          // Opaco
		}
	}

	// 3. Compresión JPEG a calidad 60 (buen balance CPU/Calidad)
	buf := b.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	jpeg.Encode(buf, img, &jpeg.Options{Quality: 60})
	
	frameBytes := make([]byte, buf.Len())
	copy(frameBytes, buf.Bytes())
	b.bufPool.Put(buf)

	// 4. Distribuir a los navegadores
	b.mutex.Lock()
	for ch := range b.clients {
		select {
		case ch <- frameBytes:
		default:
			// Si la red del cliente es lenta, se saltará este frame para no atrasar la transmisión "tiempo real"
		}
	}
	b.mutex.Unlock()
}
