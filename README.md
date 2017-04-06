# gortmp
client :
ffmpeg.exe -report -f dshow -i audio="virtual-audio-capturer" -f dshow -i video="screen-capture-recorder" -vcodec libx264 -acodec aac -s 1920*1080 -r 25 -g 25 -pix_fmt yuv420p -preset veryfast -tune zerolatency -f flv rtmp://server_ip_address/myapp/mystream

server :
go run main.go


   

