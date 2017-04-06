package rtmp

import (
	"errors"
	"fmt"
)

type ServerHandler interface {
	OnPublishing(s *RtmpNetStream) error
	OnPlaying(s *RtmpNetStream) error
	OnClosed(s *RtmpNetStream)
	OnError(s *RtmpNetStream, err error)
}

type DefaultServerHandler struct {
}

// 发布者成功发布流后,就启动广播
func (p *DefaultServerHandler) OnPublishing(s *RtmpNetStream) error {
	// 在广播中发现这个广播已经存在,那么就认为这个广播是无效的.(例如已经发布ip/myapp/mystream这个广播,再次发布ip/app/mystream,就认为这个广播是无效的)
	if _, ok := find_broadcast(s.streamPath); ok {
		return errors.New("NetStream.Publish.BadName")
	}

	start_broadcast(s, 5, 5)

	return nil
}

// 订阅者成功订阅流后,就将订阅者添加进广播中
func (p *DefaultServerHandler) OnPlaying(s *RtmpNetStream) error {
	// 根据订阅者(s)提供的信息,来查找订阅者需要订阅的广播,如果找到了,那么就让这个广播添加这个订阅者
	if d, ok := find_broadcast(s.streamPath); ok {
		d.addSubscriber(s)
		return nil
	}

	return errors.New("NetStream.Play.StreamNotFound")
}

func (dsh *DefaultServerHandler) OnClosed(s *RtmpNetStream) {
	mode := "UNKNOWN"
	if s.mode == 2 {
		mode = "CONSUMER"
	} else if s.mode == 3 {
		mode = "PROXY"
	} else if s.mode == 2|1 {
		mode = "PRODUCER|CONSUMER"
	}

	fmt.Printf("NetStream OnClosed, remoteAddr : %v\npath : %v\nmode : %v\n", s.conn.remoteAddr, s.streamPath, mode)

	if d, ok := find_broadcast(s.streamPath); ok {
		if s.mode == 1 {
			d.stop()
		} else if s.mode == 2 {
			d.removeSubscriber(s)
		} else if s.mode == 2|1 {
			d.removeSubscriber(s)
			d.stop()
		}
	}
}

func (dsh *DefaultServerHandler) OnError(s *RtmpNetStream, err error) {
	fmt.Printf("NetStream OnError, remoteAddr : %v\npath : %v\nerror : %v\n", s.conn.remoteAddr, s.streamPath, err)
	s.Close()
}
