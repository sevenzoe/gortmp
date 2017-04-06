package hls

import (
	"../util"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	HLS_KEY_METHOD_AES_128 = "AES-128"
)

// https://datatracker.ietf.org/doc/draft-pantos-http-live-streaming/

// 以”#EXT“开头的表示一个”tag“,否则表示注释,直接忽略
type Playlist struct {
	ExtM3U         string      // indicates that the file is an Extended M3U [M3U] Playlist file. (4.3.3.1) -- 每个M3U文件第一行必须是这个tag.
	Version        int         // indicates the compatibility version of the Playlist file. (4.3.1.2) -- 协议版本号.
	Sequence       int         // indicates the Media Sequence Number of the first Media Segment that appears in a Playlist file. (4.3.3.2) -- 第一个媒体段的序列号.
	Targetduration int         // specifies the maximum Media Segment duration. (4.3.3.1) -- 每个视频分段最大的时长(单位秒).
	PlaylistType   int         // rovides mutability information about the Media Playlist file. (4.3.3.5) -- 提供关于PlayList的可变性的信息.
	Discontinuity  int         // indicates a discontinuity between theMedia Segment that follows it and the one that preceded it. (4.3.2.3) -- 该标签后边的媒体文件和之前的媒体文件之间的编码不连贯(即发生改变)(场景用于插播广告等等).
	Key            PlaylistKey // specifies how to decrypt them. (4.3.2.4) -- 解密媒体文件的必要信息(表示怎么对media segments进行解码).
	EndList        string      // indicates that no more Media Segments will be added to the Media Playlist file. (4.3.3.4) -- 标示没有更多媒体文件将会加入到播放列表中,它可能会出现在播放列表文件的任何地方,但是不能出现两次或以上.
	Inf            PlaylistInf // specifies the duration of a Media Segment. (4.3.2.1) -- 指定每个媒体段(ts)的持续时间.
}

// Discontinuity :
// file format
// number, type and identifiers of tracks
// timestamp sequence
// encoding parameters
// encoding sequence

type PlaylistKey struct {
	Method string // specifies the encryption method. (4.3.2.4)
	Uri    string // key url. (4.3.2.4)
	IV     string // key iv. (4.3.2.4)
}

type PlaylistInf struct {
	Duration float64
	Title    string
}

func (this *Playlist) Init(filename string) (err error) {
	defer this.handleError()

	if util.Exist(filename) {
		if err = os.Remove(filename); err != nil {
			return
		}
	}

	var file *os.File
	file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	// ss := fmt.Sprintf("#EXTM3U\n"+
	// 	"#EXT-X-VERSION:%d\n"+
	// 	"#EXT-X-MEDIA-SEQUENCE:%d\n"+
	// 	"#EXT-X-TARGETDURATION:%d\n"+
	// 	"#EXT-X-PLAYLIST-TYPE:%d\n"+
	// 	"#EXT-X-DISCONTINUITY:%d\n"+
	// 	"#EXT-X-KEY:METHOD=%s,URI=%s,IV=%s\n"+
	// 	"#EXT-X-ENDLIST", hls.Version, hls.Sequence, hls.Targetduration, hls.PlaylistType, hls.Discontinuity, hls.Key.Method, hls.Key.Uri, hls.Key.IV)

	ss := fmt.Sprintf("#EXTM3U\n"+
		"#EXT-X-VERSION:%d\n"+
		"#EXT-X-MEDIA-SEQUENCE:%d\n"+
		"#EXT-X-TARGETDURATION:%d\n", this.Version, this.Sequence, this.Targetduration)

	if _, err = file.WriteString(ss); err != nil {
		return
	}

	file.Close()

	return
}

func (this *Playlist) WriteInf(filename string, inf PlaylistInf) (err error) {
	defer this.handleError()

	var file *os.File
	file, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	ss := fmt.Sprintf("#EXTINF:%.3f,\n"+
		"%s\n", inf.Duration, inf.Title)

	if _, err = file.WriteString(ss); err != nil {
		return
	}

	file.Close()

	return
}

func (this *Playlist) UpdateInf(filename string, tmpFilename string, inf PlaylistInf) (err error) {
	var oldContent []string
	var newContent string

	var tmpFile *os.File
	tmpFile, err = os.OpenFile(tmpFilename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer tmpFile.Close()

	var ls []string
	if ls, err = util.ReadFileLines(filename); err != nil {
		return
	}

	for i, v := range ls {
		if strings.Contains(v, "#EXT-X-MEDIA-SEQUENCE") {
			var index, seqNum int
			var oldStrNum, newStrNum, newSeqStr string

			if index = strings.Index(v, ":"); index == -1 {
				err = errors.New("#EXT-X-MEDIA-SEQUENCE not found.")
				return
			}

			oldStrNum = v[index+1:]

			seqNum, err = strconv.Atoi(oldStrNum)
			if err != nil {
				return
			}

			seqNum++

			newStrNum = strconv.Itoa(seqNum)

			newSeqStr = strings.Replace(v, oldStrNum, newStrNum, 1)

			ls[i] = newSeqStr
		}

		if strings.Contains(v, "#EXTINF") {
			oldContent = append(ls[0:i], ls[i+2:]...)
			break
		}
	}

	for _, v := range oldContent {
		newContent += v + "\n"
	}

	ss := fmt.Sprintf("#EXTINF:%.3f,\n"+
		"%s\n", inf.Duration, inf.Title)

	newContent += ss

	if _, err = tmpFile.WriteString(newContent); err != nil {
		return
	}

	if err = tmpFile.Close(); err != nil {
		return
	}

	if err = os.Remove(filename); err != nil {
		return
	}

	if err = os.Rename(tmpFilename, filename); err != nil {
		return
	}

	return
}

func (this *Playlist) GetInfCount(filename string) (num int, err error) {
	var ls []string
	if ls, err = util.ReadFileLines(filename); err != nil {
		return
	}

	num = 0
	for _, v := range ls {
		if strings.Contains(v, "#EXTINF") {
			num++
		}
	}

	return
}

func (this *Playlist) handleError() {
	if err := recover(); err != nil {
		fmt.Println(err)
	}
}
