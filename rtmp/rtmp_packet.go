package rtmp

import (
	"../avformat"
	"fmt"
)

// Video or Audio
type AVPacket struct {
	Timestamp uint32
	Type      byte //8 audio,9 video

	// Video
	VideoFrameType byte //4bit
	VideoCodecID   byte //4bit

	// Audio
	SoundFormat byte //4bit
	SoundRate   byte //2bit
	SoundSize   byte //1bit
	SoundType   byte //1bit

	Payload []byte
}

func (av *AVPacket) Clone() *AVPacket {
	pkt := new(AVPacket)
	pkt.Timestamp = av.Timestamp
	pkt.Type = av.Type

	// Video
	pkt.VideoFrameType = av.VideoFrameType
	pkt.VideoCodecID = av.VideoCodecID

	// Auido
	pkt.SoundFormat = av.SoundFormat
	pkt.SoundRate = av.SoundRate
	pkt.SoundSize = av.SoundSize
	pkt.SoundType = av.SoundType

	pkt.Payload = av.Payload

	return pkt
}

func (av *AVPacket) String() string {
	if av.Type == RTMP_MSG_AUDIO {
		return fmt.Sprintf("Audio Packet Timestamp/%v Type/%v SoundFormat/%v SoundRate/%v SoundSize/%v SoundTypet/%v Payload/%v", av.Timestamp, av.Type, avformat.SoundFormat[av.SoundFormat], avformat.SoundRate[av.SoundRate], avformat.SoundSize[av.SoundSize], avformat.SoundType[av.SoundType], len(av.Payload))
	} else if av.Type == RTMP_MSG_VIDEO {
		return fmt.Sprintf("Video Packet Timestamp/%v Type/%v VideoFrameType/%v VideoCodecID/%v Payload/%v", av.Timestamp, av.Type, avformat.FrameType[av.VideoFrameType], avformat.CodecID[av.VideoCodecID], len(av.Payload))
	}

	return fmt.Sprintf("StreamPacket Timestamp/%v Type/%v Payload/%v", av.Timestamp, av.Type, len(av.Payload))
}

func (av *AVPacket) isKeyFrame() bool {
	return av.VideoFrameType == 1 || av.VideoFrameType == 4
}
