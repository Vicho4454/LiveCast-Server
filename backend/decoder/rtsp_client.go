package decoder

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"SERVLOC-TEST/backend/hardware"
	"SERVLOC-TEST/backend/ndi"
)

type Pipeline struct {
	client     *gortsplib.Client
	hwDecoder  *hardware.Decoder
	ndiEncoder *ndi.Encoder
	streamName string
	OnFrame    func(data []byte, width, height, stride int)
}

// NewPipeline creates a new decoding and translation pipeline
func NewPipeline(streamName string, ndiEnc *ndi.Encoder) *Pipeline {
	return &Pipeline{
		streamName: streamName,
		ndiEncoder: ndiEnc,
	}
}

// Start connects to the local RTSP server, captures H.264 NALUs, decodes them via VideoToolbox, and sends to NDI
func (p *Pipeline) Start() error {
	u, err := base.ParseURL("rtsp://127.0.0.1:8554/" + p.streamName)
	if err != nil {
		return err
	}

	p.client = &gortsplib.Client{
		Scheme: u.Scheme,
		Host:   u.Host,
	}

	err = p.client.Start()
	if err != nil {
		return err
	}

	session, _, err := p.client.Describe(u)
	if err != nil {
		p.client.Close()
		return err
	}

	// Find H264 track
	var h264Format *format.H264
	var videoMedia *description.Media
	for _, m := range session.Medias {
		for _, f := range m.Formats {
			if h264, ok := f.(*format.H264); ok {
				h264Format = h264
				videoMedia = m
				break
			}
		}
	}

	if h264Format == nil {
		p.client.Close()
		return fmt.Errorf("no H264 track found")
	}

	// Check if SPS/PPS are available in the SDP
	var sps, pps []byte
	if h264Format.SPS != nil {
		sps = h264Format.SPS
	}
	if h264Format.PPS != nil {
		pps = h264Format.PPS
	}

	// Initialize Hardware Decoder
	p.hwDecoder, err = hardware.NewDecoder(sps, pps)
	if err != nil {
		p.client.Close()
		return fmt.Errorf("failed to init hardware decoder: %v", err)
	}

	// Connect decoded frame callback to NDI encoder
	p.hwDecoder.OnFrameDecoded = func(data []byte, width, height, stride int) {
		p.ndiEncoder.SendVideoFrame(data, width, height, stride)
		if p.OnFrame != nil {
			p.OnFrame(data, width, height, stride)
		}
	}

	// Setup RTP packet decoder
	rtpDec, err := h264Format.CreateDecoder()
	if err != nil {
		p.hwDecoder.Close()
		p.client.Close()
		return fmt.Errorf("failed to create RTP decoder: %v", err)
	}

	_, err = p.client.Setup(session.BaseURL, videoMedia, 0, 0)
	if err != nil {
		p.hwDecoder.Close()
		p.client.Close()
		return err
	}

	// Read packets and push to hardware decoder
	p.client.OnPacketRTP(videoMedia, h264Format, func(pkt *rtp.Packet) {
		nalus, err := rtpDec.Decode(pkt)
		if err == nil {
			for _, nalu := range nalus {
				p.hwDecoder.DecodeNALU(nalu)
			}
		}
	})

	_, err = p.client.Play(nil)
	if err != nil {
		p.hwDecoder.Close()
		p.client.Close()
		return err
	}

	return nil
}

// Close stops the pipeline and releases resources
func (p *Pipeline) Close() {
	if p.client != nil {
		p.client.Close()
	}
	if p.hwDecoder != nil {
		p.hwDecoder.Close()
	}
}
