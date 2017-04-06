package rtmplog

import (
	//"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func WriteLog(text string) {
	var currentPath string
	var content string
	var err error

	logFile, _ := exec.LookPath(os.Args[0])
	logFilePath, _ := filepath.Abs(logFile)

	ss := strings.Split(logFilePath, "/")

	for _, v := range ss[:len(ss)-1] {
		currentPath += v + "/"
	}

	currentPath += "main.log"

	f, err := os.OpenFile(currentPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0664)
	if err != nil {
		panic(err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	pc, file, line, _ := runtime.Caller(2)
	function := runtime.FuncForPC(pc)

	content += "---------------"
	content += now
	content += "---------------\n"
	content += "error text : " + text + "\n"
	content += "function : " + function.Name() + "\n"
	content += "file : " + file + "\n"
	content += "line : " + strconv.Itoa(line) + "\n"

	pc, file, line, _ = runtime.Caller(1)
	function = runtime.FuncForPC(pc)

	content += "function : " + function.Name() + "\n"
	content += "file : " + file + "\n"
	content += "line : " + strconv.Itoa(line) + "\n"

	pc, file, line, _ = runtime.Caller(0)
	function = runtime.FuncForPC(pc)

	content += "function : " + function.Name() + "\n"
	content += "file : " + file + "\n"
	content += "line : " + strconv.Itoa(line) + "\n"

	content += "-------------------------------------------------\n"

	_, err = io.WriteString(f, content)
	if err != nil {
		panic(err)
	}
}
