package rtmp

import (
	"../avformat"
	"../config"
	"../hls"
	"../mpegts"
	"../util"
	"bytes"
	"fmt"
	"io"
	"os"
	//"reflect"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// http://help.adobe.com/zh_CN/FlashPlatform/reference/actionscript/3/flash/net/NetStream.html

type NetStream interface {
	AttachVideo(video chan *AVPacket)                   // 将视频流从作为源传递的 Microphone 对象附加到 NetStream 对象
	AttachAudio(audio chan *AVPacket)                   // 将音频流从作为源传递的 Microphone 对象附加到 NetStream 对象
	Publish(streamName string, streamType string) error // 将流音频、视频和数据消息从客户端发送到 Flash Media Server,可以选择在传输期间录制此流
	Play(streamName string, args ...interface{}) error  // 从本地目录或 Web 服务器播放媒体文件:从 Flash Media Server 播放媒体文件或实时流. streamName string, start uint64,length uint64,reset bool
	Pause()                                             // 暂停视频流的播放
	Resume()                                            // 恢复播放暂停的视频流
	TogglePause()                                       // 暂停或恢复流的播放
	Seek(offset uint64)                                 // 搜索与指定位置最接近的关键帧(在视频行业中也称为 I 帧)
	Send(handlerName string, args ...interface{})       // 在发布的流上向所有订阅客户端发送一条消息
	Close()                                             // 停止播放流上的所有数据,将 time 属性设置为 0,并使该流可用于其他用途
	ReceiveAudio(flag bool)                             // 指定传入的音频是否在流上播放(true),还是不播放(false)
	ReceiveVideo(flag bool)                             // 指定传入的视频是否在流上播放(true),还是不播放(false)
	NetConnection() NetConnection                       // 创建可以用来通过 NetConnection 对象播放媒体文件和发送数据的流
	BufferTime() time.Duration                          // 指定在开始显示流之前需要多长时间将消息存入缓冲区
	BytesLoaded() uint64                                // [只读] 已加载到应用程序中的数据的字节数
	BufferLength() uint64                               // [只读] 数据当前存在于缓冲区中的秒数
}

// NetStream对象类似于NetConnection对象中的一个通道.
// 这个通道可以使用NetStream.publish()发布音频和/或视频数据,也可以使用 NetStream.play()订阅已发布的流并接收数据.
// 定义了传输通道,通过这个通道,音频流、视频流以及数据消息流可以通过连接客户端到服务端的NetConnection传输.
type RtmpNetStream struct {
	conn           *RtmpNetConnection // NetConnection
	metaData       *AVPacket          // metedata
	videoTag       *AVPacket          // 每个视频包都是这样的结构,区别在于Payload的大小.FMS在发送AVC sequence header,需要加上 VideoTags,这个tag 1个字节(8bits)的数据
	audioTag       *AVPacket          // 每个音频包都是这样的结构,区别在于Payload的大小.FMS在发送AAC sequence header,需要加上 AudioTags,这个tag 1个字节(8bits)的数据
	videoKeyFrame  *AVPacket          // keyframe (for AVC, a seekableframe)（关键帧）
	videochan      chan *AVPacket     // live video chan
	audiochan      chan *AVPacket     // live audio chan
	streamPath     string             // 客户端推流的路径.(例如rtmp://192.168.2.1/myapp/mystream,那么流路径就是myapp/mystream)
	bufferTime     time.Duration      // 指定在开始显示流之前需要多长时间将消息存入缓冲区
	bufferLength   uint64             // [read-only] 数据当前存在于缓冲区中的秒数
	bufferLoad     uint64             // [read-only] 已加载到播放器中的数据的字节数
	lock           *sync.Mutex        // guards the following
	serverHandler  ServerHandler      // 服务器对客户端命令消息的响应处理
	mode           int                // mode
	vkfsended      bool               // video key frames, init false 是否发送了第一个视频帧
	akfsended      bool               // audio key frames, init false 是否发送了第一个音频帧
	vstreamToFile  bool               // 是否可以将视频流保存为文件
	astreamToFile  bool               // 是否可以将音频流保存为文件
	broadcast      *Broadcast         // Broadcast, 装载着需要广播的对象.(也即是具体的发布者)
	total_duration uint32             // media total duration 存放音频或者视频的时间戳累加.就是一个绝对时间戳. 当前绝对时间戳 = 上一个绝对时间戳 + 当前相对时间戳
	vsend_time     uint32             // 上一个视频的绝对时间戳
	asend_time     uint32             // 上一个音频的绝对时间戳
	closed         bool               // 是否关闭
	rtmpFile       *RtmpFile          // netstream write file
}

func newNetStream(conn *RtmpNetConnection, sh ServerHandler) (s *RtmpNetStream) {
	s = new(RtmpNetStream)
	s.conn = conn
	s.lock = new(sync.Mutex)
	s.serverHandler = sh
	//s.clientHandler = ch
	s.vkfsended = false
	s.akfsended = false
	s.vstreamToFile = false
	s.astreamToFile = false
	return
}

func (s *RtmpNetStream) AttachVideo(video chan *AVPacket) {
	s.videochan = video
}

func (s *RtmpNetStream) AttachAudio(audio chan *AVPacket) {
	s.audiochan = audio
}

// 先发送关键帧(Tag),之后就不断发送数据
func (s *RtmpNetStream) SendVideo(video *AVPacket) error {
	// 这里发送时间戳的依据是,当发送第一个包和Tag的时候,需要发送Chunk12的头
	// 因此这里TimeStamp我们简单的设置为0(指明一个时间而已)
	// 到了这里,不在需要再发送Chunk12的头,只需要发送Chunk4或者Chunk8的头.
	// 因此这里的时间戳,应该是一个TimeStamp Delta,记录与上一个Chunk的时间差值.
	if s.vkfsended {
		video.Timestamp -= s.vsend_time - uint32(s.bufferTime)
		s.vsend_time += video.Timestamp
		//fmt.Println("time stamp:", video.Timestamp)
		//fmt.Println("video send time:", s.vsend_time)
		return sendMessage(s.conn, SEND_VIDEO_MESSAGE, video)
	}

	if !video.isKeyFrame() {
		return nil
	}

	vTag := s.broadcast.publisher.videoTag // 从发布者发布的数据中,拿出视频Tag.
	if vTag == nil {
		fmt.Println("Video Tag nil")
		return nil
	}

	vTag.Timestamp = 0

	// 如果视频的格式是AVC(H.264)的话,VideoTagHeader(1个字节)会多出4个字节的信息.AVCPacketType(1Bytes)和 CompositionTime(3 Bytes).
	// AVCPacketType表示接下来VIDEODATA(AVCVIDEOPACKET)的内容.
	// if AVCPacketType == 0 -> AVCDecoderConfigurationRecord 并且 frametype 必须为1.
	// AVCDecoderConfigurationRecord
	// AVC sequence header就是AVCDecoderConfigurationRecord结构，该结构在标准文档“ISO-14496-15 AVC file format”中有详细说明
	version := vTag.Payload[4+1]                                            // configurationVersion, 8bit. 版本号 1
	avcPfofile := vTag.Payload[4+2]                                         // AVCProfileIndication, 8bit. sps[1]
	profileCompatibility := vTag.Payload[4+3]                               // profile_compatibility, 8bit. sps[2]
	avcLevel := vTag.Payload[4+4]                                           // AVCLevelIndication, 8bit. sps[3]
	reserved := vTag.Payload[4+5] >> 2                                      // reserved, 6bit. reserved 111111
	lengthSizeMinusOne := vTag.Payload[4+5] & 0x03                          // lengthSizeMinusOne, 2bit. H.264 视频中 NALU 的长度,一般为3
	reserved2 := vTag.Payload[4+6] >> 5                                     // reserved, 3bit. reserved 111
	numOfSPS := vTag.Payload[4+6] & 31                                      // numOfSequenceParameterSets, 5bit. sps个数,一般为1
	spsLength := util.BigEndian.Uint16(vTag.Payload[4+7:])                  // sequenceParameterSetLength, 16bit, sps长度.
	sps := vTag.Payload[4+9 : 4+9+int(spsLength)]                           // sps
	numOfPPS := vTag.Payload[4+9+int(spsLength)]                            // numOfPictureParameterSets, 8bit, pps个数,一般为1
	ppsLength := util.BigEndian.Uint16(vTag.Payload[4+9+int(spsLength)+1:]) // pictureParameterSetLength, 16bit, pps长度
	pps := vTag.Payload[4+9+int(spsLength)+1+2:]                            // pps

	// AVCDecoderConfigurationRecord 后面就是 NALUs.
	fmt.Sprintf("ConfigurationVersion:%v\nAVCProfileIndication:%v\nprofile_compatibility/%v\nAVCLevelIndication/%v\nreserved/%v\nlengthSizeMinusOne/%v\nreserved2/%v\nNumOfSequenceParameterSets/%v\nSequenceParameterSetLength/%v\nSPS/%02x\nNumOfPictureParameterSets/%v\nPictureParameterSetLength/%v\nPPS/%02x\n",
		version, avcPfofile, profileCompatibility, avcLevel, reserved, lengthSizeMinusOne, reserved2, numOfSPS, spsLength, sps, numOfPPS, ppsLength, pps)

	err := sendMessage(s.conn, SEND_FULL_VDIEO_MESSAGE, vTag)
	if err != nil {
		return err
	}

	s.vkfsended = true
	s.vsend_time = video.Timestamp
	video.Timestamp = 0

	return sendMessage(s.conn, SEND_FULL_VDIEO_MESSAGE, video)
}

// 先发送关键帧(Tag),之后就不断发送数据
func (s *RtmpNetStream) SendAudio(audio *AVPacket) error {
	if !s.vkfsended { // 先发送视频才开始发送音频
		return nil
	}

	//
	// | audio key frame| audio data 1| audio data 2| audio data 3| ... |
	//
	// 绝对时间戳: 0 -> | 1000		  | 9000		| 17000		  | ... |
	//
	// 相对时间戳: 0 ->	| 8000		  | 8000		| 8000		  | ...	|
	if s.akfsended {
		audio.Timestamp -= s.asend_time - uint32(s.bufferTime) // 当前音频相对时间戳 == 当前音频绝对时间戳 - 上一个音频绝对时间戳.buffer time == 0, asend_time总是保存的是上一个音频绝对时间戳.
		s.asend_time += audio.Timestamp                        // 当前音频的绝对时间戳 = 上一个音频的绝对时间戳 + 当前音频的相对时间戳. audio.Timestamp 总是保存当前音频的相对时间戳
		return sendMessage(s.conn, SEND_AUDIO_MESSAGE, audio)  // 这里发送的时间戳是相对时间戳
	}

	// FMS推送H264和AAC直播流,需要首先发送"AVC sequence header"和"AAC sequence header",这两项数据包含的是重要的编码信息,没有它们,解码器将无法解码.
	// 在发送这两个header需要在前面分别加上 VideoTags、AudioTags  这两个个tags都是1个字节（8bits）的数据
	// Audio Tag == SoundFormat(4 Bit) + SoundRate(2 Bit) + SoundSize(1 Bit) + SoundTypet(1 Bit)
	aTag := s.broadcast.publisher.audioTag // 从发布者发布的数据中,拿出音频Tag.
	aTag.Timestamp = 0

	err := sendMessage(s.conn, SEND_FULL_AUDIO_MESSAGE, aTag) // 发送音频Tag.
	if err != nil {
		return err
	}

	s.akfsended = true                                         // 标示第一个完整的包已经发送
	s.asend_time = audio.Timestamp                             // 音频发送时间,接收到客户端的音频消息中,会获取该值
	audio.Timestamp = 0                                        // 音频时间戳,初始化为0
	return sendMessage(s.conn, SEND_FULL_AUDIO_MESSAGE, audio) // 发送第一个完整的音频包
}

func (s *RtmpNetStream) WriteVideo(w io.Writer, video *AVPacket, fileType int) (err error) {
	switch fileType {
	case RTMP_FILE_TYPE_ES_H264:
		{
			if s.rtmpFile.vtwrite {
				if _, err = w.Write(video.Payload); err != nil {
					return
				}

				return nil
			}

			if _, err = w.Write(s.videoTag.Payload); err != nil {
				return
			}

			s.rtmpFile.vtwrite = true
		}
	case RTMP_FILE_TYPE_TS:
		{
			if s.rtmpFile.vtwrite {
				var packet mpegts.MpegTsPESPacket
				if packet, err = rtmpVideoPacketToPES(video, s.rtmpFile.avc); err != nil {
					return
				}

				frame := new(mpegts.MpegtsPESFrame)
				frame.Pid = 0x101
				frame.IsKeyFrame = video.isKeyFrame()
				frame.ContinuityCounter = byte(s.rtmpFile.video_cc % 16)
				frame.ProgramClockReferenceBase = uint64(video.Timestamp) * 90
				if err = mpegts.WritePESPacket(w, frame, packet); err != nil {
					return
				}

				s.rtmpFile.video_cc = uint16(frame.ContinuityCounter)

				return nil
			}

			if s.rtmpFile.avc, err = decodeAVCDecoderConfigurationRecord(s.videoTag.Clone()); err != nil {
				return
			}

			if err = mpegts.WriteDefaultPATPacket(w); err != nil {
				return
			}

			if err = mpegts.WriteDefaultPMTPacket(w); err != nil {
				return
			}

			s.rtmpFile.vtwrite = true
		}
	case RTMP_FILE_TYPE_HLS_TS:
		{
			if s.rtmpFile.vtwrite {
				var packet mpegts.MpegTsPESPacket
				if packet, err = rtmpVideoPacketToPES(video, s.rtmpFile.avc); err != nil {
					return
				}

				if video.isKeyFrame() {
					// 当前的时间戳减去上一个ts切片的时间戳
					if int64(video.Timestamp-s.rtmpFile.vwrite_time) >= s.rtmpFile.hls_fragment {
						//fmt.Println("time :", video.Timestamp, tsSegmentTimestamp)

						tsFilename := strings.Split(s.streamPath, "/")[1] + "-" + strconv.FormatInt(time.Now().Unix(), 10) + ".ts"

						if err = writeHlsTsSegmentFile(s.rtmpFile.hls_path+"/"+tsFilename, s.rtmpFile.hls_segment_data.Bytes()); err != nil {
							return
						}

						inf := hls.PlaylistInf{
							Duration: float64((video.Timestamp - s.rtmpFile.vwrite_time) / 1000),
							Title:    tsFilename,
						}

						if s.rtmpFile.hls_segment_count >= uint32(config.HLSWindow) {
							if err = s.rtmpFile.hls_playlist.UpdateInf(s.rtmpFile.hls_m3u8_name, s.rtmpFile.hls_m3u8_name+".tmp", inf); err != nil {
								return
							}
						} else {
							if err = s.rtmpFile.hls_playlist.WriteInf(s.rtmpFile.hls_m3u8_name, inf); err != nil {
								return
							}
						}

						s.rtmpFile.hls_segment_count++
						s.rtmpFile.vwrite_time = video.Timestamp
						s.rtmpFile.hls_segment_data.Reset()
					}
				}

				frame := new(mpegts.MpegtsPESFrame)
				frame.Pid = 0x101
				frame.IsKeyFrame = video.isKeyFrame()
				frame.ContinuityCounter = byte(s.rtmpFile.video_cc % 16)
				frame.ProgramClockReferenceBase = uint64(video.Timestamp) * 90
				if err = mpegts.WritePESPacket(s.rtmpFile.hls_segment_data, frame, packet); err != nil {
					return
				}

				s.rtmpFile.video_cc = uint16(frame.ContinuityCounter)

				return nil
			}

			if s.rtmpFile.avc, err = decodeAVCDecoderConfigurationRecord(s.videoTag.Clone()); err != nil {
				return
			}

			if config.HLSFragment > 0 {
				s.rtmpFile.hls_fragment = config.HLSFragment * 1000
			} else {
				s.rtmpFile.hls_fragment = 10000
			}

			s.rtmpFile.hls_playlist = hls.Playlist{
				Version:        3,
				Sequence:       0,
				Targetduration: int(s.rtmpFile.hls_fragment / 666), // hlsFragment * 1.5 / 1000
			}

			s.rtmpFile.hls_path = config.HLSPath + "/" + strings.Split(s.streamPath, "/")[0]
			s.rtmpFile.hls_m3u8_name = s.rtmpFile.hls_path + "/mystream.m3u8"

			if !util.Exist(s.rtmpFile.hls_path) {
				if err = os.Mkdir(s.rtmpFile.hls_path, os.ModePerm); err != nil {
					return
				}
			}

			if err = s.rtmpFile.hls_playlist.Init(s.rtmpFile.hls_m3u8_name); err != nil {
				fmt.Println(err)
				return
			}

			s.rtmpFile.hls_segment_data = &bytes.Buffer{}
			s.rtmpFile.hls_segment_count = 0

			s.rtmpFile.vtwrite = true
		}
	case RTMP_FILE_TYPE_FLV:
		{
			if s.rtmpFile.vtwrite {
				if err = writeFLVTag(w, video); err != nil {
					return
				}

				return nil
			}

			header := avformat.FLVHeader{
				SignatureF:     0x46,
				SignatureL:     0x4C,
				SignatureV:     0x56,
				Version:        0x01,
				TypeFlagsAudio: 1,
				TypeFlagsVideo: 1,
				DataOffse:      9,
			}

			if err = avformat.WriteFLVHeader(w, header); err != nil {
				return
			}

			// PreviousTagSize0 == 0x00000000
			if err = util.WriteUint32ToByte(w, 0x00000000, true); err != nil {
				return
			}

			if err = writeFLVTag(w, s.videoTag.Clone()); err != nil {
				return
			}

			s.rtmpFile.vtwrite = true
		}
	case RTMP_FILE_TYPE_MP4:
		{
		}
	default:
		{
			err = errors.New("unknow video file type.")
			return
		}
	}

	return nil
}

func (s *RtmpNetStream) WriteAudio(w io.Writer, audio *AVPacket, fileType int) (err error) {
	switch fileType {
	case RTMP_FILE_TYPE_ES_AAC:
		{
			if s.rtmpFile.atwrite {
				if _, err = w.Write(audio.Payload); err != nil {
					return
				}

				return nil
			}

			if _, err = w.Write(s.audioTag.Payload); err != nil {
				return
			}

			s.rtmpFile.atwrite = true
		}
	case RTMP_FILE_TYPE_TS:
		{
			if !s.rtmpFile.vtwrite {
				return nil
			}

			if s.rtmpFile.atwrite {
				var packet mpegts.MpegTsPESPacket
				if packet, err = rtmpAudioPacketToPES(audio, s.rtmpFile.asc); err != nil {
					return
				}

				frame := new(mpegts.MpegtsPESFrame)
				frame.Pid = 0x102
				frame.IsKeyFrame = false
				frame.ContinuityCounter = byte(s.rtmpFile.audio_cc % 16)
				//frame.ProgramClockReferenceBase = 0
				if err = mpegts.WritePESPacket(w, frame, packet); err != nil {
					return
				}

				s.rtmpFile.audio_cc = uint16(frame.ContinuityCounter)

				return nil
			}

			if s.rtmpFile.asc, err = decodeAudioSpecificConfig(s.audioTag.Clone()); err != nil {
				return
			}

			s.rtmpFile.atwrite = true
		}
	case RTMP_FILE_TYPE_HLS_TS:
		{
			if !s.rtmpFile.vtwrite {
				return nil
			}

			if s.rtmpFile.atwrite {
				var packet mpegts.MpegTsPESPacket
				if packet, err = rtmpAudioPacketToPES(audio, s.rtmpFile.asc); err != nil {
					return
				}

				frame := new(mpegts.MpegtsPESFrame)
				frame.Pid = 0x102
				frame.IsKeyFrame = false
				frame.ContinuityCounter = byte(s.rtmpFile.audio_cc % 16)
				//frame.ProgramClockReferenceBase = 0
				if err = mpegts.WritePESPacket(s.rtmpFile.hls_segment_data, frame, packet); err != nil {
					return
				}

				s.rtmpFile.audio_cc = uint16(frame.ContinuityCounter)

				return nil
			}

			if s.rtmpFile.asc, err = decodeAudioSpecificConfig(s.audioTag.Clone()); err != nil {
				return
			}

			s.rtmpFile.atwrite = true
		}
	case RTMP_FILE_TYPE_FLV:
		{
			if !s.rtmpFile.vtwrite {
				return nil
			}

			if s.rtmpFile.atwrite {
				if err = writeFLVTag(w, audio); err != nil {
					return
				}

				return nil
			}

			if err = writeFLVTag(w, s.audioTag.Clone()); err != nil {
				return
			}

			s.rtmpFile.atwrite = true
		}
	case RTMP_FILE_TYPE_MP4:
		{
		}
	default:
		{
			err = errors.New("unknow audio file type.")
			return
		}
	}

	return nil
}

func (s *RtmpNetStream) Close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return
	}

	s.conn.Close()
	s.closed = true
	s.serverHandler.OnClosed(s)
}

// Client publish -> FFmpeg
// Client subscriber -> FFplay

// Client publish stream : ReleaseStreamMessage -> FCPublishMessage  -> CreateStreamMessage  -> PublishMessage -> MetadataMessage
// Client unpublish stream : FCUnpublishMessage -> DeleteStreamMessage

// Client subscriber stream : CreateStreamMessage -> PlayMessage
// Client unsubscriber stream : DeleteStreamMessage

// Client subscriber -> VLC

// Client subscriber stream : CreateStreamMessage -> GetStreamLengthMessage -> PlayMessage

// Publishing the Live Stream:

// Adobe recommends the following call sequence which mimics FMLE:
// NetConnection.Connect.Success -> releaseStream -> FCPublish -> NetStream.Publish.Start -> publish
// Once the client (encoder) has successfully authenticated with the ingest server, it must:
// 1) First remove the stream from the server.
// 2) Then notify the ingest server of the intent to publish the live stream, via an FCPublish call.
// 3) Then send a message to the Flash Media Server.
// 4) Then specify a callback function, onFCPublish, to receive the ingest server’s response.
// 5) Wait for a response from the ingest server. A response code of NetStream.Publish.Start indicates that the server is ready to receive the stream. The client can then publish the stream as shown below

// Unpublishing the Live Stream :

// Similar to the publishing section above, when the client is ready to stop the live stream, it must:
// 1) First remove the stream from the server.
// 2) Then notify the ingest server of the intent to unpublish the live stream, via an FCUnpublish call.
// 3) Then specify a callback function, onFCUnpublish, to receive the ingest server’s response.
// 4) Wait for a response from the ingest server. A response code of NetStream.Unpublish.Success indicates that the stream was successfully unpublished. The stream resources have been deallocated by the server, and the client can proceed to remove its local stream resources.

func (s *RtmpNetStream) msgLoopProc() {
	for {
		msg, err := recvMessage(s.conn)
		if err != nil {
			s.serverHandler.OnError(s, err)
			break
		}

		if msg.Header().ChunkMessgaeHeader.MessageLength <= 0 {
			continue
		}

		// 调试模式下,打印各种消息
		if config.DebugMode {
			msgID := msg.Header().ChunkMessgaeHeader.MessageTypeID
			if msgID == RTMP_MSG_AUDIO || msgID == RTMP_MSG_VIDEO {
			} else {
				fmt.Println(msg.String())
			}
		}

		switch v := msg.(type) {
		case *AudioMessage:
			{
				audioMessageHandle(s, v)
			}
		case *VideoMessage:
			{
				videoMessageHandle(s, v)
			}
		case *MetadataMessage:
			{
				metadataMessageHandle(s, v)
				//decodeMetadataMessage(s.metaData)
			}
		case *CreateStreamMessage:
			{
				if err := createStreamMessageHandle(s, v); err != nil {
					s.serverHandler.OnError(s, err)
					return
				}
			}
		case *PublishMessage:
			{
				if err := publishMessageHandle(s, v); err != nil {
					s.serverHandler.OnError(s, err)
					return
				}
			}
		case *PlayMessage:
			{
				if err := playMessageHandle(s, v); err != nil {
					s.serverHandler.OnError(s, err)
					return
				}
			}
		case *ReleaseStreamMessage:
			{
				// TODO:
			}
		case *CloseStreamMessage:
			{
				s.Close()
			}
		case *FCPublishMessage:
			{
				// // TODO
				// if err := fcPublishMessageHandle(s, v); err != nil {
				// 	return
				// }
			}
		case *FCUnpublishMessage:
			{
				// // TODO
				// if err := fcUnPublishMessageHandle(s, v); err != nil {
				// 	return
				// }
			}
		default:
			{
				fmt.Println("Other Message :", v)
			}
		}
	}
}

func audioMessageHandle(s *RtmpNetStream, audio *AudioMessage) {
	pkt := new(AVPacket)
	if audio.RtmpHeader.ChunkMessgaeHeader.Timestamp == 0xffffff {
		s.total_duration += audio.RtmpHeader.ChunkExtendedTimestamp.ExtendTimestamp
		pkt.Timestamp = s.total_duration
	} else {
		s.total_duration += audio.RtmpHeader.ChunkMessgaeHeader.Timestamp // 绝对时间戳
		pkt.Timestamp = s.total_duration                                  // 当前时间戳(用绝对时间戳做当前音频包的时间戳)
	}

	//fmt.Println("recv audio time stamp:", pkt.Timestamp)

	pkt.Type = audio.RtmpHeader.ChunkMessgaeHeader.MessageTypeID
	pkt.Payload = audio.RtmpBody.Payload

	tmp := pkt.Payload[0]             // 第一个字节保存着音频的相关信息
	pkt.SoundFormat = tmp >> 4        // 音频格式 AAC或者其他.客户端在发送的时候,会左移4位,因此接收到后,右移4位
	pkt.SoundRate = (tmp & 0x0c) >> 2 // 采样率 0 = 5.5 kHz or 1 = 11 kHz or 2 = 22 kHz or 3 = 44 kHz
	pkt.SoundSize = (tmp & 0x02) >> 1 // 采样精度 0 = 8-bit samples or 1 = 16-bit samples
	pkt.SoundType = tmp & 0x01        // 音频类型 0 = Mono sound or 1 = Stereo sound

	if s.audioTag == nil { // (AAC Header(2 Bytes) + AAC sequence Header(2 Bytes))
		s.audioTag = pkt
	} else {
		s.audiochan <- pkt
	}
}

func videoMessageHandle(s *RtmpNetStream, video *VideoMessage) {
	pkt := new(AVPacket)
	if video.RtmpHeader.ChunkMessgaeHeader.Timestamp == 0xffffff {
		s.total_duration += video.RtmpHeader.ChunkExtendedTimestamp.ExtendTimestamp
		pkt.Timestamp = s.total_duration
	} else {
		s.total_duration += video.RtmpHeader.ChunkMessgaeHeader.Timestamp
		pkt.Timestamp = s.total_duration
	}

	pkt.Type = video.RtmpHeader.ChunkMessgaeHeader.MessageTypeID
	pkt.Payload = video.RtmpBody.Payload

	//fmt.Println("recv video time stamp:", pkt.Timestamp)

	// Video = Tag + Payload
	// Video Tag (1 byte) == FrameType(4 bits) + CodecID(4 bits),发送为avc数据,所以CodecID为7.
	// Video Payload = AVCPacketType(1 Byte) + CompositionTime(3 Byte) + AVCDecoderConfigurationRecord
	// NALU类型 & 0001 1111 == 5 就是I帧
	// NALU类型在 00 00 00 01 分割之后的下一个字节
	tmp := pkt.Payload[0]         // 第一个字节保存着视频的相关信息.
	pkt.VideoFrameType = tmp >> 4 // 帧类型 4Bit, H264一般为1或者2
	pkt.VideoCodecID = tmp & 0x0f // 编码类型ID 4Bit, JPEG, H263, AVC...

	if s.videoTag == nil {
		s.videoTag = pkt
	} else {
		if pkt.VideoFrameType == 1 { // 关键帧
			s.videoKeyFrame = pkt
		}

		s.videochan <- pkt
	}
}

func metadataMessageHandle(s *RtmpNetStream, mete *MetadataMessage) {
	pkt := new(AVPacket)
	pkt.Timestamp = mete.RtmpHeader.ChunkMessgaeHeader.Timestamp

	if mete.RtmpHeader.ChunkMessgaeHeader.Timestamp == 0xffffff {
		pkt.Timestamp = mete.RtmpHeader.ChunkExtendedTimestamp.ExtendTimestamp
	}

	pkt.Type = mete.RtmpHeader.ChunkMessgaeHeader.MessageTypeID
	pkt.Payload = mete.RtmpBody.Payload

	if s.metaData == nil {
		s.metaData = pkt
	}
}

func createStreamMessageHandle(s *RtmpNetStream, csmsg *CreateStreamMessage) error {
	s.conn.streamID = s.conn.nextStreamID(csmsg.RtmpHeader.ChunkBasicHeader.ChunkStreamID)
	return sendMessage(s.conn, SEND_CREATE_STREAM_RESPONSE_MESSAGE, csmsg.TransactionId)
}

// 当发布者成功发布流后,服务器会接收到发布流的消息,然后进行消息广播
func publishMessageHandle(s *RtmpNetStream, pbmsg *PublishMessage) error {
	if strings.HasSuffix(s.conn.appName, "/") { // appName == "myapp"
		s.streamPath = s.conn.appName + strings.Split(pbmsg.PublishingName, "?")[0] // PublishingName ==  myapp/mystream
	} else {
		s.streamPath = s.conn.appName + "/" + strings.Split(pbmsg.PublishingName, "?")[0] // s.streamPath == myapp/mystream
	}

	err := s.serverHandler.OnPublishing(s)
	if err != nil {

		prmdErr := newPublishResponseMessageData(s.conn.streamID, "error", err.Error())

		err = sendMessage(s.conn, SEND_PUBLISH_RESPONSE_MESSAGE, prmdErr) // 服务器端发送publish的响应消息.
		if err != nil {
			return err
		}

		return nil
	}

	err = sendMessage(s.conn, SEND_STREAM_BEGIN_MESSAGE, nil) // 服务器端发送另一个协议消息(用户控制),这一消息包含 'StreamBegin' 事件,来指示发送给客户端的流的起点
	if err != nil {
		return err
	}

	prmdStart := newPublishResponseMessageData(s.conn.streamID, NetStream_Publish_Start, Level_Status)

	err = sendMessage(s.conn, SEND_PUBLISH_START_MESSAGE, prmdStart) // 服务器端发送publish start的消息.
	if err != nil {
		return err
	}

	if s.mode == 0 {
		s.mode = 1
	} else {
		s.mode = s.mode | 1
	}

	return nil
}

// 当订阅者成功订阅流后,服务器会接收到订阅流的消息
func playMessageHandle(s *RtmpNetStream, plmsg *PlayMessage) error {
	if strings.HasSuffix(s.conn.appName, "/") { // appName == "myapp"
		s.streamPath = s.conn.appName + strings.Split(plmsg.StreamName, "?")[0] // StreamName ==  myapp/mystream
	} else {
		s.streamPath = s.conn.appName + "/" + strings.Split(plmsg.StreamName, "?")[0] // s.streamPath == myapp/mystream
	}

	fmt.Println("stream path:", s.streamPath)

	s.conn.writeChunkSize = 512 //RTMP_MAX_CHUNK_SIZE
	err := s.serverHandler.OnPlaying(s)
	if err != nil {
		prmdErr := newPlayResponseMessageData(s.conn.streamID, "error", err.Error())

		err = sendMessage(s.conn, SEND_PLAY_RESPONSE_MESSAGE, prmdErr) // 服务器端发送play response的消息
		if err != nil {
			return err
		}
	}

	err = sendMessage(s.conn, SEND_CHUNK_SIZE_MESSAGE, uint32(s.conn.writeChunkSize)) // 服务器端发送设置块大小的消息
	if err != nil {
		return err
	}

	err = sendMessage(s.conn, SEND_STREAM_IS_RECORDED_MESSAGE, nil) // 服务器端发送另一个协议消息(用户控制),这个消息中定义了 'StreamIsRecorded' 事件和流 ID.消息在前两个字节中保存事件类型,在后四个字节中保存流 ID
	if err != nil {
		return err
	}

	sendMessage(s.conn, SEND_STREAM_BEGIN_MESSAGE, nil) // 服务器端发送另一个协议消息(用户控制),这一消息包含 'StreamBegin' 事件,来指示发送给客户端的流的起点
	if err != nil {
		return err
	}

	prmdReset := newPlayResponseMessageData(s.conn.streamID, NetStream_Play_Reset, Level_Status)

	err = sendMessage(s.conn, SEND_PLAY_RESPONSE_MESSAGE, prmdReset) // 服务端发送Play Reset消息,服务端发送只有当客户端发送的播放命令设置了reset命令的条件下,服务端才发送NetStream.Play.reset消息.
	if err != nil {
		return err
	}

	prmdStart := newPlayResponseMessageData(s.conn.streamID, NetStream_Play_Start, Level_Status)

	err = sendMessage(s.conn, SEND_PLAY_RESPONSE_MESSAGE, prmdStart) // 服务端发送Play Start消息
	if err != nil {
		return err
	}

	if s.mode == 0 {
		s.mode = 2
	} else {
		s.mode = s.mode | 2
	}

	return nil
}

// 客户端应该在发送FCPublishMessage消息的时候,就指定一个回调函数onFCPublish,来处理服务器返回的信息.
// 如果服务器发送NetStream.Publish.Start的消息给客户端,那么客户端可以开始推流了.
// 反之,如果发送NetStream.Publish.BadName的消息给客户端,那么客户端应该在回调函数onFCPublish中作出相应的处理.
func fcPublishMessageHandle(s *RtmpNetStream, fcpmsg *FCPublishMessage) (err error) {
	return
}

// 客户端应该在发送FCUnpublishMessage消息的时候,就指定一个回调函数onFCUnpublish,来处理服务器返回的信息.
// 如果服务器发送NetStream.Unpublish.Success的消息给客户端,表示服务器已经取消了该流的发布.客户端可以继续进行其他操作了.
func fcUnPublishMessageHandle(s *RtmpNetStream, fcunpmsg *FCUnpublishMessage) (err error) {
	// resData := newAMFObjects()
	// resData["code"] = NetStream_Unpublish_Success
	// resData["level"] = Level_Status
	// resData["streamid"] = conn.streamID
	// err = sendMessage(conn, SEND_UNPUBLISH_RESPONSE_MESSAGE, resData)
	// if err != nil {
	// 	return
	// }

	return
}

func decodeMetadataMessage(metadata *AVPacket) {
	if len(metadata.Payload) <= 0 {
		return
	}

	amf := newAMFDecoder(metadata.Payload)
	objs, _ := amf.readObjects()

	for _, v := range objs {
		switch tt := v.(type) {
		case string:
			{
				fmt.Println("string :", tt)
			}
		case []byte:
			{
				fmt.Println("[]byte :", tt)
			}
		case byte:
			{
				fmt.Println("byte :", tt)
			}
		case int:
			{
				fmt.Println("int :", tt)
			}
		case float64:
			{
				fmt.Println("float64 :", tt)
			}
		case AMFObjects:
			{
				for i, v1 := range tt {
					fmt.Println(i, " : ", v1)
				}
			}
		default:
			{
				fmt.Println("default", tt)
			}
		}
	}
}
