//go:build windows
package hardware

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/ffmpeg/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/ffmpeg/lib -lavcodec -lavutil -lswscale
#include <libavcodec/avcodec.h>
#include <libavutil/hwcontext.h>
#include <libavutil/opt.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

typedef void (*FFmpegFrameCallback)(void* user_data, uint8_t* bgra_data, int width, int height, int stride);

typedef struct {
    AVCodecContext *codec_ctx;
    AVBufferRef *hw_device_ctx;
    struct SwsContext *sws_ctx;
    AVFrame *hw_frame;
    AVFrame *sw_frame;
    AVFrame *bgra_frame;
    AVPacket *pkt;
    FFmpegFrameCallback callback;
    void *user_data;
    uint8_t *bgra_buffer;
    int bgra_buffer_size;
} FFmpegDecoderCtx;

extern void goFrameCallbackWindows(void* user_data, uint8_t* bgra_data, int width, int height, int stride);

static enum AVPixelFormat get_hw_format(AVCodecContext *ctx, const enum AVPixelFormat *pix_fmts) {
    const enum AVPixelFormat *p;
    for (p = pix_fmts; *p != -1; p++) {
        if (*p == AV_PIX_FMT_D3D11 || *p == AV_PIX_FMT_DXVA2_VLD) {
            return *p;
        }
    }
    return AV_PIX_FMT_NONE;
}

static void* FFmpegDecoderCreate(const uint8_t* sps, size_t sps_size, const uint8_t* pps, size_t pps_size, FFmpegFrameCallback callback, void* user_data) {
    FFmpegDecoderCtx *ctx = calloc(1, sizeof(FFmpegDecoderCtx));
    ctx->callback = callback;
    ctx->user_data = user_data;

    const AVCodec *codec = avcodec_find_decoder(AV_CODEC_ID_H264);
    if (!codec) { free(ctx); return NULL; }

    ctx->codec_ctx = avcodec_alloc_context3(codec);
    if (!ctx->codec_ctx) { free(ctx); return NULL; }

    // Init hardware device (D3D11VA - Windows Native)
    int err = av_hwdevice_ctx_create(&ctx->hw_device_ctx, AV_HWDEVICE_TYPE_D3D11VA, NULL, NULL, 0);
    if (err < 0) {
        // Fallback to DXVA2
        err = av_hwdevice_ctx_create(&ctx->hw_device_ctx, AV_HWDEVICE_TYPE_DXVA2, NULL, NULL, 0);
        if (err < 0) {
            ctx->hw_device_ctx = NULL; // CPU fallback
        }
    }

    if (ctx->hw_device_ctx) {
        ctx->codec_ctx->hw_device_ctx = av_buffer_ref(ctx->hw_device_ctx);
        ctx->codec_ctx->get_format = get_hw_format;
    }

    // Extradata requires Annex B formatting: 0x00 00 00 01
    if (sps_size > 0 && pps_size > 0) {
        size_t extra_size = 4 + sps_size + 4 + pps_size;
        ctx->codec_ctx->extradata = av_mallocz(extra_size + AV_INPUT_BUFFER_PADDING_SIZE);
        ctx->codec_ctx->extradata_size = extra_size;
        
        ctx->codec_ctx->extradata[0] = 0; ctx->codec_ctx->extradata[1] = 0; ctx->codec_ctx->extradata[2] = 0; ctx->codec_ctx->extradata[3] = 1;
        memcpy(ctx->codec_ctx->extradata + 4, sps, sps_size);
        
        ctx->codec_ctx->extradata[4+sps_size] = 0; ctx->codec_ctx->extradata[4+sps_size+1] = 0; ctx->codec_ctx->extradata[4+sps_size+2] = 0; ctx->codec_ctx->extradata[4+sps_size+3] = 1;
        memcpy(ctx->codec_ctx->extradata + 4 + sps_size + 4, pps, pps_size);
    }

    if (avcodec_open2(ctx->codec_ctx, codec, NULL) < 0) {
        avcodec_free_context(&ctx->codec_ctx);
        free(ctx);
        return NULL;
    }

    ctx->hw_frame = av_frame_alloc();
    ctx->sw_frame = av_frame_alloc();
    ctx->bgra_frame = av_frame_alloc();
    ctx->pkt = av_packet_alloc();

    return ctx;
}

static void FFmpegDecoderDecode(void* decoder, const uint8_t* nalu_data, size_t size) {
    FFmpegDecoderCtx *ctx = (FFmpegDecoderCtx*)decoder;
    if (!ctx || !nalu_data || size == 0) return;

    ctx->pkt->data = (uint8_t*)nalu_data;
    ctx->pkt->size = size;

    if (avcodec_send_packet(ctx->codec_ctx, ctx->pkt) < 0) return;

    while (avcodec_receive_frame(ctx->codec_ctx, ctx->hw_frame) == 0) {
        AVFrame *frame = ctx->hw_frame;

        if (frame->format == AV_PIX_FMT_D3D11 || frame->format == AV_PIX_FMT_DXVA2_VLD) {
            if (av_hwframe_transfer_data(ctx->sw_frame, ctx->hw_frame, 0) < 0) {
                av_frame_unref(ctx->hw_frame);
                continue;
            }
            frame = ctx->sw_frame;
        }

        int w = frame->width;
        int h = frame->height;
        if (!ctx->sws_ctx || ctx->bgra_frame->width != w || ctx->bgra_frame->height != h) {
            if (ctx->sws_ctx) sws_freeContext(ctx->sws_ctx);
            ctx->sws_ctx = sws_getContext(w, h, frame->format, w, h, AV_PIX_FMT_BGRA, SWS_FAST_BILINEAR, NULL, NULL, NULL);
            
            if (ctx->bgra_buffer) av_freep(&ctx->bgra_buffer);
            ctx->bgra_buffer_size = av_image_get_buffer_size(AV_PIX_FMT_BGRA, w, h, 1);
            ctx->bgra_buffer = av_malloc(ctx->bgra_buffer_size);
            
            av_image_fill_arrays(ctx->bgra_frame->data, ctx->bgra_frame->linesize, ctx->bgra_buffer, AV_PIX_FMT_BGRA, w, h, 1);
            ctx->bgra_frame->width = w;
            ctx->bgra_frame->height = h;
            ctx->bgra_frame->format = AV_PIX_FMT_BGRA;
        }

        sws_scale(ctx->sws_ctx, (const uint8_t * const *)frame->data, frame->linesize, 0, h, ctx->bgra_frame->data, ctx->bgra_frame->linesize);

        if (ctx->callback) {
            ctx->callback(ctx->user_data, ctx->bgra_frame->data[0], w, h, ctx->bgra_frame->linesize[0]);
        }

        av_frame_unref(ctx->hw_frame);
        av_frame_unref(ctx->sw_frame);
    }
}

static void FFmpegDecoderDestroy(void* decoder) {
    FFmpegDecoderCtx *ctx = (FFmpegDecoderCtx*)decoder;
    if (ctx) {
        if (ctx->codec_ctx) avcodec_free_context(&ctx->codec_ctx);
        if (ctx->hw_device_ctx) av_buffer_unref(&ctx->hw_device_ctx);
        if (ctx->sws_ctx) sws_freeContext(ctx->sws_ctx);
        if (ctx->hw_frame) av_frame_free(&ctx->hw_frame);
        if (ctx->sw_frame) av_frame_free(&ctx->sw_frame);
        if (ctx->bgra_frame) av_frame_free(&ctx->bgra_frame);
        if (ctx->pkt) av_packet_free(&ctx->pkt);
        if (ctx->bgra_buffer) av_freep(&ctx->bgra_buffer);
        free(ctx);
    }
}
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

type Decoder struct {
	ref            C.FFmpegDecoderRef
	OnFrameDecoded func(data []byte, width, height, stride int)
	id             int
}

var (
	winDecoders   = make(map[int]*Decoder)
	winDecodersMu sync.Mutex
	winNextID     int
)

//export goFrameCallbackWindows
func goFrameCallbackWindows(userData unsafe.Pointer, bgraData *C.uint8_t, width C.int, height C.int, stride C.int) {
	id := int(uintptr(userData))
	winDecodersMu.Lock()
	decoder, ok := winDecoders[id]
	winDecodersMu.Unlock()

	if ok && decoder.OnFrameDecoded != nil {
		length := int(height * stride)
		data := unsafe.Slice((*byte)(unsafe.Pointer(bgraData)), length)
		decoder.OnFrameDecoded(data, int(width), int(height), int(stride))
	}
}

// NewDecoder initializes a hardware decoder using FFmpeg for Windows.
func NewDecoder(sps, pps []byte) (*Decoder, error) {
	if len(sps) == 0 || len(pps) == 0 {
		return nil, fmt.Errorf("invalid SPS or PPS")
	}

	cSPS := C.CBytes(sps)
	cPPS := C.CBytes(pps)
	defer C.free(cSPS)
	defer C.free(cPPS)

	winDecodersMu.Lock()
	winNextID++
	id := winNextID
	winDecodersMu.Unlock()

	userData := unsafe.Pointer(uintptr(id))

	ref := C.FFmpegDecoderCreate(
		(*C.uint8_t)(cSPS), C.size_t(len(sps)),
		(*C.uint8_t)(cPPS), C.size_t(len(pps)),
		(C.FFmpegFrameCallback)(C.goFrameCallbackWindows),
		userData,
	)

	if ref == nil {
		return nil, fmt.Errorf("failed to create FFmpeg decoder session")
	}

	d := &Decoder{
		ref: ref,
		id:  id,
	}

	winDecodersMu.Lock()
	winDecoders[id] = d
	winDecodersMu.Unlock()

	return d, nil
}

func (d *Decoder) DecodeNALU(nalu []byte) {
	if d.ref == nil || len(nalu) == 0 {
		return
	}

	// Unlike VideoToolbox which needs AVCC (Length prefix), FFmpeg expects Annex B (0x00 0x00 0x00 0x01 prefix)
	// Because gortsplib removed the prefix, we restore it for FFmpeg
	annexB := make([]byte, 4+len(nalu))
	annexB[0] = 0x00
	annexB[1] = 0x00
	annexB[2] = 0x00
	annexB[3] = 0x01
	copy(annexB[4:], nalu)

	cAnnexB := C.CBytes(annexB)
	defer C.free(cAnnexB)

	C.FFmpegDecoderDecode(d.ref, (*C.uint8_t)(cAnnexB), C.size_t(len(annexB)))
}

func (d *Decoder) Close() {
	if d.ref != nil {
		C.FFmpegDecoderDestroy(d.ref)
		d.ref = nil
	}
	winDecodersMu.Lock()
	delete(winDecoders, d.id)
	winDecodersMu.Unlock()
}
