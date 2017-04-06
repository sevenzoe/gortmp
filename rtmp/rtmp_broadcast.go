package rtmp

import (
	//"../config"
	//"../util"
	"fmt"
	//"strings"
	//"os"
	"sync"
	"time"
)

var (
	broadcasts = make(map[string]*Broadcast)
)

// 一个Broadcast代表着服务器已经在发布一个流,如果有多个客户端推流上来,那么服务器会有多个Broadcast.
// 客户端订阅的时候,会选择订阅哪个Broadcast.然后通过Broadcast将订阅者和发布者联系起来.

type Broadcast struct {
	lock       *sync.Mutex               // lock
	publisher  *RtmpNetStream            // 发布者
	subscriber map[string]*RtmpNetStream // 订阅者
	streamPath string                    // 发布者发布的流路径
	control    chan interface{}          // 订阅者的控制,包括play,stop...
}

type AVChannel struct {
	id    string
	audio chan *AVPacket
	video chan *AVPacket
}

func find_broadcast(path string) (*Broadcast, bool) {
	v, ok := broadcasts[path]
	return v, ok
}

func start_broadcast(publisher *RtmpNetStream, vl, al int) {
	av := &AVChannel{
		id:    publisher.conn.remoteAddr,
		audio: make(chan *AVPacket, al), // 开辟一个音频通道
		video: make(chan *AVPacket, vl)} // 开辟一个视频通道

	publisher.AttachAudio(av.audio) // 发布者发布的音频全部流入这个通道
	publisher.AttachVideo(av.video) // 发布者发布的视频全部流入这个通道

	b := &Broadcast{
		streamPath: publisher.streamPath,               // 发布者的流路径
		lock:       new(sync.Mutex),                    // lock
		publisher:  publisher,                          // 发布者信息, *RtmpNetStream
		subscriber: make(map[string]*RtmpNetStream, 0), // 订阅者信息, map[string]*RtmpNetStream
		control:    make(chan interface{}, 10)}         // 订阅者的控制

	broadcasts[publisher.streamPath] = b // 添加广播

	b.start()
}

func (b *Broadcast) addSubscriber(s *RtmpNetStream) {
	// Broadcast 其实就是一个发布者发布的广播.
	// RtmpNetStream 其实就是一个订阅者.
	// broadcasts 就是装载着所有发布的广播.
	// 有可能有多个订阅者订阅同一个广播.因此将订阅者和发布者的信息相关联起来.在后面发送数据给订阅者的时候,需要拿出发布者数据Tag.

	// 订阅者s订阅的广播是b
	// 广播b接受订阅者s的控制
	s.broadcast = b
	b.control <- s // 这里会添加订阅者
}

func (b *Broadcast) removeSubscriber(s *RtmpNetStream) {
	s.closed = true
	b.control <- s
}

func (b *Broadcast) stop() {
	delete(broadcasts, b.streamPath)
	b.control <- "stop"
}

func (b *Broadcast) start() {
	go func(b *Broadcast) {
		defer func() {
			if e := recover(); e != nil {
				fmt.Println(e)
			}

			fmt.Println("Broadcast :" + b.streamPath + " stopped")
		}()

		// begintime --> server.go
		d := time.Now().Sub(begintime)
		fmt.Printf("------------Intreval Time :%v ------------\n", d)

		// Implement io.Writer. Write the specified file format.
		// file format is RTMP_FILE_TYPE_HLS_TS, WriteAudio() or WriteVideo(), The first parameter fill nil.
		//
		// write ts file (format : RTMP_FILE_TYPE_TS).
		// filename := config.ResourceLivePath + "/live.ts"
		// if util.Exist(filename) {
		// 	if err := os.Remove(filename); err != nil {
		// 		fmt.Println("rtmp remove file error:", err)
		// 	}
		// }

		// file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
		// if err != nil {
		// 	fmt.Println("rtmp write file error:", err)
		// }
		// defer file.Close()

		b.publisher.astreamToFile = true
		b.publisher.vstreamToFile = true
		b.publisher.rtmpFile = newRtmpFile()

		// SendAudio(),函数接收的参数是(audio *AVPacket)
		// 如果不拷贝一份数据传递过去,那么如果在SendAudio()函数内部,如果改变了audio这个参数的值,将会影响数据的正确性
		for {
			select {
			case amsg := <-b.publisher.audiochan: // 取出发布者中的音频数据
				{
					for _, s := range b.subscriber { // 订阅者
						err := s.SendAudio(amsg.Clone()) // 给订阅者发送音频数据
						if err != nil {
							s.serverHandler.OnError(s, err)
						}
					}

					// write file
					if b.publisher.astreamToFile {
						err := b.publisher.WriteAudio(nil, amsg.Clone(), RTMP_FILE_TYPE_HLS_TS)
						if err != nil {
							// handler error
							fmt.Println("wirte audio file error :", err)
						}
					}
				}
			case vmsg := <-b.publisher.videochan: // 取出发布者中的视频数据
				{
					for _, s := range b.subscriber { // 订阅者
						err := s.SendVideo(vmsg.Clone()) // 给订阅者发送视频数据
						if err != nil {
							s.serverHandler.OnError(s, err)
						}
					}

					// write file
					if b.publisher.vstreamToFile {
						err := b.publisher.WriteVideo(nil, vmsg.Clone(), RTMP_FILE_TYPE_HLS_TS)
						if err != nil {
							// handler error
							fmt.Println("wirte video file error :", err)
						}
					}
				}
			case obj := <-b.control: // 订阅者的控制.例如订阅者开始播放,或者取消播放都会到这里先处理.会打印消费者信息.
				{
					if c, ok := obj.(*RtmpNetStream); ok {
						if c.closed {
							delete(b.subscriber, c.conn.remoteAddr)
							fmt.Println("Subscriber Closed, Broadcast :", b.streamPath, "\nSubscribe :", len(b.subscriber))
						} else {
							b.subscriber[c.conn.remoteAddr] = c                                                           // 添加订阅者
							fmt.Println("Subscriber Open, Broadcast :", b.streamPath, "\nSubscribe :", len(b.subscriber)) // 打印信息
						}
					} else if v, ok := obj.(string); ok && "stop" == v {
						for k, ss := range b.subscriber { // k == string, ss = RtmpNetStream
							delete(b.subscriber, k) // 删除订阅者
							ss.Close()              // 关闭RtmpNetStream
						}
					}
				}
			case <-time.After(time.Second * 100):
				{
					fmt.Println("Broadcast " + b.streamPath + " Video | Audio Buffer Empty,Timeout 100s")
					b.stop()
					b.publisher.Close()
					return
				}
			}
		}
	}(b)
}
