# gortmp

## Client(IP : 192.168.2.99)

1. ffmpeg : push the desktop to the server.

    ffmpeg.exe -report -f dshow -i audio="virtual-audio-capturer" -f dshow -i video="screen-capture-recorder" -vcodec libx264 -acodec aac -s 1920*1080 -r 25 -g 25 -pix_fmt yuv420p -preset veryfast -tune zerolatency -f flv rtmp://192.168.2.100/myapp/mystream
   

## Server(IP : 192.168.2.100)

1. Run Server(need to install "golang" environment)。

    go run main.go
    

## Pull Live Stream(IP : 192.168.2.99)

1. ffplay.

    ffplay.exe -i rtmp://192.168.2.100/myapp/mystream -fflags nobuffer
    
## Live Delay

1. LAN delay in about 0.5 seconds.

2. Delay picture.
![image](https://github.com/sevenzoe/Photos/raw/master/Delay.png)

