package avformat

import (
	//"../rtmplog"
	"../util"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// FLV Tag Type
	FLV_TAG_TYPE_AUDIO  = 0x08
	FLV_TAG_TYPE_VIDEO  = 0x09
	FLV_TAG_TYPE_SCRIPT = 0x12
)

var (
	// 音频格式. 4 bit
	SoundFormat = map[byte]string{
		0:  "Linear PCM, platform endian",
		1:  "ADPCM",
		2:  "MP3",
		3:  "Linear PCM, little endian",
		4:  "Nellymoser 16kHz mono",
		5:  "Nellymoser 8kHz mono",
		6:  "Nellymoser",
		7:  "G.711 A-law logarithmic PCM",
		8:  "G.711 mu-law logarithmic PCM",
		9:  "reserved",
		10: "AAC",
		11: "Speex",
		14: "MP3 8Khz",
		15: "Device-specific sound"}

	// 采样频率. 2 bit
	SoundRate = map[byte]string{
		0: "5.5kHz",
		1: "11kHz",
		2: "22kHz",
		3: "44kHz"}

	// 量化精度. 1 bit
	SoundSize = map[byte]string{
		0: "8Bit",
		1: "16Bit"}

	// 音频类型. 1bit
	SoundType = map[byte]string{
		0: "Mono",
		1: "Stereo"}

	// 视频帧类型. 4bit
	FrameType = map[byte]string{
		1: "keyframe (for AVC, a seekable frame)",
		2: "inter frame (for AVC, a non-seekable frame)",
		3: "disposable inter frame (H.263 only)",
		4: "generated keyframe (reserved for server use only)",
		5: "video info/command frame"}

	// 视频编码类型. 4bit
	CodecID = map[byte]string{
		1: "JPEG (currently unused)",
		2: "Sorenson H.263",
		3: "Screen video",
		4: "On2 VP6",
		5: "On2 VP6 with alpha channel",
		6: "Screen video version 2",
		7: "AVC"}
)

type FLVStream struct {
	packet       *FLV
	audioTags    []FLVAudioTag
	videoTags    []FLVVideoTag
	aTagChan     chan *FLVAudioTag
	vTagChan     chan *FLVVideoTag
	sTagChan     chan *FLVScriptTag
	tagChan      chan *FLVTag
	headerChan   chan *FLVHeader
	videoWritten bool
}

func NewFLVStream() (flvs *FLVStream) {
	flvs = new(FLVStream)
	flvs.packet = new(FLV)
	flvs.aTagChan = make(chan *FLVAudioTag, 0)
	flvs.vTagChan = make(chan *FLVVideoTag, 0)
	flvs.sTagChan = make(chan *FLVScriptTag, 0)
	flvs.tagChan = make(chan *FLVTag, 0)
	flvs.headerChan = make(chan *FLVHeader, 0)
	return
}

type FLV struct {
	FLVHeader
	FLVBody
}

type FLVHeader struct {
	SignatureF         byte   // 8 bits  Signature byte always 'F' (0x46)
	SignatureL         byte   // 8 bits  Signature byte always 'L' (0x4C)
	SignatureV         byte   // 8 bits  Signature byte always 'V' (0x56)
	Version            byte   // 8 bits  File version (for example, 0x01 for FLV version 1)
	TypeFlagsReserved1 byte   // 5 bits  Must be 0
	TypeFlagsAudio     byte   // 1 bit   Audio tags are present
	TypeFlagsReserved2 byte   // 1 bit   Must be 0
	TypeFlagsVideo     byte   // 1 bit   Vudio tags are present
	DataOffse          uint32 // 32 bits Offset in bytes from start of file to start of body (that is, size of header)
}

// PreviousTagSize[0] Always 0, PreviousTagSize[N] Size of last tag.
type FLVBody struct {
	PreviousTagSize []uint32 // 32 bits Size of previous tag
	Tags            []FLVTag // Tags, Size of PreviousTagSize
}

// Tag DTS == Timestamp
// Tag PTS == Timestamp | TimestampExtended << 24
// 11 Bytes + Data(n)
type FLVTag struct {
	TagType           byte         // 8 bits  Type of this tag
	DataSize          uint32       // 24 bits Length of the data in the Data field, DataSize == len(Data) - 11
	Timestamp         uint32       // 24 bits Time in milliseconds at which the data in this tag applies. This value isrelative to the first tag in the FLV file, which always has a timestamp of 0.
	TimestampExtended byte         // 8 bits  Extension of the Timestamp field to form a SI32 value. This field represents the upper 8 bits, while the previous Timestamp field represents the lower
	StreamID          uint32       // 24 bits Always 0
	Data              bytes.Buffer // DataSize bits Body of the tag
}

type FLVAudioTag struct {
	FLVAudioTagHeader
	SoundData []byte
}

type FLVAudioTagHeader struct {
	SoundFormat byte // 4 bits Format of SoundData
	SoundRate   byte // 2 bits Sampling rate,For AAC: always 3
	SoundSize   byte // 1 bit  Size of each sample.
	SoundType   byte // 1 bit  Mono or stereo sound.Mono or stereo sound,For Nellymoser: always 0,For AAC: always 1
}

// if FLVAudioTag.FLVAudioTagHeader.SoundFormat == 10, AAC
type AACAudioData struct {
	AACPacketType byte   // 8 bits 0: AAC sequence heade, 1: AAC raw
	Data          []byte // n bits if AACPacketType == 0 AudioSpecificConfig, else if AACPacketType == 1 Raw AAC frame data
}

type FLVVideoTag struct {
	FLVVideoTagHeader
	VideoData []byte
}

type FLVVideoTagHeader struct {
	FrameType byte // 4 bits Type of video frame.
	CodecID   byte // 4 bits Codec Identifier
}

// if FLVVideoTag.FLVVideoTagHeader.CodecID == 7, AVC
// CompositionTime = (pts - dts) / 90 millisecond
type AVCVideoPacket struct {
	AVCPacketType   byte   // 8 bits  0: AVC sequence heade, 1: AVC NALU, 2: AVC end of sequence
	CompositionTime uint32 // 24 bits if AVCPacketType == 1 Composition time offset else 0
	Data            []byte // n bits  if AVCPacketType == 0 AVCDecoderConfigurationRecord, else if AVCPacketType == 1 One or more NALUs (can be individual slices per FLV packets; that is, full frames are not strictly required), else if AVCPacketType == 2 Empty
}

// Metadata Tag
type FLVScriptTag struct {
	AMF1 FLVScriptAMF1
	AMF2 FLVScriptAMF2
}

// 13 Byte
type FLVScriptAMF1 struct {
	AMFType      byte   // 8 bits
	StringLength uint16 // 16 bits 0x000A("onMetaData")
	StringData   string // "onMetaData"
}

type FLVScriptAMF2 struct {
	AMFType         byte        // 8 bits
	ECMAArrayLength uint32      // 32 bits
	Object          []AMFObject // ECMAArrayLength size
}

type AMFObject struct {
	ObjectLength uint16       // 16 bits
	ObjectString string       // objectLength bits
	ObjectType   uint32       // 24 bits
	ObjectValue  bytes.Buffer // The number of bytes depends on the ObjectType.
}

func (flvs *FLVStream) ReadPacket(r io.Reader) (header FLVHeader, err error) {
	var tag FLVTag
	var previousTagSize0 uint32
	header, err = ReadFLVHeader(r)
	if err != nil {
		return
	}

	flvs.headerChan <- &header

	// PreviousTagSize0 == 0x00000000
	previousTagSize0, err = util.ReadByteToUint32(r, true)
	if err != nil {
		return
	}

	if previousTagSize0 != 0x00000000 {
		err = errors.New("PreviousTagSize0 error!")
		return
	}

	for {
		tag, err = ReadFLVTag(r)
		if err == io.EOF {
			close(flvs.tagChan)
		}

		if err != nil {
			return
		}

		//fmt.Println("send tag data length:", tag.Data.Len())
		// 这里为什么要这么做,具体参考MPEGTS在传输PES包的时候的解释.
		flvs.tagChan <- &FLVTag{
			TagType:   tag.TagType,
			DataSize:  tag.DataSize,
			Timestamp: tag.Timestamp,
			StreamID:  tag.StreamID,
			Data:      tag.Data,
		}

		// PreviousTagSizeN
		_, err = util.ReadByteToUint32(r, true)
		if err != nil {
			return
		}

		switch tag.TagType {
		case FLV_TAG_TYPE_AUDIO:
			{
				var audioTagHeader byte
				aTag := &FLVAudioTag{}

				audioTagHeader, err = tag.Data.ReadByte()
				if err != nil {
					return
				}

				aTag.FLVAudioTagHeader.SoundFormat = (audioTagHeader & 0xF0) >> 4
				aTag.FLVAudioTagHeader.SoundRate = (audioTagHeader & 0x0C) >> 2
				aTag.FLVAudioTagHeader.SoundSize = (audioTagHeader & 0x02) >> 1
				aTag.FLVAudioTagHeader.SoundType = audioTagHeader & 0x01

				aTag.SoundData = make([]byte, tag.DataSize-1)

				_, err = tag.Data.Read(aTag.SoundData)
				if err != nil {
					return
				}

				flvs.audioTags = append(flvs.audioTags, *aTag)
				flvs.aTagChan <- aTag
			}
		case FLV_TAG_TYPE_VIDEO:
			{
				var videoTagHeader byte
				vTag := &FLVVideoTag{}

				videoTagHeader, err = tag.Data.ReadByte()
				if err != nil {
					return
				}

				vTag.FLVVideoTagHeader.FrameType = (videoTagHeader & 0xF0) >> 4
				vTag.FLVVideoTagHeader.CodecID = videoTagHeader & 0x0F

				vTag.VideoData = make([]byte, tag.DataSize-1)

				_, err = tag.Data.Read(vTag.VideoData)
				if err != nil {
					return
				}

				flvs.videoTags = append(flvs.videoTags, *vTag)
				flvs.vTagChan <- vTag
			}
		case FLV_TAG_TYPE_SCRIPT:
			{
				sTag := &FLVScriptTag{}

				flvs.sTagChan <- sTag
			}
		default:
			{
				fmt.Println("Unkonw Tag!")
			}
		}
	}

	return
}

func ReadFLVHeader(r io.Reader) (header FLVHeader, err error) {
	signatureFLV, err := util.ReadByteToUint24(r, true)
	if err != nil {
		return
	}

	header.SignatureF = byte((signatureFLV & 0xff0000) >> 16)
	header.SignatureL = byte((signatureFLV & 0xff00) >> 8)
	header.SignatureV = byte(signatureFLV & 0xff)

	if header.SignatureF != 0x46 && header.SignatureL != 0x4c && header.SignatureV != 0x56 {
		err = errors.New("flv header is not 'flv'")
		return
	}

	header.Version, err = util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	flags, err := util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	header.TypeFlagsAudio = flags & 0x20
	header.TypeFlagsVideo = flags & 0x80

	header.DataOffse, err = util.ReadByteToUint32(r, true)
	if err != nil {
		return
	}

	return
}

func ReadFLVTag(r io.Reader) (tag FLVTag, err error) {
	tag.TagType, err = util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	tag.DataSize, err = util.ReadByteToUint24(r, true)
	if err != nil {
		return
	}

	tag.Timestamp, err = util.ReadByteToUint24(r, true)
	if err != nil {
		return
	}

	tag.TimestampExtended, err = util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	tag.StreamID, err = util.ReadByteToUint24(r, true)
	if err != nil {
		return
	}

	lr := &io.LimitedReader{R: r, N: int64(tag.DataSize)}

	if _, err = io.CopyN(&tag.Data, lr, lr.N); err != nil {
		return
	}

	return
}

func WriteFLVHeader(w io.Writer, header FLVHeader) (err error) {
	h := uint32(header.SignatureF)<<24 + uint32(header.SignatureL)<<16 + uint32(header.SignatureV)<<8 + uint32(header.Version)
	if err = util.WriteUint32ToByte(w, h, true); err != nil {
		return
	}

	flags := uint8(header.TypeFlagsReserved1)<<3 + uint8(header.TypeFlagsAudio)<<2 + uint8(header.TypeFlagsReserved2)<<1 + uint8(header.TypeFlagsReserved1)
	if err = util.WriteUint8ToByte(w, flags); err != nil {
		return
	}

	if err = util.WriteUint32ToByte(w, header.DataOffse, true); err != nil {
		return
	}

	return
}

func WriteFLVTag(w io.Writer, tag FLVTag) (err error) {
	if err = util.WriteUint8ToByte(w, tag.TagType); err != nil {
		return
	}

	if err = util.WriteUint24ToByte(w, tag.DataSize, true); err != nil {
		return
	}

	if err = util.WriteUint24ToByte(w, tag.Timestamp, true); err != nil {
		return
	}

	if err = util.WriteUint8ToByte(w, tag.TimestampExtended); err != nil {
		return
	}

	if err = util.WriteUint24ToByte(w, tag.StreamID, true); err != nil {
		return
	}

	// Tag Data
	if _, err = w.Write(tag.Data.Bytes()); err != nil {
		return err
	}

	// PreviousTagSizeN(4)
	bb := util.BigEndian.ToUint32(tag.DataSize + 11)
	if _, err = w.Write(bb); err != nil {
		return err
	}

	return
}

func (flvs *FLVStream) TestRead() (err error) {
	/*
		defer func() {
			close(flvs.aTagChan)
			close(flvs.vTagChan)
			close(flvs.sTagChan)
		}()
	*/

	file, err := os.Open("in.flv")
	if err != nil {
		return
	}
	defer file.Close()

	if _, err = flvs.ReadPacket(file); err != nil {
		return
	}

	return
}

func (flvs *FLVStream) TestWrite() (err error) {
	file, err := os.OpenFile("out.flv", os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	header, ok := <-flvs.headerChan
	if !ok {
		return errors.New("header error")
	}

	if err = WriteFLVHeader(file, *header); err != nil {
		return
	}

	// PreviousTagSize0 == 0x00000000
	if err = util.WriteUint32ToByte(file, 0x00000000, true); err != nil {
		return
	}

	var aTags []FLVAudioTag
	var vTags []FLVVideoTag
	var sTags []FLVScriptTag

	for {
		select {
		case aTag := <-flvs.aTagChan:
			{
				// TODO:
				//fmt.Println("recv audio tag")
				aTags = append(aTags, *aTag)
			}
		case vTag := <-flvs.vTagChan:
			{
				// TODO:
				//fmt.Println("recv video tag")
				vTags = append(vTags, *vTag)
			}
		case sTag := <-flvs.sTagChan:
			{
				// TODO:
				//fmt.Println("recv script tag")
				sTags = append(sTags, *sTag)
			}
		case tag, ok := <-flvs.tagChan:
			{
				if !ok {
					// the last tag
					return
				}

				bw := &bytes.Buffer{}

				if err = WriteFLVTag(bw, *tag); err != nil {
					return
				}

				// TODO:这里是将每次的buffer拼接起来,最后面一次性写入?
				// 这里本来想用IOV来写入,虽然这里用了Byte.Buffer但是好像最终写入的时候也会对内存拷贝,因此会降低性能.
				// 而IOV可以避免内存拷贝.但是不知道为什么用IOV来写,写出的文件是错误的.这个还需要完善
				//body = append(body, bw.Bytes()...)

				file.Write(bw.Bytes())
			}
		case <-time.After(time.Second * 10):
			{
				fmt.Println("time out 10s")
				return
			}
		}
	}

	return
}

func (flvs *FLVStream) Test() (err error) {
	go flvs.TestRead()

	if err := flvs.TestWrite(); err != nil {
		return err
	}

	return nil
}
