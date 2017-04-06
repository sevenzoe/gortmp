package rtmp

import (
	//"bufio"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"
)

var begintime time.Time

var handler ServerHandler = new(DefaultServerHandler)

type Server struct {
	Addr        string
	Handler     ServerHandler
	ReadTimeout time.Duration
	WriteTimout time.Duration
	Lock        *sync.Mutex
}

func ListenAndServe(addr string) error {
	s := Server{
		Addr:        addr,                            // 服务器的IP地址和端口信息
		Handler:     handler,                         // 请求处理函数的路由复用器
		ReadTimeout: time.Duration(time.Second * 15), // timeout
		WriteTimout: time.Duration(time.Second * 15), // timeout
		Lock:        new(sync.Mutex)}                 // lock
	return s.ListenAndServer()
}

// golang http.ListenAndServer source code
func (s *Server) ListenAndServer() error {
	addr := s.Addr
	if addr == "" {
		addr = ":1935"
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		go s.loop(l)
	}

	return nil
}

func (s *Server) loop(listener net.Listener) error {
	defer listener.Close()
	var tempDelay time.Duration

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				fmt.Printf("rtmp: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}

		tempDelay = 0
		c := newRtmpNetConnect(conn, s)
		go s.serve(c)
	}
}

func (s *Server) serve(rtmpNetConn *RtmpNetConnection) {

	begintime = time.Now()

	/* Handshake */
	err := handshake(rtmpNetConn.brw) // 握手
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	// NetConnect
	// 1. 客户端发送命令消息中的“连接”(connect)到服务器,请求与一个服务应用实例建立连接
	// 2. 服务器接收到连接命令消息后,发送确认窗口大小(Window Acknowledgement Size)协议消息到客户端,同时连接到连接命令中提到的应用程序.
	// 3. 服务器发送设置带宽()协议消息到客户端.
	// 4. 客户端处理设置带宽协议消息后,发送确认窗口大小(Window Acknowledgement Size)协议消息到服务器端.
	// 5. 服务器发送用户控制消息中的“流开始”(Stream Begin)消息到客户端.
	// 6. 服务器发送命令消息中的“结果”(_result),通知客户端连接的状态.

	msg, err := recvMessage(rtmpNetConn) // 户端发送 connect 命令到服务器端以请求对服务器端应用实例的连接,在这里将其读出来
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	connect, ok := msg.(*ConnectMessage) // 收到 connect 消息
	if !ok || connect.CommandName != "connect" {
		fmt.Println("not recv rtmp connet message")
		rtmpNetConn.Close()
		return
	}

	data := decodeAMFObject(connect.Object, "app") // 客户端要连接到的服务应用名
	if data != nil {
		rtmpNetConn.appName, ok = data.(string)
		if !ok {
			fmt.Println("rtmp connet message <app> decode error")
			rtmpNetConn.Close()
			return
		}
	}

	// data = decodeAMFObject(connect.Object, "tcUrl") // url
	// if data != nil {
	// 	TcUrl := data.(string)
	// 	fmt.Println("tcurl :", TcUrl)
	// }

	data = decodeAMFObject(connect.Object, "objectEncoding") // AMF编码方法
	if data != nil {
		rtmpNetConn.objectEncoding, ok = data.(float64)
		if !ok {
			fmt.Println("rtmp connet message <objectEncoding> decode error")
			rtmpNetConn.Close()
			return
		}
	}

	err = sendMessage(rtmpNetConn, SEND_ACK_WINDOW_SIZE_MESSAGE, uint32(512<<10)) // 服务器端发送协议消息 '窗口确认大小' 到客户端
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	err = sendMessage(rtmpNetConn, SEND_SET_PEER_BANDWIDTH_MESSAGE, uint32(512<<10)) // 服务器端发送协议消息 '设置对端带宽' 到客户端
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	err = sendMessage(rtmpNetConn, SEND_STREAM_BEGIN_MESSAGE, nil) // 服务器端发送另一个用户控制消息 (StreamBegin) 类型的协议消息到客户端
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	crmd := newConnectResponseMessageData(rtmpNetConn.objectEncoding)

	err = sendMessage(rtmpNetConn, SEND_CONNECT_RESPONSE_MESSAGE, crmd) // 服务器端发送结果命令消息告知客户端连接状态 (success/fail)
	if err != nil {
		rtmpNetConn.Close()
		return
	}

	rtmpNetConn.connected = true

	/* NetStream */

	handler := s.Handler
	newNetStream(rtmpNetConn, handler).msgLoopProc()
}
