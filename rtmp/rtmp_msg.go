package rtmp

import (
	"../util"
	"bytes"
	"encoding/json"
	"fmt"
)

// http://www.adobe.com/cn/devnet/rtmp.html

// Sender -> Media data -> Message -> Chunk -> TCP Protocal -> Receiver -> Chunk -> Message -> Media data

// Message Format:
// The format of a message that can be split into chunks to support multiplexing depends on a higher level protocol. The message format
// SHOULD however contain the following fields which are necessary for creating the chunks.

// Timestamp : Timestamp of the message. This field can transport 4 bytes.

// Length : Length of the message payload. If the message header cannot be elided, it should be included in the length. This field occupies 3 bytes in the chunk header

// Type Id : A range of type IDs are reserved for protocol control messages. These messages which propagate information are
// handled by both RTMP Chunk Stream protocol and the higher-level protocol. All other type IDs are available for use by the higher-level
// protocol, and treated as opaque values by RTMP Chunk Stream. In fact, nothing in RTMP Chunk Stream requires these values to be
// used as a type; all (non-protocol) messages could be of the same type, or the application could use this field to distinguish
// simultaneous tracks rather than types. This field occupies 1 byte in the chunk header

// Message Stream ID : The message stream ID can be any arbitrary value. Different message streams multiplexed onto the same chunk stream
// are demultiplexed based on their message stream IDs. Beyond that, as far as RTMP Chunk Stream is concerned, this is an opaque value.
// This field occupies 4 bytes in the chunk header in little endian format.

// 消息格式必须包含上面4个字段."SHOULD"在[RFC2119]中进行解释.

// RTMP Message Format:
// The RTMP message has two parts, a header and its payload(body).

// RTMP Message Header:
// Message Type : One byte field to represent the message type. A range of type IDs (1-6) are reserved for protocol control messages.

// Length : Three-byte field that represents the size of the payload in bytes. It is set in big-endian format.

// Timestamp : Four-byte field that contains a timestamp of the message. The 4 bytes are packed in the big-endian order.

// Message Stream Id : Three-byte field that identifies the stream of the message. These bytes are set in big-endian format.

// RTMP Message Payload(Body):
// The other part of the message is the payload, which is the actual data contained in the message. For example, it could be some audio
// samples or compressed video data. The payload format and interpretation are beyond the scope of this document.

//
// RTMP Message == Control Message (1-7) + Audio Message (8) + Video Message (9) + Data Message (15, 18) +
// Shared Object Message (16, 19) + Command Message (17, 20) + Aggregate Message (22)
//
// (1-7): Control Message == Set Chunk Size Message (1) + Abort Message (2) + Acknowledgement Message (3) + User Control Message (4) +
// Window Acknowledgement Size Message (5) + Set Peer Bandwidth (6) + Edeg Message (7)
//
// (4): User Control Message == Stream Bgin Message (=0) + Stream EOF Message (=1) + StreamDry Message (=2) + SetBuffer Length Message (=3) +
// StreamIsRecorded Message (=4) + PingRequest Messgae (=6) + PingResponse Message(=7)
//
// (15,18): Data Message == Metadata Message (AMF3 == 15, AMF0 == 18)
//
// (16,19): Shared Object Message == Shared Object Message (AMF3 == 16, AMF0 == 19)
//
// (17,20): Command Message = NetConnect Message + NetStream Message (AMF3 == 17, AMF0 == 20)
//
// (17,20): NetConnect Message == Connect Message + Call Message + Close Message + Create Stream Message
//
// (17,20): NetStream Message == Play Message + Play2 Message +  Delete Stream Message + Close Stream Message +
// Receive Audio Message + Receive Video Message + Publish Message + Seek Message + Pause Message
//
// (17,20): Responce Message == Client -> Send ("_result", "onStatus", "_error", Command Message) -> Server
//
//

type RtmpMessage interface {
	Header() *RtmpHeader
	Body() *RtmpBody
	String() string
}

type RtmpHeader ChunkHeader
type RtmpBody ChunkBody

func newRtmpHeader(chunkID uint32, timestamp uint32, messageLength uint32, messageType byte, messageStreamID uint32, extendTimestamp uint32) *RtmpHeader {
	head := new(RtmpHeader)
	head.ChunkBasicHeader.ChunkStreamID = chunkID
	head.ChunkMessgaeHeader.Timestamp = timestamp
	head.ChunkMessgaeHeader.MessageLength = messageLength
	head.ChunkMessgaeHeader.MessageTypeID = messageType
	head.ChunkMessgaeHeader.MessageStreamID = messageStreamID
	head.ChunkExtendedTimestamp.ExtendTimestamp = extendTimestamp
	return head
}

func (h *RtmpHeader) Clone() *RtmpHeader {
	head := new(RtmpHeader)
	head.ChunkBasicHeader.ChunkStreamID = h.ChunkBasicHeader.ChunkStreamID
	head.ChunkMessgaeHeader.Timestamp = h.ChunkMessgaeHeader.Timestamp
	head.ChunkMessgaeHeader.MessageLength = h.ChunkMessgaeHeader.MessageLength
	head.ChunkMessgaeHeader.MessageTypeID = h.ChunkMessgaeHeader.MessageTypeID
	head.ChunkMessgaeHeader.MessageStreamID = h.ChunkMessgaeHeader.MessageStreamID
	head.ChunkExtendedTimestamp.ExtendTimestamp = h.ChunkExtendedTimestamp.ExtendTimestamp

	return head
}

func GetRtmpMessage(head *RtmpHeader, body *RtmpBody) RtmpMessage {
	switch head.ChunkMessgaeHeader.MessageTypeID {
	case RTMP_MSG_CHUNK_SIZE: // RTMP消息类型ID=1,设置块大小.
		{
			m := newChunkSizeMessage() // ControlMessage +  ChunkSize(4 Bytes)
			m.RtmpHeader = head
			m.RtmpBody = body
			m.ChunkSize = util.BigEndian.Uint32(body.Payload)
			return m
		}
	case RTMP_MSG_ABORT: // RTMP消息类型ID=2,取消消息,用于通知正在等待接收块以完成消息的对等端,丢弃一个块流中已经接收的部分并且取消对该消息的处理.
		{
			m := newAbortMessage() // ControlMessage + ChunkId(4 Bytes)
			m.RtmpHeader = head
			m.RtmpBody = body
			m.ChunkStreamId = util.BigEndian.Uint32(body.Payload)
			return m
		}
	case RTMP_MSG_ACK: // RTMP消息类型ID=3,确认(致谢).Acknowledgement.在数据通信传输中,接收站发给发送站的一种传输控制字符.它表示确认发来的数据已经接收无误.
		{
			m := newAcknowledgementMessage() // ControlMessage + SequenceNumber(4 Bytes) 也就是目前接收到的字节数
			m.RtmpHeader = head
			m.RtmpBody = body
			m.SequenceNumber = util.BigEndian.Uint32(body.Payload)
			return m
		}
	case RTMP_MSG_USER_CONTROL: // RTMP消息类型ID=4, 用户控制消息.客户端或服务端发送本消息通知对方用户的控制事件.
		{
			eventtype := util.BigEndian.Uint16(body.Payload) // | Event Type(16 Bit) | Event Data |
			eventdata := body.Payload[2:]
			switch eventtype {
			case RTMP_USER_STREAM_BEGIN: // 服务端向客户端发送本事件通知对方一个流开始起作用可以用于通讯.在默认情况下,服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
				{
					m := newStreamBeginMessage() // UserControlMessage + StreamId(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					if len(eventdata) >= 4 {
						//服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
						m.StreamID = util.BigEndian.Uint32(eventdata)
					}
					return m
				}
			case RTMP_USER_STREAM_EOF: // 服务端向客户端发送本事件通知客户端,数据回放完成.果没有发行额外的命令,就不再发送数据.客户端丢弃从流中接收的消息.4字节的事件数据表示,回放结束的流的ID.
				{
					m := newStreamEOFMessage() // UserControlMessage + StreamId(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					//服务端通知客户端流结束,4字节的事件数据表示,回放结束的流的ID
					m.StreamID = util.BigEndian.Uint32(eventdata)
					return m
				}
			case RTMP_USER_STREAM_DRY: // 服务端向客户端发送本事件通知客户端,流中没有更多的数据.如果服务端在一定周期内没有探测到更多的数据,就可以通知客户端流枯竭.4字节的事件数据表示枯竭流的ID.
				{
					m := newStreamDryMessage() // UserControlMessage + StreamId(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					//服务端通知客户端流枯竭,4字节的事件数据表示枯竭流的ID
					m.StreamID = util.BigEndian.Uint32(eventdata)
					return m
				}
			case RTMP_USER_SET_BUFFLEN: // 客户端向服务端发送本事件,告知对方自己存储一个流的数据的缓存的长度(毫秒单位).当服务端开始处理一个流得时候发送本事件.事件数据的头四个字节表示流ID,后4个字节表示缓存长度(毫秒单位).
				{
					m := newSetBufferMessage() // UserControlMessage + StreamId(4 Bytes) + Millisecond(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					m.StreamID = util.BigEndian.Uint32(eventdata)
					m.Millisecond = util.BigEndian.Uint32(eventdata[4:])
					return m
				}
			case RTMP_USER_STREAM_IS_RECORDED: // 服务端发送本事件通知客户端,该流是一个录制流.4字节的事件数据表示录制流的.
				{
					m := newStreamIsRecordedMessage() // UserControlMessage + StreamId(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					m.StreamID = util.BigEndian.Uint32(eventdata)
					return m
				}
			case RTMP_USER_PING_REQUEST: // 服务端通过本事件测试客户端是否可达.事件数据是4个字节的事件戳.代表服务调用本命令的本地时间.客户端在接收到kMsgPingRequest之后返回kMsgPingResponse事件
				{
					m := newPingRequestMessage() // UserControlMessage + Timestamp(4 Bytes)
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					m.Timestamp = util.BigEndian.Uint32(eventdata)
					return m
				}
			case RTMP_USER_PING_RESPONSE: // 客户端向服务端发送本消息响应ping请求.事件数据是接kMsgPingRequest请求的时间.
				{
					m := newPingResponseMessage() // UserControlMessage
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					return m
				}
			case RTMP_USER_EMPTY:
				{
					m := newBufferEmptyMessage() // UserControlMessage
					m.RtmpHeader = head
					m.RtmpBody = body
					m.EventType = eventtype
					m.EventData = eventdata
					return m
				}
			default:
				{
					m := newUnknowRtmpMessage()
					m.RtmpHeader = head
					m.RtmpBody = body
					return m
				}
			}
		}
	case RTMP_MSG_ACK_SIZE: // RTMP消息类型ID=5, 确认(致谢)窗口大小.Window Acknowledgement Size.
		{
			m := newWindowAcknowledgementSizeMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.AcknowledgementWindowsize = util.BigEndian.Uint32(body.Payload)
			return m
		}
	case RTMP_MSG_BANDWIDTH: // RTMP消息类型ID=6, 置对等端带宽.客户端或服务端发送本消息更新对等端的输出带宽.
		{
			m := newSetPeerBandwidthMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.AcknowledgementWindowsize = util.BigEndian.Uint32(body.Payload)
			if len(body.Payload) > 4 {
				m.LimitType = body.Payload[4]
			}
			return m
		}
	case RTMP_MSG_EDGE: // RTMP消息类型ID=7, 用于边缘服务与源服务器.
		{
			m := newEdegMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AUDIO: // RTMP消息类型ID=8, 音频数据.客户端或服务端发送本消息用于发送音频数据.
		{
			m := newAudioMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_VIDEO: // RTMP消息类型ID=9, 视频数据.客户端或服务端发送本消息用于发送视频数据.
		{
			m := newVideoMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AMF3_METADATA: // RTMP消息类型ID=15, 数据消息.用AMF3编码.
		{
			m := newMetadataMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AMF3_SHARED: // RTMP消息类型ID=16, 共享对象消息.用AMF3编码.
		{
			m := newSharedObjectMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AMF3_COMMAND: // RTMP消息类型ID=17, 命令消息.用AMF3编码.
		{
			return decodeCommandAMF3(head, body)
		}
	case RTMP_MSG_AMF0_METADATA: // RTMP消息类型ID=18, 数据消息.用AMF0编码.
		{
			m := newMetadataMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AMF0_SHARED: // RTMP消息类型ID=19, 共享对象消息.用AMF0编码.
		{
			m := newSharedObjectMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	case RTMP_MSG_AMF0_COMMAND: // RTMP消息类型ID=20, 命令消息.用AMF0编码.
		{
			return decodeCommandAMF0(head, body) // 解析具体的命令消息
		}
	case RTMP_MSG_AGGREGATE:
		{
			m := newAggregateMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		} // RTMP消息类型ID=22, 聚集消息.多个RTMP子消息的集合
	default:
		{
			m := newUnknowRtmpMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	}
}

// 03 00 00 00 00 01 02 14 00 00 00 00 02 00 07 63 6F 6E 6E 65 63 74 00 3F F0 00 00 00 00 00 00 08
//
// 这个函数解析的是从02(第13个字节)开始,前面12个字节是Header,后面的是Payload,即解析Payload.
//
// 解析用AMF0编码的命令消息.(Payload)
// 第一个字节(Byte)为此数据的类型.例如:string,int,bool...

// string就是字符类型,一个byte的amf类型,两个bytes的字符长度,和N个bytes的数据.
// 比如: 02 00 02 33 22,第一个byte为amf类型,其后两个bytes为长度,注意这里的00 02是大端模式,33 22是字符数据

// umber类型其实就是double,占8bytes.
// 比如: 00 00 00 00 00 00 00 00,第一个byte为amf类型,其后8bytes为double值0.0

// boolean就是布尔类型,占用1byte.
// 比如:01 00,第一个byte为amf类型,其后1byte是值,false.

// object类型要复杂点.
// 第一个byte是03表示object,其后跟的是N个(key+value).最后以00 00 09表示object结束
func decodeCommandAMF0(head *RtmpHeader, body *RtmpBody) RtmpMessage {
	amf := newAMFDecoder(body.Payload) // rtmp_amf.go, amf 是 bytes类型, 将rtmp body(payload)放到bytes.Buffer(amf)中去.
	cmd := readString(amf)             // rtmp_amf.go, 将payload的bytes类型转换成string类型.
	switch cmd {
	case "connect":
		{
			m := newConnectMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf) // "connect"数据后,就是传输ID字段. 1个字节
			m.Object = readObject(amf)               // 命令对象
			m.Optional = readObject(amf)             // 可选用户变量
			return m
		}
	case "call":
		{
			m := newCallMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			m.Object = readObject(amf)
			m.Optional = readObject(amf)
			return m
		}
	case "createStream":
		{
			m := newCreateStreamMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.Object = readObject(amf)
			return m
		}
	case "play":
		{
			m := newPlayMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.StreamName = readString(amf)
			m.Start = readNumber(amf)
			m.Duration = readNumber(amf)
			m.Rest = readBool(amf)
			return m
		}
	case "play2":
		{
			m := newPlay2Message()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			m.StartTime = readNumber(amf)
			m.OldStreamName = readString(amf)
			m.StreamName = readString(amf)
			m.Duration = readNumber(amf)
			m.Transition = readString(amf)
			return m
		}
	case "publish":
		{
			m := newPublishMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.PublishingName = readString(amf)
			m.PublishingType = readString(amf)
			return m
		}
	case "pause":
		{
			m := newPauseMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.Pause = readBool(amf)
			m.Milliseconds = readNumber(amf)
			return m
		}
	case "seek":
		{
			m := newSeekMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.Milliseconds = readNumber(amf)
			return m
		}
	case "deleteStream":
		{
			m := newDeleteStreamMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.StreamId = uint32(readNumber(amf))
			return m
		}
	case "closeStream":
		{
			m := newCloseStreamMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.StreamId = uint32(readNumber(amf))
			return m
		}
	case "releaseStream":
		{
			m := newReleaseStreamMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.StreamId = uint32(readNumber(amf))
			return m
		}
	case "receiveAudio":
		{
			m := newReceiveAudioMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.BoolFlag = readBool(amf)
			return m
		}
	case "receiveVideo":
		{
			m := newReceiveVideoMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.TransactionId = readTransactionId(amf)
			amf.readNull()
			m.BoolFlag = readBool(amf)
			return m
		}
	case "_result":
		{
			m := newResponseMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.Properties = readObject(amf)
			m.Infomation = readObject(amf)
			return m
		}
	case "onStatus":
		{
			m := newResponseMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.Properties = readObject(amf)
			m.Infomation = readObject(amf)
			return m
		}
	case "_error":
		{
			m := newResponseMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			m.Properties = readObject(amf)
			m.Infomation = readObject(amf)
			return m
		}
	case "FCPublish":
		{
			m := newFCPublishMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			return m
		}
	case "FCUnpublish":
		{
			m := newFCUnpublishMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			m.CommandName = cmd
			return m
		}
	default:
		{
			fmt.Println("decode command amf0 cmd:", cmd)
			m := newUnknowRtmpMessage()
			m.RtmpHeader = head
			m.RtmpBody = body
			return m
		}
	}
}

func decodeCommandAMF3(head *RtmpHeader, body *RtmpBody) RtmpMessage {
	body.Payload = body.Payload[1:]
	return decodeCommandAMF0(head, body)
}

func readTransactionId(amf *AMF) uint64 {
	v, _ := amf.readNumber()
	return uint64(v)
}
func readString(amf *AMF) string {
	v, _ := amf.readString()
	return v
}
func readNumber(amf *AMF) uint64 {
	v, _ := amf.readNumber()
	return uint64(v)
}
func readBool(amf *AMF) bool {
	v, _ := amf.readBool()
	return v
}

func readObject(amf *AMF) AMFObjects {
	v, _ := amf.readObject()
	return v
}

/* Control Message */
type ControlMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newControlMessage() *ControlMessage {
	return &ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *ControlMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ControlMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ControlMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ControlMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

/* Command Message */
type CommandMessage struct {
	RtmpHeader    *RtmpHeader
	RtmpBody      *RtmpBody `json:"-"`
	CommandName   string    // 命令名. 字符串. 命令名.设置为"connect"
	TransactionId uint64    // 传输ID. 数字. 总是设为1
}

func newCommandMessage() *CommandMessage {
	return &CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *CommandMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *CommandMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *CommandMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("CommandMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 1.
// Set Chunk Size, is used to notify the peer of a new maximum chunk size

// chunk size (31 bits): This field holds the new maximum chunk size,in bytes, which will be used for all of the sender’s subsequent chunks until further notice
type ChunkSizeMessage struct {
	ControlMessage
	ChunkSize uint32 // 4 bytes
}

func newChunkSizeMessage() *ChunkSizeMessage {
	return &ChunkSizeMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ChunkSizeMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 4)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload, msg.ChunkSize)
}

func (msg *ChunkSizeMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ChunkSizeMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ChunkSizeMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ChunkSizeMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 2, Abort Message.
// Is used to notify the peer if it is waiting for chunks to complete a message, then to discard
// the partially received message over a chunk stream. The peer receives the chunk stream ID as this protocol message’s payload. An
// application may send this message when closing in order to indicate that further processing of the messages is not required

// chunk stream ID (32 bits): This field holds the chunk stream ID, whose current message is to be discarded.
type AbortMessage struct {
	ControlMessage
	ChunkStreamId uint32 // 4 byte chunk stream id
}

func newAbortMessage() *AbortMessage {
	return &AbortMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *AbortMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 4)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload, msg.ChunkStreamId)
}

func (msg *AbortMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *AbortMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *AbortMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("AbortMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 3, Acknowledgement Message.
// The client or the server MUST send an acknowledgment to the peer after receiving bytes equal to the window size.
// The window size is the maximum number of bytes that the sender sends without receiving acknowledgment from the receiver. This message specifies the
// sequence number, which is the number of the bytes received so far.

// sequence number (32 bits): This field holds the number of bytes received so far.
type AcknowledgementMessage struct {
	ControlMessage
	SequenceNumber uint32 // 4 bytes
}

func newAcknowledgementMessage() *AcknowledgementMessage {
	return &AcknowledgementMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *AcknowledgementMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 4)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload, msg.SequenceNumber)
}

func (msg *AcknowledgementMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *AcknowledgementMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *AcknowledgementMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("AcknowledgementMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 4, User Control Messages.
// User Control messages SHOULD use message stream ID 0 (known as the control stream) and, when sent over RTMP Chunk Stream,
// be sent on chunk stream ID 2. User Control messages are effective at the point they are received in the stream; their timestamps are ignored.

// Event Type (16 bits) : The first 2 bytes of the message data are used to identify the Event type. Event type is followed by Event data.
// Event Data
type UserControlMessage struct {
	ControlMessage
	EventType uint16
	EventData []byte
}

func newUserControlMessage() *UserControlMessage {
	return &UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *UserControlMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *UserControlMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *UserControlMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("UserControlMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 5, Window Acknowledgement Size Message.
// The client or the server sends this message to inform the peer of the window size to use between sending acknowledgments.

// AcknowledgementWindowsize (4 bytes)
type WindowAcknowledgementSizeMessage struct {
	ControlMessage
	AcknowledgementWindowsize uint32 // 4 bytes
}

func newWindowAcknowledgementSizeMessage() *WindowAcknowledgementSizeMessage {
	return &WindowAcknowledgementSizeMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *WindowAcknowledgementSizeMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 4)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload, msg.AcknowledgementWindowsize)
}

func (msg *WindowAcknowledgementSizeMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *WindowAcknowledgementSizeMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *WindowAcknowledgementSizeMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("WindowAcknowledgementSizeMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 6, Set Peer Bandwidth Message.
// The client or the server sends this message to limit the output bandwidth of its peer.

// AcknowledgementWindowsize (4 bytes)
// LimitType : The Limit Type is one of the following values: 0 - Hard, 1 - Soft, 2- Dynamic.
type SetPeerBandwidthMessage struct {
	ControlMessage
	AcknowledgementWindowsize uint32 // 4 bytes
	LimitType                 byte
}

func newSetPeerBandwidthMessage() *SetPeerBandwidthMessage {
	return &SetPeerBandwidthMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *SetPeerBandwidthMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 5)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload, msg.AcknowledgementWindowsize)
	msg.RtmpBody.Payload[4] = msg.LimitType
}

func (msg *SetPeerBandwidthMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *SetPeerBandwidthMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *SetPeerBandwidthMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("SetPeerBandwidthMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Protocol control message 7, is used between edge server and origin server.
type EdegMessage struct {
	ControlMessage
}

func newEdegMessage() *EdegMessage {
	return &EdegMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *EdegMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *EdegMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *EdegMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("EdegMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 8, Audio Message. The client or the server sends this message to send audio data to the peer.
// The message type value of 8 is reserved for audio messages.
type AudioMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newAudioMessage() *AudioMessage {
	return &AudioMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *AudioMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *AudioMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *AudioMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("AudioMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 9, Video Message. The client or the server sends this message to send video data to the peer.
// The message type value of 9 is reserved for video messages.
type VideoMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newVideoMessage() *VideoMessage {
	return &VideoMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *VideoMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *VideoMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *VideoMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("VideoMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 15, 18. Data Message. The client or the server sends this message to send Metadata or any
// user data to the peer. Metadata includes details about the data(audio, video etc.) like creation time, duration,
// theme and so on. These messages have been assigned message type value of 18 for AMF0 and message type value of 15 for AMF3
type MetadataMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
	Proterties map[string]interface{} `json:",omitempty"`
}

func newMetadataMessage() *MetadataMessage {
	return &MetadataMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *MetadataMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *MetadataMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *MetadataMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("MetadataMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 16, 19. Shared Object Message. A shared object is a Flash object (a collection of name value pairs)
// that are in synchronization across multiple clients, instances, and so on. The message types 19 for AMF0 and 16 for AMF3
// are reserved for shared object events. Each message can contain multiple events.
type SharedObjectMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newSharedObjectMessage() *SharedObjectMessage {
	return &SharedObjectMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *SharedObjectMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *SharedObjectMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *SharedObjectMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("SharedObjectMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 17, 20. Command Message. Command messages carry the AMF-encoded commands between the client
// and the server. These messages have been assigned message type value of 20 for AMF0 encoding and message type value of 17 for AMF3
// encoding. These messages are sent to perform some operations like connect, createStream, publish, play, pause on the peer. Command
// messages like onstatus, result etc. are used to inform the sender about the status of the requested commands. A command message
// consists of command name, transaction ID, and command object that contains related parameters. A client or a server can request Remote
// Procedure Calls (RPC) over streams that are communicated using the command messages to the peer.

// NetConnect + NetStream
// NetConnect == Connect + Call + Close + Create Stream
// NetStream == Play + Play2 + DeleteStream + CloseStream + ReceiveAudio + ReceiveVideo + Publish + Seek + Pause
// Response Client = ResponseConnect + ResponseCall +

// Message 22. Aggregate Message. An aggregate message is a single message that contains a series of
// RTMP sub-messages using the format described in Section 6.1. Message type 22 is used for aggregate messages.
type AggregateMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newAggregateMessage() *AggregateMessage {
	return &AggregateMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *AggregateMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *AggregateMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *AggregateMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("AggregateMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Unknow Rtmp Message
type UnknowRtmpMessage struct {
	RtmpHeader *RtmpHeader
	RtmpBody   *RtmpBody
}

func newUnknowRtmpMessage() *UnknowRtmpMessage {
	return &UnknowRtmpMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}
}

func (msg *UnknowRtmpMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *UnknowRtmpMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *UnknowRtmpMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("UnknowRtmpMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Message 17, 20.
// The following class objects are used to send various commands:
// NetConnection An object that is a higher-level representation of connection between the server and the client.
// NetStream An object that represents the channel over which audio streams, video streams and other related data are sent. We also
// send commands like play , pause etc. which control the flow of the data.

// The following commands can be sent on the NetConnection:
// Connect
// Call
// Close
// Create Stream

// Connect Message
// The client sends the connect command to the server to request connection to a server application instance.
type ConnectMessage struct {
	CommandMessage
	Object   interface{} `json:",omitempty"` // 命令对象. 对象. 含有名值对的命令信息对
	Optional interface{} `json:",omitempty"` // 可选的用户变量.对象. 任何可选信息
}

// Object 可选值:
// App 				客户端要连接到的服务应用名 												Testapp
// Flashver			Flash播放器版本.和应用文档中getversion()函数返回的字符串相同.			FMSc/1.0
// SwfUrl			发起连接的swf文件的url													file://C:/ FlvPlayer.swf
// TcUrl			服务url.有下列的格式.protocol://servername:port/appName/appInstance		rtmp://localhost::1935/testapp/instance1
// fpad				是否使用代理															true or false
// audioCodecs		指示客户端支持的音频编解码器											SUPPORT_SND_MP3
// videoCodecs		指示支持的视频编解码器													SUPPORT_VID_SORENSON
// pageUrl			SWF文件被加载的页面的Url												http:// somehost/sample.html
// objectEncoding	AMF编码方法																AMF编码方法	kAMF3

func newConnectMessage() *ConnectMessage {
	return &ConnectMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ConnectMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))

	if msg.Object != nil {
		amf.encodeObject(msg.Object.(AMFObjects))
	}
	if msg.Optional != nil {
		amf.encodeObject(msg.Optional.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

func (msg *ConnectMessage) Encode3() {
	msg.Encode0()

	buf := new(bytes.Buffer)
	buf.WriteByte(0)
	buf.Write(msg.RtmpBody.Payload)
	msg.RtmpBody.Payload = buf.Bytes()
}

func (msg *ConnectMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ConnectMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ConnectMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ConnectMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Call Message.
// The call method of the NetConnection object runs remote procedure calls (RPC) at the receiving end.
// The called RPC name is passed as a parameter to the call command.
type CallMessage struct {
	CommandMessage
	Object   interface{} `json:",omitempty"`
	Optional interface{} `json:",omitempty"`
}

func newCallMessage() *CallMessage {
	return &CallMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *CallMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))

	if msg.Object != nil {
		amf.encodeObject(msg.Object.(AMFObjects))
	}
	if msg.Optional != nil {
		amf.encodeObject(msg.Optional.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

func (msg *CallMessage) Encode3() {
	msg.Encode0()

	buf := new(bytes.Buffer)
	buf.WriteByte(0)
	buf.Write(msg.RtmpBody.Payload)
	msg.RtmpBody.Payload = buf.Bytes()
}

func (msg *CallMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *CallMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *CallMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("CallMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Create Stream Message.
// The client sends this command to the server to create a logical channel for message communication The publishing of audio,
// video, and metadata is carried out over stream channel created using the createStream command.

type CreateStreamMessage struct {
	CommandMessage
	Object interface{}
}

func newCreateStreamMessage() *CreateStreamMessage {
	return &CreateStreamMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *CreateStreamMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))

	if msg.Object != nil {
		amf.encodeObject(msg.Object.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *CreateStreamMessage) Encode3() {
	msg.Encode0()

	buf := new(bytes.Buffer)
	buf.WriteByte(0)
	buf.Write(msg.RtmpBody)
	msg.RtmpBody = buf.Bytes()
}*/

func (msg *CreateStreamMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *CreateStreamMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *CreateStreamMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("CreateStreamMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// The following commands can be sent on the NetStream by the client to the server:

// Play
// Play2
// DeleteStream
// CloseStream
// ReceiveAudio
// ReceiveVideo
// Publish
// Seek
// Pause
// Release(37)
// FCPublish

// Play Message
// The client sends this command to the server to play a stream. A playlist can also be created using this command multiple times
type PlayMessage struct {
	CommandMessage
	Object     interface{} `json:",omitempty"`
	StreamName string
	Start      uint64
	Duration   uint64
	Rest       bool
}

func newPlayMessage() *PlayMessage {
	return &PlayMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

// 命令名 -> 命令名,设置为”play”
// 传输ID -> 0
// 命令对象
// 流名字 -> 要播放流的名字
// start -> 可选的参数,以秒为单位定义开始时间.默认值为 -2,表示用户首先尝试播放流名字段中定义的直播流.
// Duration -> 可选的参数,以秒为单位定义了回放的持续时间.默认值为 -1.-1 值意味着一个直播流会一直播放直到它不再可用或者一个录制流一直播放直到结束
// Reset -> 可选的布尔值或者数字定义了是否对以前的播放列表进行 flush

func (msg *PlayMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))
	amf.writeNull()
	amf.writeString(msg.StreamName)

	if msg.Start > 0 {
		amf.writeNumber(float64(msg.Start))
	}
	if msg.Duration > 0 {
		amf.writeNumber(float64(msg.Duration))
	}

	amf.writeBool(msg.Rest)
	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *PlayMessage) Encode3() {
}*/

func (msg *PlayMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *PlayMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *PlayMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("PlayMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Play2 Message
// Unlike the play command, play2 can switch to a different bit rate stream without changing the timeline of the content played. The
// server maintains multiple files for all supported bitrates that the client can request in play2.
type Play2Message struct {
	CommandMessage
	StartTime     uint64
	OldStreamName string
	StreamName    string
	Duration      uint64
	Transition    string
}

func newPlay2Message() *Play2Message {
	return &Play2Message{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *Play2Message) Encode0() {
}

func (msg *Play2Message) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *Play2Message) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *Play2Message) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("Play2Message [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Delete Stream Message
// NetStream sends the deleteStream command when the NetStream object is getting destroyed
type DeleteStreamMessage struct {
	CommandMessage
	Object   interface{}
	StreamId uint32
}

func newDeleteStreamMessage() *DeleteStreamMessage {
	return &DeleteStreamMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *DeleteStreamMessage) Encode0() {
}

func (msg *DeleteStreamMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *DeleteStreamMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *DeleteStreamMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("DeleteStreamMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Close Stream Message
type CloseStreamMessage struct {
	CommandMessage
	Object   interface{}
	StreamId uint32
}

func newCloseStreamMessage() *CloseStreamMessage {
	return &CloseStreamMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *CloseStreamMessage) Encode0() {
}

func (msg *CloseStreamMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *CloseStreamMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *CloseStreamMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("CloseStreamMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Receive Audio Message
// NetStream sends the receiveAudio message to inform the server whether to send or not to send the audio to the client
type ReceiveAudioMessage struct {
	CommandMessage
	Object   interface{}
	BoolFlag bool
}

func newReceiveAudioMessage() *ReceiveAudioMessage {
	return &ReceiveAudioMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ReceiveAudioMessage) Encode0() {
}

func (msg *ReceiveAudioMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ReceiveAudioMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ReceiveAudioMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ReceiveAudioMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Receive Video Message
// NetStream sends the receiveVideo message to inform the server whether to send the video to the client or not
type ReceiveVideoMessage struct {
	CommandMessage
	Object   interface{}
	BoolFlag bool
}

func newReceiveVideoMessage() *ReceiveVideoMessage {
	return &ReceiveVideoMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ReceiveVideoMessage) Encode0() {
}

func (msg *ReceiveVideoMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ReceiveVideoMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ReceiveVideoMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ReceiveVideoMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Publish Message
// The client sends the publish command to publish a named stream to the server. Using this name,
// any client can play this stream and receive the published audio, video, and data messages
type PublishMessage struct {
	CommandMessage
	Object         interface{} `json:",omitempty"`
	PublishingName string
	PublishingType string
}

func newPublishMessage() *PublishMessage {
	return &PublishMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

// 命令名 -> 命令名,设置为”publish”
// 传输ID -> 0
// 命令对象
// 发布名 -> 流发布的名字
// 发布类型 -> 设置为”live”，”record”或”append”.

// “record”:流被发布,并且数据被录制到一个新的文件,文件被存储到服务端的服务应用的目录的一个子目录下.如果文件已经存在则重写文件.
// “append”:流被发布并且附加到一个文件之后.如果没有发现文件则创建一个文件.
// “live”:发布直播数据而不录制到文件

func (msg *PublishMessage) Encode0() {
}

func (msg *PublishMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *PublishMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *PublishMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("PublishMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Seek Message
// The client sends the seek command to seek the offset (in milliseconds) within a media file or playlist.
type SeekMessage struct {
	CommandMessage
	Object       interface{} `json:",omitempty"`
	Milliseconds uint64
}

func newSeekMessage() *SeekMessage {
	return &SeekMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *SeekMessage) Encode0() {
}

func (msg *SeekMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *SeekMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *SeekMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("SeekMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Pause Message
// The client sends the pause command to tell the server to pause or start playing.
type PauseMessage struct {
	CommandMessage
	Object       interface{}
	Pause        bool
	Milliseconds uint64
}

func newPauseMessage() *PauseMessage {
	return &PauseMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

// 命令名 -> 命令名,设置为”pause”
// 传输ID -> 0
// 命令对象 -> null
// Pause/Unpause Flag -> true 或者 false，来指示暂停或者重新播放
// milliSeconds -> 流暂停或者重新开始所在的毫秒数.这个是客户端暂停的当前流时间.当回放已恢复时,服务器端值发送带有比这个值大的 timestamp 消息

func (msg *PauseMessage) Encode0() {
}

func (msg *PauseMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *PauseMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *PauseMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("PauseMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Release Stream Message
// TODO: Release
type ReleaseStreamMessage struct {
	CommandMessage
	Object   interface{}
	StreamId uint32
}

func newReleaseStreamMessage() *ReleaseStreamMessage {
	return &ReleaseStreamMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ReleaseStreamMessage) Encode0() {
}

func (msg *ReleaseStreamMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ReleaseStreamMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ReleaseStreamMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ReleaseStreamMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// FCPublish Message

type FCPublishMessage struct {
	CommandMessage
}

func newFCPublishMessage() *FCPublishMessage {
	return &FCPublishMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *FCPublishMessage) Encode0() {
}

func (msg *FCPublishMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *FCPublishMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *FCPublishMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("FCPublishMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// FCUnpublish Message

type FCUnpublishMessage struct {
	CommandMessage
}

func newFCUnpublishMessage() *FCUnpublishMessage {
	return &FCUnpublishMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *FCUnpublishMessage) Encode0() {
}

func (msg *FCUnpublishMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *FCUnpublishMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *FCUnpublishMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("FCUnpublishMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Message. Server -> Response -> Client
//

//
// Response Connect Message
//
type ResponseConnectMessage struct {
	CommandMessage
	Properties interface{} `json:",omitempty"`
	Infomation interface{} `json:",omitempty"`
}

func newResponseConnectMessage() *ResponseConnectMessage {
	return &ResponseConnectMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponseConnectMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))

	if msg.Properties != nil {
		amf.encodeObject(msg.Properties.(AMFObjects))
	}
	if msg.Infomation != nil {
		amf.encodeObject(msg.Infomation.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *ResponseConnectMessage) Encode3() {
}*/

func (msg *ResponseConnectMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponseConnectMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponseConnectMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponseConnectMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Call Message
//
type ResponseCallMessage struct {
	CommandMessage
	Object   interface{}
	Response interface{}
}

func newResponseCallMessage() *ResponseCallMessage {
	return &ResponseCallMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponseCallMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))

	if msg.Object != nil {
		amf.encodeObject(msg.Object.(AMFObjects))
	}
	if msg.Response != nil {
		amf.encodeObject(msg.Response.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *ResponseCallMessage) Encode3() {
}*/

func (msg *ResponseCallMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponseCallMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponseCallMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponseCallMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Create Stream Message
//
type ResponseCreateStreamMessage struct {
	CommandMessage
	Object   interface{} `json:",omitempty"`
	StreamId uint32
}

func newResponseCreateStreamMessage() *ResponseCreateStreamMessage {
	return &ResponseCreateStreamMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponseCreateStreamMessage) Encode0() {
	amf := newAMFEncoder() // rtmp_amf.go
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))
	amf.writeNull()
	amf.writeNumber(float64(msg.StreamId))
	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *ResponseCreateStreamMessage) Encode3() {
}*/

func (msg *ResponseCreateStreamMessage) Decode0(head *RtmpHeader, body RtmpBody) {
	amf := newAMFDecoder(body.Payload)
	if obj, err := amf.decodeObject(); err == nil {
		msg.CommandName = obj.(string)
	}
	if obj, err := amf.decodeObject(); err == nil {
		msg.TransactionId = uint64(obj.(float64))
	}

	amf.decodeObject()
	if obj, err := amf.decodeObject(); err == nil {
		msg.StreamId = uint32(obj.(float64))
	}
}
func (msg *ResponseCreateStreamMessage) Decode3(head *RtmpHeader, body RtmpBody) {
	var tmpRtmpBody RtmpBody
	tmpRtmpBody.Payload = body.Payload[1:]
	msg.Decode0(head, tmpRtmpBody)
}

func (msg *ResponseCreateStreamMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponseCreateStreamMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponseCreateStreamMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponseCreateStreamMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Play Message
//
type ResponsePlayMessage struct {
	CommandMessage
	Object      interface{} `json:",omitempty"`
	Description string
}

func newResponsePlayMessage() *ResponsePlayMessage {
	return &ResponsePlayMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponsePlayMessage) Encode0() {
	amf := newAMFEncoder() // rtmp_amf.go
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))
	amf.writeNull()
	if msg.Object != nil {
		amf.encodeObject(msg.Object.(AMFObjects))
	}
	amf.writeString(msg.Description)
	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *ResponsePlayMessage) Encode3() {
}*/

func (msg *ResponsePlayMessage) Decode0(head *RtmpHeader, body RtmpBody) {
	amf := newAMFDecoder(body.Payload)
	if obj, err := amf.decodeObject(); err == nil {
		msg.CommandName = obj.(string)
	}
	if obj, err := amf.decodeObject(); err == nil {
		msg.TransactionId = uint64(obj.(float64))
	}

	obj, err := amf.decodeObject()
	if err == nil && obj != nil {
		msg.Object = obj
	} else if obj, err := amf.decodeObject(); err == nil {
		msg.Object = obj
	}
}
func (msg *ResponsePlayMessage) Decode3(head *RtmpHeader, body RtmpBody) {
	var tmpRtmpBody RtmpBody
	tmpRtmpBody.Payload = body.Payload[1:]
	msg.Decode0(head, tmpRtmpBody)
}

func (msg *ResponsePlayMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponsePlayMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponsePlayMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponsePlayMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Publish Message
//
type ResponsePublishMessage struct {
	CommandMessage
	Properties interface{} `json:",omitempty"`
	Infomation interface{} `json:",omitempty"`
}

func newResponsePublishMessage() *ResponsePublishMessage {
	return &ResponsePublishMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

// 命令名 -> 命令名,设置为"OnStatus"
// 传输ID -> 0
// 属性 -> null
// 信息 -> level, code, description

func (msg *ResponsePublishMessage) Encode0() {
	amf := newAMFEncoder()
	amf.writeString(msg.CommandName)
	amf.writeNumber(float64(msg.TransactionId))
	amf.writeNull()

	if msg.Properties != nil {
		amf.encodeObject(msg.Properties.(AMFObjects))
	}
	if msg.Infomation != nil {
		amf.encodeObject(msg.Infomation.(AMFObjects))
	}

	msg.RtmpBody.Payload = amf.Bytes()
}

/*
func (msg *ResponsePublishMessage) Encode3() {
}*/

func (msg *ResponsePublishMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponsePublishMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponsePublishMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponsePublishMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Seek Message
//
type ResponseSeekMessage struct {
	CommandMessage
	Description string
}

func newResponseSeekMessage() *ResponseSeekMessage {
	return &ResponseSeekMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponseSeekMessage) Encode0() {
}

//func (msg *ResponseSeekMessage) Encode3() {
//}

func (msg *ResponseSeekMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponseSeekMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponseSeekMessage) String() string {
	msg.RtmpBody = new(RtmpBody)
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponseSeekMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Pause Message
//
type ResponsePauseMessage struct {
	CommandMessage
	Description string
}

// 命令名 -> 命令名,设置为"OnStatus"
// 传输ID -> 0
// 描述

func (msg *ResponsePauseMessage) Encode0() {
}

//func (msg *ResponsePauseMessage) Encode3() {
//}

func (msg *ResponsePauseMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponsePauseMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponsePauseMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponsePauseMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

//
// Response Message
//
type ResponseMessage struct {
	CommandMessage
	Properties  interface{} `json:",omitempty"`
	Infomation  interface{} `json:",omitempty"`
	Description string
}

func newResponseMessage() *ResponseMessage {
	return &ResponseMessage{CommandMessage: CommandMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}
}

func (msg *ResponseMessage) Encode0() {
}

//func (msg *ResponseMessage) Encode3() {
//}

func (msg *ResponseMessage) Decode0(head *RtmpHeader, body RtmpBody) {
	amf := newAMFDecoder(body.Payload)
	if obj, err := amf.decodeObject(); err == nil {
		msg.CommandName = obj.(string)
	}
	if obj, err := amf.decodeObject(); err == nil {
		msg.TransactionId = uint64(obj.(float64))
	}
}

func (msg *ResponseMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *ResponseMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *ResponseMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("ResponseMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// User Control Message 4.
// The client or the server sends this message to notify the peer about the user control events.
// For information about the message format, see Section 6.2.

// The following user control event types are supported:

// Stream Begin (=0)
// The server sends this event to notify the client that a stream has become functional and can be
// used for communication. By default, this event is sent on ID 0 after the application connect
// command is successfully received from the client. The event data is 4-byte and represents
// the stream ID of the stream that became functional.
type StreamBeginMessage struct {
	UserControlMessage
	StreamID uint32
}

func newStreamBeginMessage() *StreamBeginMessage {
	return &StreamBeginMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *StreamBeginMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 6)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.StreamID)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *StreamBeginMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *StreamBeginMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *StreamBeginMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("StreamBeginMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Stream EOF (=1)
// The server sends this event to notify the client that the playback of data is over as requested
// on this stream. No more data is sent without issuing additional commands. The client discards the messages
// received for the stream. The 4 bytes of event data represent the ID of the stream on which playback has ended.
type StreamEOFMessage struct {
	UserControlMessage
	StreamID uint32
}

func newStreamEOFMessage() *StreamEOFMessage {
	return &StreamEOFMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *StreamEOFMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 6)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.StreamID)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *StreamEOFMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *StreamEOFMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *StreamEOFMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("StreamEOFMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Stream Dry (=2)
// The server sends this event to notify the client that there is no more data on the stream. If the
// server does not detect any message for a time period, it can notify the subscribed clients
// that the stream is dry. The 4 bytes of event data represent the stream ID of the dry stream.
type StreamDryMessage struct {
	UserControlMessage
	StreamID uint32
}

func newStreamDryMessage() *StreamDryMessage {
	return &StreamDryMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *StreamDryMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 6)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.StreamID)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *StreamDryMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *StreamDryMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *StreamDryMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("StreamDryMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// SetBuffer Length (=3)
// The client sends this event to inform the server of the buffer size (in milliseconds) that is
// used to buffer any data coming over a stream. This event is sent before the server starts |
// processing the stream. The first 4 bytes of the event data represent the stream ID and the next |
// 4 bytes represent the buffer length, in  milliseconds.
type SetBufferMessage struct {
	UserControlMessage
	StreamID    uint32
	Millisecond uint32
}

func newSetBufferMessage() *SetBufferMessage {
	return &SetBufferMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *SetBufferMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 10)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.StreamID)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[6:], msg.Millisecond)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *SetBufferMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *SetBufferMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *SetBufferMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("SetBufferMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// StreamIsRecorded (=4)
// The server sends this event to notify the client Recorded that the stream is a recorded stream.
// The 4 bytes event data represent the stream ID of the recorded stream.
type StreamIsRecordedMessage struct {
	UserControlMessage
	StreamID uint32
}

func newStreamIsRecordedMessage() *StreamIsRecordedMessage {
	return &StreamIsRecordedMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *StreamIsRecordedMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 6)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.StreamID)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *StreamIsRecordedMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *StreamIsRecordedMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *StreamIsRecordedMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("StreamIsRecordedMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// PingRequest (=6)
// The server sends this event to test whether the client is reachable. Event data is a 4-byte
// timestamp, representing the local server time when the server dispatched the command.
// The client responds with PingResponse on receiving MsgPingRequest.
type PingRequestMessage struct {
	UserControlMessage
	Timestamp uint32
}

func newPingRequestMessage() *PingRequestMessage {
	return &PingRequestMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *PingRequestMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 6)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	util.BigEndian.PutUint32(msg.RtmpBody.Payload[2:], msg.Timestamp)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *PingRequestMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *PingRequestMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *PingRequestMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("PingRequestMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// PingResponse (=7)
// The client sends this event to the server in response to the ping request. The event data is |
// a 4-byte timestamp, which was received with the PingRequest request.
type PingResponseMessage struct {
	UserControlMessage
}

func newPingResponseMessage() *PingResponseMessage {
	return &PingResponseMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *PingResponseMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 2)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *PingResponseMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *PingResponseMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *PingResponseMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("PingResponseMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}

// Buffer Empty (=31)
type BufferEmptyMessage struct {
	UserControlMessage
}

func newBufferEmptyMessage() *BufferEmptyMessage {
	return &BufferEmptyMessage{UserControlMessage: UserControlMessage{ControlMessage: ControlMessage{RtmpHeader: &RtmpHeader{}, RtmpBody: &RtmpBody{}}}}
}

func (msg *BufferEmptyMessage) Encode() {
	msg.RtmpBody.Payload = make([]byte, 2)
	util.BigEndian.PutUint16(msg.RtmpBody.Payload, msg.EventType)
	msg.EventData = msg.RtmpBody.Payload[2:]
}

func (msg *BufferEmptyMessage) Header() *RtmpHeader {
	return msg.RtmpHeader
}

func (msg *BufferEmptyMessage) Body() *RtmpBody {
	return msg.RtmpBody
}

func (msg *BufferEmptyMessage) String() string {
	if b, err := json.Marshal(*msg); err == nil {
		return fmt.Sprintf("BufferEmptyMessage [length:%d]: %v", len(msg.RtmpBody.Payload), string(b))
	} else {
		panic(err)
	}
}
