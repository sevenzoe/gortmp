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
