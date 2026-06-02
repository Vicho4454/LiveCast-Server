//go:build windows
package ndi

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/ndi/Include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/ndi/Lib/x64 -lProcessing.NDI.Lib.x64
#include <stdbool.h>
#include <Processing.NDI.Lib.h>
#include <stdlib.h>

void set_stride_win(NDIlib_video_frame_v2_t *frame, int stride) {
	frame->line_stride_in_bytes = stride;
}
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

type Encoder struct {
	sendInstance C.NDIlib_send_instance_t
	mu           sync.Mutex
	name         string
}

func Init() error {
	if C.NDIlib_initialize() == C.bool(false) {
		return fmt.Errorf("failed to initialize NDI SDK on Windows")
	}
	return nil
}

func Destroy() {
	C.NDIlib_destroy()
}

func NewEncoder(streamName string) (*Encoder, error) {
	cName := C.CString(streamName)
	defer C.free(unsafe.Pointer(cName))

	createDesc := C.NDIlib_send_create_t{
		p_ndi_name:  cName,
		p_groups:    nil,
		clock_video: C.bool(true),
		clock_audio: C.bool(false),
	}

	sendInstance := C.NDIlib_send_create(&createDesc)
	if sendInstance == nil {
		return nil, fmt.Errorf("failed to create NDI send instance on Windows")
	}

	return &Encoder{
		sendInstance: sendInstance,
		name:         streamName,
	}, nil
}

func (e *Encoder) SendVideoFrame(data []byte, width, height, stride int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.sendInstance == nil {
		return
	}

	cData := C.CBytes(data)
	defer C.free(cData)

	videoFrame := C.NDIlib_video_frame_v2_t{
		xres:                 C.int(width),
		yres:                 C.int(height),
		FourCC:               C.NDIlib_FourCC_video_type_BGRA,
		frame_rate_N:         30000,
		frame_rate_D:         1001,
		picture_aspect_ratio: C.float(float32(width) / float32(height)),
		frame_format_type:    C.NDIlib_frame_format_type_progressive,
		timecode:             C.NDIlib_send_timecode_synthesize,
		p_data:               (*C.uint8_t)(cData),
	}
	C.set_stride_win(&videoFrame, C.int(stride))

	C.NDIlib_send_send_video_v2(e.sendInstance, &videoFrame)
}

func (e *Encoder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sendInstance != nil {
		C.NDIlib_send_destroy(e.sendInstance)
		e.sendInstance = nil
	}
}
