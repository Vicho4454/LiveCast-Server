//go:build darwin
package hardware

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreFoundation -framework CoreMedia -framework VideoToolbox -framework CoreVideo -framework Foundation
#include "vt_decoder_darwin.h"
#include <stdlib.h>
#include <string.h>

// Forward declaration of our Go callback
extern void goFrameCallback(void* user_data, uint8_t* bgra_data, int width, int height, int stride);
*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"sync"
	"unsafe"
)

// Decoder handles hardware accelerated decoding of RTMP H.264 streams
type Decoder struct {
	ref            C.VTDecoderRef
	OnFrameDecoded func(data []byte, width, height, stride int)
	id             int
}

var (
	decoders   = make(map[int]*Decoder)
	decodersMu sync.Mutex
	nextID     int
)

//export goFrameCallback
func goFrameCallback(userData unsafe.Pointer, bgraData *C.uint8_t, width C.int, height C.int, stride C.int) {
	id := int(uintptr(userData))
	decodersMu.Lock()
	decoder, ok := decoders[id]
	decodersMu.Unlock()

	if ok && decoder.OnFrameDecoded != nil {
		// Convert C array to Go slice safely
		length := int(height * stride)
		data := unsafe.Slice((*byte)(unsafe.Pointer(bgraData)), length)
		decoder.OnFrameDecoded(data, int(width), int(height), int(stride))
	}
}

// NewDecoder initializes a hardware decoder using Apple's VideoToolbox.
func NewDecoder(sps, pps []byte) (*Decoder, error) {
	if len(sps) == 0 || len(pps) == 0 {
		return nil, fmt.Errorf("invalid SPS or PPS")
	}

	cSPS := C.CBytes(sps)
	cPPS := C.CBytes(pps)
	defer C.free(cSPS)
	defer C.free(cPPS)

	decodersMu.Lock()
	nextID++
	id := nextID
	decodersMu.Unlock()

	userData := unsafe.Pointer(uintptr(id))

	ref := C.VTDecoderCreate(
		(*C.uint8_t)(cSPS), C.size_t(len(sps)),
		(*C.uint8_t)(cPPS), C.size_t(len(pps)),
		(C.VTFrameCallback)(C.goFrameCallback),
		userData,
	)

	if ref == nil {
		return nil, fmt.Errorf("failed to create VideoToolbox session")
	}

	d := &Decoder{
		ref: ref,
		id:  id,
	}

	decodersMu.Lock()
	decoders[id] = d
	decodersMu.Unlock()

	return d, nil
}

// DecodeNALU receives a single NAL unit (without Annex B start code) and decodes it.
func (d *Decoder) DecodeNALU(nalu []byte) {
	if d.ref == nil || len(nalu) == 0 {
		return
	}

	// VideoToolbox requires AVCC format: a 4-byte big-endian length prefix followed by the NALU data
	size := len(nalu)
	avcc := make([]byte, 4+size)
	binary.BigEndian.PutUint32(avcc[0:4], uint32(size))
	copy(avcc[4:], nalu)

	cAVCC := C.CBytes(avcc)
	defer C.free(cAVCC)

	C.VTDecoderDecode(d.ref, (*C.uint8_t)(cAVCC), C.size_t(len(avcc)))
}

// Close releases hardware resources
func (d *Decoder) Close() {
	if d.ref != nil {
		C.VTDecoderDestroy(d.ref)
		d.ref = nil
	}
	decodersMu.Lock()
	delete(decoders, d.id)
	decodersMu.Unlock()
}
