# gortmp

server :
go run main.go

## 客户端(客户端机器IP地址: 192.168.2.99)

1. 使用ffmpeg推送桌面流去服务器.

    ffmpeg.exe -report -f dshow -i audio="virtual-audio-capturer" -f dshow -i video="screen-capture-recorder" -vcodec libx264 -acodec aac -s 1920*1080 -r 25 -g 25 -pix_fmt yuv420p -preset veryfast -tune zerolatency -f flv rtmp://192.168.2.100/myapp/mystream
   

## 服务器(服务器IP地址: 192.168.2.100)

1. 直接运行(需要安装golang环境)。

    go run main.go
    

## 直播端(直播端机器IP地址: 192.168.2.99)

1. 使用ffplay去观看.

    ffplay.exe -i rtmp://192.168.2.100/myapp/mystream -fflags nobuffer
    
## 延时效果

1. 局域网内延时大概在0.5秒左右.

2. 局域网内延时效果图:
![image](https://github.com/sevenzoe/Photos/raw/master/Delay.png)

****************************************************************************
****************************************************************************

## gortmp 目录


* avformat目录 : 存放音视频格式的一些结构体.
* config目录   : 读取配置文件.
* hls目录      : hls相关函数.
* mpegts目录   : mpegts相关函数.
* rtmp目录     : rtmp相关函数.(程序的主流程)
* rtmplog目录  : 日志.
* util目录     : 公共函数.

## gortmp server 流程

1. 监听1935端口.
2. 握手(handshake).
3. 接收"ConnectMessage"消息.
4. 发送"Acknowledgement Size Message"消息.
5. 发送"Set Peer Bandwidth Message"消息.
6. 发送"Stream Begin Message"消息.
7. 发送"Connect Response Message"消息.
8. 消息循环.(消息循环里面处理各种消息,例如音频,视频,元数据,创建流,发布流,开始流等等)

## rtmp目录

1. server.go
    
    完成了前期大部分的准备工作.

2. rtmp_handshake.go

    具体的握手处理.
    
3. rtmp_socket.go

    具体的收/发消息函数.所有服务器接收到的消息或者要发送给客户端的消息,都是在这里实现的.
    
    >* recvMessage() : 接收所有发送给服务器的消息,具体实现是readChunk().
    >* readChunk() : rtmp发送消息是以Chunk的形式的,因此这里是读取所有Chunk,从而知道是什么消息.
    >* readChunkStreamID() : 读取Chunk Stream ID.
    >* readChunkType() : 读Chunk Type.
    >* sendMessage() : 发送消息给客户端.具体实现writeMessage().
    >* writeMessage() : 具体实现发送消息给客户端.
    
    
4. rtmp_amf.go

    AMF编/解码的具体实现.
    
5. rtmp_chunk.go

    rtmp Chunk的类型.(一共4种)
    
6. rtmp_event.go

    rtmp 的事件类型.

7. rtmp_file.go

    实现了将rtmp的音视频流保存成文件.
    
8. rtmp_handler.go

    rtmp在接收到相应消息时的处理.
    
9. rtmp_msg.go

    里面包含了所有的rtmp消息.
    
10. rtmp_netconnection.go

    [NetConnection](http://help.adobe.com/zh_CN/FlashPlatform/reference/actionscript/3/flash/net/NetConnection.html)
    
11. rtmp_netstream.go

    [NetConnection](http://help.adobe.com/zh_CN/FlashPlatform/reference/actionscript/3/flash/net/NetStream.html)
    
    >* SendAudio() : 发送发布者音频给订阅者.
    >* SendVideo() : 发送发布者视频给订阅者.
    >* WriteAudio() : 将发布者音频写成文件.
    >* WriteVideo() : 将发布者视频写成文件.
    >* msgLoopProc() : 消息循环处理函数.所有的消息一开始都集中在这里,然后会去到具体的处理函数中.

12. rtmp_packet.go

    rtmp 音视频包的封装.

13. rtmp_broadcast.go

    rtmp 广播. **发布者/订阅者模型**.(**也就是生产者/消费者**)
    
    所有发布/订阅的流都在这里.
    
    例如 :
    
    1. push : 客户端推送一个流上来,假设这个流的名字为"myapp/mystream"那么我们简单的把"myapp/mystream"这个流标记为**发布者**并将其放入广播中.
    
    2. pull : 如果这个时候发现有拉流的情况,也即是有**订阅者**去订阅"myapp/mystream"这个流,那么我们就会从**发布者**中取出数据,发送给**订阅者**.
    
    具体如下 :
    
    发布者 : ffmpeg -i xxxx -f rtmp://ip/myapp/mystream
    
    订阅者 : ffplay -i rtmp://ip/myapp/mystream
    
    