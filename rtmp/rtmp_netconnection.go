package rtmp

import (
	"bufio"
	"net"
	"sync"
	"time"
)

// http://help.adobe.com/zh_CN/FlashPlatform/reference/actionscript/3/flash/net/NetConnection.html

type NetConnection interface {
	Connect(command string, args ...interface{}) error
	Call(command string, args ...interface{}) error
	Close()
	Connected() bool
	URL() string
}

type RtmpNetConnection struct {
	remoteAddr         string
	url                string
	appName            string
	server             *Server
	readChunkSize      int
	writeChunkSize     int
	createTime         string
	bandwidth          uint32
	readSeqNum         uint32                      // 当前读的字节
	writeSeqNum        uint32                      // 当前写的字节
	totalWrite         uint32                      // 总共写了多少字节
	totalRead          uint32                      // 总共读了多少字节
	objectEncoding     float64                     // NetConnection对象的默认对象编码
	conn               net.Conn                    // conn
	br                 *bufio.Reader               // Read
	bw                 *bufio.Writer               // Write
	brw                *bufio.ReadWriter           // Read and Write,用来握手
	lock               *sync.Mutex                 // lock
	incompleteRtmpBody map[uint32][]byte           // 完整的RtmpBody,在网络上是被分成一块一块的,需要将其组装起来
	rtmpHeader         map[uint32]*RtmpHeader      // RtmpHeader
	connected          bool                        // 连接是否完成
	nextStreamID       func(chunkid uint32) uint32 // 下一个流ID
	streamID           uint32                      // 流ID
}

var gstreamid = uint32(64)

func gen_next_stream_id(chunkid uint32) uint32 {
	gstreamid += 1
	return gstreamid
}

func newRtmpNetConnect(conn net.Conn, s *Server) (c *RtmpNetConnection) {
	c = new(RtmpNetConnection)
	c.br = bufio.NewReader(conn)
	c.bw = bufio.NewWriter(conn)
	c.brw = bufio.NewReadWriter(c.br, c.bw)
	c.conn = conn
	c.lock = new(sync.Mutex)
	c.server = s
	c.bandwidth = RTMP_MAX_CHUNK_SIZE * 8
	c.createTime = time.Now().String()
	c.remoteAddr = conn.RemoteAddr().String()
	c.nextStreamID = gen_next_stream_id
	c.readChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	c.writeChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	c.rtmpHeader = make(map[uint32]*RtmpHeader)
	c.incompleteRtmpBody = make(map[uint32][]byte)
	c.objectEncoding = 0
	return
}

func (c *RtmpNetConnection) Connect(command string, args ...interface{}) error {
	return nil
}

func (c *RtmpNetConnection) Call(command string, args ...interface{}) error {
	return nil
}

func (c *RtmpNetConnection) Connected() bool {
	return c.connected
}

func (c *RtmpNetConnection) Close() {
	if c.conn == nil {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.conn.Close()
	c.connected = false
}

func (c *RtmpNetConnection) URL() string {
	return c.url
}
