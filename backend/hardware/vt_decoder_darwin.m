#ifdef __APPLE__

#import <Foundation/Foundation.h>
#import <VideoToolbox/VideoToolbox.h>
#import <CoreMedia/CoreMedia.h>
#import "vt_decoder_darwin.h"

typedef struct {
    VTDecompressionSessionRef session;
    CMVideoFormatDescriptionRef formatDesc;
    VTFrameCallback callback;
    void* user_data;
} DecoderContext;

static void decompressionOutputCallback(
    void *decompressionOutputRefCon,
    void *sourceFrameRefCon,
    OSStatus status,
    VTDecodeInfoFlags infoFlags,
    CVImageBufferRef imageBuffer,
    CMTime presentationTimeStamp,
    CMTime presentationDuration)
{
    if (status != noErr || !imageBuffer) return;

    DecoderContext* ctx = (DecoderContext*)decompressionOutputRefCon;
    
    // Lock the pixel buffer
    CVPixelBufferLockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
    
    uint8_t* baseAddress = (uint8_t*)CVPixelBufferGetBaseAddress(imageBuffer);
    size_t width = CVPixelBufferGetWidth(imageBuffer);
    size_t height = CVPixelBufferGetHeight(imageBuffer);
    size_t stride = CVPixelBufferGetBytesPerRow(imageBuffer);
    
    if (ctx->callback && baseAddress) {
        ctx->callback(ctx->user_data, baseAddress, (int)width, (int)height, (int)stride);
    }
    
    CVPixelBufferUnlockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
}

VTDecoderRef VTDecoderCreate(const uint8_t* sps, size_t sps_size, const uint8_t* pps, size_t pps_size, VTFrameCallback callback, void* user_data) {
    DecoderContext* ctx = (DecoderContext*)calloc(1, sizeof(DecoderContext));
    ctx->callback = callback;
    ctx->user_data = user_data;
    
    const uint8_t* parameterSetPointers[2] = { sps, pps };
    size_t parameterSetSizes[2] = { sps_size, pps_size };
    
    OSStatus status = CMVideoFormatDescriptionCreateFromH264ParameterSets(
        kCFAllocatorDefault,
        2, // parameter count
        parameterSetPointers,
        parameterSetSizes,
        4, // NAL unit header length (AVCC format)
        &ctx->formatDesc
    );
    
    if (status != noErr) {
        free(ctx);
        return NULL;
    }
    
    // We request BGRA pixel format to match NDI
    NSDictionary* destinationImageBufferAttributes = @{
        (id)kCVPixelBufferPixelFormatTypeKey: @(kCVPixelFormatType_32BGRA),
        // Let VideoToolbox choose width/height from the format description
    };
    
    VTDecompressionOutputCallbackRecord callbackRecord;
    callbackRecord.decompressionOutputCallback = decompressionOutputCallback;
    callbackRecord.decompressionOutputRefCon = ctx;
    
    status = VTDecompressionSessionCreate(
        kCFAllocatorDefault,
        ctx->formatDesc,
        NULL,
        (__bridge CFDictionaryRef)destinationImageBufferAttributes,
        &callbackRecord,
        &ctx->session
    );
    
    if (status != noErr) {
        CFRelease(ctx->formatDesc);
        free(ctx);
        return NULL;
    }
    
    return (VTDecoderRef)ctx;
}

void VTDecoderDecode(VTDecoderRef decoder, const uint8_t* nalu_avcc, size_t size) {
    DecoderContext* ctx = (DecoderContext*)decoder;
    if (!ctx || !ctx->session) return;
    
    CMBlockBufferRef blockBuffer = NULL;
    OSStatus status = CMBlockBufferCreateWithMemoryBlock(
        kCFAllocatorDefault,
        (void*)nalu_avcc,
        size,
        kCFAllocatorNull,
        NULL,
        0,
        size,
        0,
        &blockBuffer
    );
    
    if (status != noErr) return;
    
    CMSampleBufferRef sampleBuffer = NULL;
    const size_t sampleSizeArray[] = { size };
    status = CMSampleBufferCreateReady(
        kCFAllocatorDefault,
        blockBuffer,
        ctx->formatDesc,
        1,
        0, NULL,
        1, sampleSizeArray,
        &sampleBuffer
    );
    
    if (status == noErr && sampleBuffer) {
        VTDecodeInfoFlags infoFlagsOut;
        VTDecompressionSessionDecodeFrame(
            ctx->session,
            sampleBuffer,
            kVTDecodeFrame_EnableAsynchronousDecompression,
            NULL,
            &infoFlagsOut
        );
        CFRelease(sampleBuffer);
    }
    
    if (blockBuffer) {
        CFRelease(blockBuffer);
    }
}

void VTDecoderDestroy(VTDecoderRef decoder) {
    DecoderContext* ctx = (DecoderContext*)decoder;
    if (ctx) {
        if (ctx->session) {
            VTDecompressionSessionInvalidate(ctx->session);
            CFRelease(ctx->session);
        }
        if (ctx->formatDesc) {
            CFRelease(ctx->formatDesc);
        }
        free(ctx);
    }
}

#endif
