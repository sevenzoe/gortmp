package config

import (
	"../util"
	"errors"
	"os"
	"strconv"
	"strings"
)

var (
	AppName          string
	DebugMode        bool
	HLSEnabled       bool
	HLSFragment      int64
	HLSWindow        int
	HLSPath          string
	ResourcePath     string // 资源文件的路径
	ResourceLivePath string // 资源文件的路径
	ResourceVodPath  string // 资源文件的路径
	ResourceTempPath string // 资源文件的路径
)

type Config struct {
	File    *os.File
	Secions map[string]section
}

type section struct {
	Name   string
	Fields map[string]string
}

func (cfg *Config) Init(filename string) (err error) {
	cfg.Secions = make(map[string]section)

	cfg.File, err = os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return
	}

	var ls []string
	ls, err = util.ReadFileLines(filename)
	if err != nil {
		return
	}

	var sectionName string
	for _, v := range ls {

		// 去掉左右空格
		v = strings.TrimSpace(v)

		// 空行
		if v == "" {
			continue
		}

		// 注释
		if strings.HasPrefix(v, "#") {
			continue
		}

		// section
		if strings.HasPrefix(v, "[") {
			if strings.HasSuffix(v, "]") {
				sectionName = v[1 : len(v)-1]

				continue
			}
		}

		index := strings.Index(v, "=")
		if index < 0 {
			return errors.New("Init error! strings.Index(=) < 0.")
		}

		key := strings.TrimSpace(v[:index])
		if len(key) == 0 {
			return errors.New("Init error, len(key) == 0.")
		}

		value := strings.TrimSpace(v[index+1:])
		if len(value) == 0 {
			return errors.New("Init error, len(value) == 0.")
		}

		if _, ok := cfg.Secions[sectionName]; ok {
			cfg.Secions[sectionName].Fields[key] = value
		} else {
			sec := section{
				Name: sectionName,
				Fields: map[string]string{
					key: value,
				},
			}

			cfg.Secions[sectionName] = sec
		}
	}

	if err = cfg.initBaseData(); err != nil {
		return
	}

	return
}

func (cfg *Config) Read(sectionName string, key string) (value string, err error) {
	var ok bool
	var sec section
	if sec, ok = cfg.Secions[sectionName]; ok {
		if value, ok = sec.Fields[key]; ok {
			return
		} else {
			err = errors.New("key not found!")
			return
		}
	} else {
		err = errors.New("section not found!")
		return
	}
	return
}

func (cfg *Config) initBaseData() (err error) {
	var dir, value string

	if value, err = cfg.Read("App", "Name"); err != nil {
		AppName = "sr_rtmp"
	} else {
		AppName = value
	}

	if value, err = cfg.Read("App", "DebugMode"); err != nil {
		DebugMode = false
	} else {
		if value == "on" {
			DebugMode = true
		} else {
			DebugMode = false
		}
	}

	if value, err = cfg.Read("HLS", "Enabled"); err != nil {
		HLSEnabled = false
	} else {
		if value == "on" {
			HLSEnabled = true
		} else {
			HLSEnabled = false
		}
	}

	if HLSEnabled {
		if value, err = cfg.Read("HLS", "HLS_Path"); err != nil {
			HLSPath = "./"
		} else {
			HLSPath = value
		}

		if value, err = cfg.Read("HLS", "HLS_Fragment"); err != nil {
			HLSFragment = 0
		} else {
			var v int64
			if v, err = strconv.ParseInt(value, 10, 32); err != nil {
				HLSFragment = 0
			} else {
				HLSFragment = v
			}
		}

		if value, err = cfg.Read("HLS", "HLS_Window"); err != nil {
			HLSWindow = 3
		} else {
			var v int
			if v, err = strconv.Atoi(value); err != nil {
				HLSWindow = 3
			} else {
				HLSWindow = v
			}
		}
	}

	if dir, err = os.Getwd(); err != nil {
		return
	}

	ResourcePath = dir + "/resource"
	ResourceVodPath = ResourcePath + "/vod"
	ResourceTempPath = ResourcePath + "/temp"
	ResourceLivePath = ResourcePath + "/live"

	if !util.Exist(ResourcePath) {
		if err = os.Mkdir(ResourcePath, os.ModePerm); err != nil {
			return
		}
	}

	if !util.Exist(ResourceTempPath) {
		if err = os.Mkdir(ResourceTempPath, os.ModePerm); err != nil {
			return
		}
	}

	if !util.Exist(ResourceLivePath) {
		if err = os.Mkdir(ResourceLivePath, os.ModePerm); err != nil {
			return
		}
	}

	if !util.Exist(ResourceVodPath) {
		if err = os.Mkdir(ResourceVodPath, os.ModePerm); err != nil {
			return
		}
	}

	return
}
