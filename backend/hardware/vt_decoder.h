#ifndef VT_DECODER_H
#define VT_DECODER_H

#include <stdint.h>
#include <stddef.h>

// Opaque pointer to our decoder context
typedef void* VTDecoderRef;

// Callback signature for when a frame is decoded
typedef void (*VTFrameCallback)(void* user_data, uint8_t* bgra_data, int width, int height, int stride);

// Initialize the decoder with SPS and PPS
VTDecoderRef VTDecoderCreate(const uint8_t* sps, size_t sps_size, const uint8_t* pps, size_t pps_size, VTFrameCallback callback, void* user_data);

// Decode an AVCC formatted NALU (4 byte length header + NALU data)
void VTDecoderDecode(VTDecoderRef decoder, const uint8_t* nalu_avcc, size_t size);

// Destroy the decoder
void VTDecoderDestroy(VTDecoderRef decoder);

#endif
