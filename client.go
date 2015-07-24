package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"encoding/json"
	"code.google.com/p/go-uuid/uuid"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"path/filepath"
	"strings"
	"time"
)

type HttpError interface {
	error
	StatusCode() int
}

// Errors
type HandlingError struct {
	message string
}

func (self *HandlingError) StatusCode() int {
	return 500
}

func (self *HandlingError) Error() string {
	return self.message
}

func NewHandlingError(message string) HttpError {
	return &HandlingError{message}
}

type httpError struct {
	statusCode int
	message    string
}

func (self *httpError) Error() string {
	return self.message
}

func (self *httpError) StatusCode() int {
	return self.statusCode
}

func NewHttpError(statusCode int, message string) HttpError {
	return &httpError{statusCode, message}
}

func safePath(rootPath string, path string) (string, error) {
	absPath, err := filepath.Abs(filepath.Join(rootPath, path))
	if err != nil {
		return "", NewHttpError(500, err.Error())
	}
	if !strings.HasPrefix(absPath, rootPath) {
		return "", NewHandlingError("Hacking attempt")
	}
	return absPath, nil
}

var writeLock = make(map[string]chan bool)

type RootedRPCHandler struct {
        rootPath string
}

func (self *RootedRPCHandler) handleRequest(requestChannel chan []byte, responseChannel chan []byte, closeChannel chan bool) {
	commandBuffer, ok := <-requestChannel
	if !ok {
		return
	}
	command := string(commandBuffer)
	// headers
	_, ok = <-requestChannel
	if !ok {
		return
	}

	var err HttpError
	commandParts := strings.Split(command, " ")
	method := commandParts[0]
	path := strings.Join(commandParts[1:], "/")
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	switch method {
	case "GET":
		err = self.handleGet(path, requestChannel, responseChannel)
	case "HEAD":
		err = self.handleHead(path, requestChannel, responseChannel)
	case "PUT":
		err = self.handlePut(path, requestChannel, responseChannel)
	case "DELETE":
		err = self.handleDelete(path, requestChannel, responseChannel)
	case "POST":
		err = self.handlePost(path, requestChannel, responseChannel)
	}
	if err != nil {
		sendError(responseChannel, err, commandParts[0] != "HEAD")
	}
	responseChannel <- DELIMITERBUFFER
	closeChannel <- true
}

func sendError(responseChannel chan []byte, err HttpError, withMessageInBody bool) {
	responseChannel <- statusCodeBuffer(err.StatusCode())

	if withMessageInBody {
		responseChannel <- headerBuffer(map[string]string{"Content-Type": "text/plain"})
		responseChannel <- []byte(err.Error())
	} else {
		responseChannel <- headerBuffer(map[string]string{"Content-Length": "0"})
	}
}

func dropUntilDelimiter(requestChannel chan []byte) {
	for {
		buffer, ok := <-requestChannel
		if !ok {
			break
		}
		if IsDelimiter(buffer) {
			break
		}
	}
}

func headerBuffer(headers map[string]string) []byte {
	var headerBuffer bytes.Buffer
	for h, v := range headers {
		headerBuffer.Write([]byte(fmt.Sprintf("%s: %s\n", h, v)))
	}
	bytes := headerBuffer.Bytes()
	return bytes[:len(bytes)-1]
}

func statusCodeBuffer(code int) []byte {
	return IntToBytes(code)
}

func waitForLock(path string) {
	if writeLock[path] != nil {
		<-writeLock[path]
	}
}

func (self *RootedRPCHandler) handleGet(path string, requestChannel chan []byte, responseChannel chan []byte) HttpError {
	waitForLock(path)

	dropUntilDelimiter(requestChannel)
	safePath, err := safePath(self.rootPath, path)
	if err != nil {
		return err.(HttpError)
	}
	stat, err := os.Stat(safePath)
	if err != nil {
		return NewHttpError(404, "Not found")
	}
	responseChannel <- statusCodeBuffer(200)
	if stat.IsDir() {
		responseChannel <- headerBuffer(map[string]string{"Content-Type": "text/plain"})
		files, _ := ioutil.ReadDir(safePath)
		for _, f := range files {
			if f.Name()[0] == '.' {
				continue
			}
			if f.IsDir() {
				responseChannel <- []byte(fmt.Sprintf("%s/\n", f.Name()))
			} else {
				responseChannel <- []byte(fmt.Sprintf("%s\n", f.Name()))
			}
		}
	} else { // File
		mimeType := mime.TypeByExtension(filepath.Ext(safePath))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		responseChannel <- headerBuffer(map[string]string{
			"Content-Type": mimeType,
			"ETag":         stat.ModTime().String(),
		})
		f, err := os.Open(safePath)
		if err != nil {
			return NewHttpError(500, "Could not open file")
		}
		defer f.Close()
		for {
			buffer := make([]byte, BUFFER_SIZE)
			n, _ := f.Read(buffer)
			if n == 0 {
				break
			}
			responseChannel <- buffer[:n]
		}
	}
	return nil
}

func (self *RootedRPCHandler) handleHead(path string, requestChannel chan []byte, responseChannel chan []byte) HttpError {
	waitForLock(path)

	safePath, err := safePath(self.rootPath, path)
	dropUntilDelimiter(requestChannel)
	if err != nil {
		return err.(HttpError)
	}
	stat, err := os.Stat(safePath)
	if err != nil {
		return NewHttpError(404, "Not found")
	}
	responseChannel <- statusCodeBuffer(200)
	fileType := "file"
	if stat.IsDir() {
		fileType = "directory"
	}
	responseChannel <- headerBuffer(map[string]string{
		"ETag":           stat.ModTime().String(),
		"Content-Length": "0",
		"X-Type": fileType,
	})
	return nil
}

func (self *RootedRPCHandler) handlePut(path string, requestChannel chan []byte, responseChannel chan []byte) HttpError {
	if writeLock[path] != nil {
		// Already writing
		dropUntilDelimiter(requestChannel)
		return NewHttpError(500, "Write already going on")
	}

	writeLock[path] = make(chan bool)

	defer func() {
		close(writeLock[path])
		writeLock[path] = nil
	}()

	safePath, err := safePath(self.rootPath, path)
	if err != nil {
		dropUntilDelimiter(requestChannel)
		return err.(HttpError)
	}
	dir := filepath.Dir(safePath)
	os.MkdirAll(dir, 0777)

	// To avoid corrupted files, we'll write to a temp path first
	tempPath := dir + "/.zedtmp." + uuid.New()
	f, err := os.Create(tempPath)
	if err != nil {
		dropUntilDelimiter(requestChannel)
		return NewHttpError(500, fmt.Sprintf("Could not create file: %s", tempPath))
	}
	for {
		buffer := <-requestChannel
		if IsDelimiter(buffer) {
			break
		}
		_, err := f.Write(buffer)
		if err != nil {
			dropUntilDelimiter(requestChannel)
			return NewHttpError(500, "Could not write to file")
		}
	}
	f.Sync()
	f.Close()

        // Get existing file permissions
	var mode os.FileMode = 0666
	stat, err := os.Stat(safePath)
	if err == nil {
		mode = stat.Mode()
	}
	// And then copy it over again

        f, err = os.Open(tempPath)
        if err != nil {
                return NewHttpError(500, fmt.Sprintf("Could not read temporary file for copy: %s", tempPath))
        }
	fout, err := os.OpenFile(safePath, os.O_WRONLY | os.O_TRUNC | os.O_CREATE, mode)
        if err != nil {
                return NewHttpError(500, fmt.Sprintf("Could not open target file for copy: %s", safePath))
        }

        io.Copy(fout, f)
        f.Close()
        fout.Close()

        os.Remove(tempPath)

//         if err := os.Chmod(tempPath, mode); err != nil {
//                 return NewHttpError(500, "unable to chmod tmpfile: " + err.Error())
//         }

//         // Rename the temp file to a the real file. This is done "atomically",
//         // so that even if something goes weird, we'll either have an old or new version.
//         if err := os.Rename(tempPath, safePath); err != nil {
//                 return NewHttpError(500, "Unable to replace old version: " + err.Error())
//         }

	stat, _ = os.Stat(safePath)
	responseChannel <- statusCodeBuffer(200)
	responseChannel <- headerBuffer(map[string]string{
		"Content-Type": "text/plain",
		"ETag":         stat.ModTime().String(),
	})
	responseChannel <- []byte("OK")
	return nil
}

func (self *RootedRPCHandler) handleDelete(path string, requestChannel chan []byte, responseChannel chan []byte) HttpError {
	waitForLock(path)

	safePath, err := safePath(self.rootPath, path)
	if err != nil {
	dropUntilDelimiter(requestChannel)
		return err.(HttpError)
	}
	_, err = os.Stat(safePath)
	if err != nil {
		return NewHttpError(404, "Not found")
	}
	err = os.Remove(safePath)
	if err != nil {
		return NewHttpError(500, "Could not delete")
	}
	responseChannel <- statusCodeBuffer(200)
	responseChannel <- headerBuffer(map[string]string{
		"Content-Type": "text/plain",
	})
	responseChannel <- []byte("OK")

	return nil
}

func walkDirectory(responseChannel chan []byte, root string, path string) {
	files, _ := ioutil.ReadDir(filepath.Join(root, path))
	for _, f := range files {
		if f.IsDir() {
			walkDirectory(responseChannel, root, filepath.Join(path, f.Name()))
		} else {
			responseChannel <- []byte(fmt.Sprintf("/%s\n", filepath.Join(path, f.Name())))
		}
	}
}

func readWholeBody(requestChannel chan []byte) []byte {
	var byteBuffer bytes.Buffer
	for {
		buffer := <-requestChannel
		if IsDelimiter(buffer) {
			break
		}
		byteBuffer.Write(buffer)
	}
	return byteBuffer.Bytes()
}

func (self *RootedRPCHandler) handlePost(path string, requestChannel chan []byte, responseChannel chan []byte) HttpError {
	safePath, err := safePath(self.rootPath, path)
	body := string(readWholeBody(requestChannel))
	if err != nil {
		return err.(HttpError)
	}
	_, err = os.Stat(safePath)
	if err != nil {
		return NewHttpError(http.StatusNotFound, "Not found")
	}

	queryValues, err := url.ParseQuery(body)
	if err != nil {
		return NewHttpError(http.StatusInternalServerError, "Could not parse body as HTTP post")
	}

	action := queryValues["action"][0]
	switch action {
	case "filelist":
		responseChannel <- statusCodeBuffer(200)
		responseChannel <- headerBuffer(map[string]string{
			"Content-Type": "text/plain",
		})
		walkDirectory(responseChannel, safePath, "")
	case "version":
		responseChannel <- statusCodeBuffer(200)
		responseChannel <- headerBuffer(map[string]string{
			"Content-Type": "text/plain",
		})
		responseChannel <- []byte(PROTOCOL_VERSION)
	default:
		return NewHttpError(http.StatusNotImplemented, "No such action")
	}

	return nil
}

// Side-effect: writes to rootPath
func ParseClientFlags(args []string) (url string, userKey string, rootPath string) {
	config := ParseConfig()

	flagSet := flag.NewFlagSet("zedrem", flag.ExitOnError)
	var stats bool
	flagSet.StringVar(&url, "u", config.Client.Url, "URL to connect to")
	flagSet.StringVar(&userKey, "key", config.Client.UserKey, "User key to use")
	flagSet.BoolVar(&stats, "stats", false, "Whether to print go-routine count and memory usage stats periodically.")
	flagSet.Parse(args)
	if stats {
		go PrintStats()
	}
	if flagSet.NArg() == 0 {
        	rootPath = "."
	} else {
		rootPath = args[len(args)-1]
	}
	return
}

func ListenForSignals() {
        sigs := make(chan os.Signal, 1)
        signal.Notify(sigs,
                syscall.SIGHUP,
                syscall.SIGINT,
                syscall.SIGTERM,
                syscall.SIGQUIT)
        go func() {
                _ = <-sigs
                os.Exit(0)
        }()
}

func RunClient(url string, id string, userKey string, rootPath string) {
	rootPath, _ = filepath.Abs(rootPath)
        ListenForSignals()
	socketUrl := fmt.Sprintf("%s/clientsocket", url)
	var ws *websocket.Conn
	var timeout time.Duration = 1e8
	config, err := websocket.NewConfig(socketUrl, socketUrl)
	if err != nil {
		fmt.Println(err)
		return
	}
	config.TlsConfig = new(tls.Config)
	// Disable this when getting a proper certificate
	config.TlsConfig.InsecureSkipVerify = true
	for {
		time.Sleep(timeout)
		var err error
		ws, err = websocket.DialConfig(config)
		timeout *= 2
		if err != nil {
			fmt.Println("Could not yet connect:", err.Error(), ", trying again in", timeout)
		} else {
			break
		}
	}

	buffer, _ := json.Marshal(HelloMessage{"0.1", id, userKey})

	if _, err := ws.Write(buffer); err != nil {
		log.Fatal(err)
		return
	}
	connectUrl := strings.Replace(url, "ws://", "http://", 1)
	connectUrl = strings.Replace(connectUrl, "wss://", "https://", 1)
	multiplexer := NewRPCMultiplexer(ws, &RootedRPCHandler{rootPath})

        if userKey == "" {
        	fmt.Print("In the Zed application copy and paste following URL to edit:\n\n")
        	fmt.Printf("  %s/fs/%s\n\n", connectUrl, id)
        } else {
                fmt.Println("A Zed window should now open. If not, make sure Zed is running and configured with the correct userKey.")
        }
	fmt.Println("Press Ctrl-c to quit.")
	err = multiplexer.Multiplex()
	if err != nil {
		// TODO do this in a cleaner way (reconnect, that is)
		if err.Error() == "no-client" {
		        fmt.Printf("ERROR: Your Zed editor is not currently connected to zedrem server %s.\nBe sure Zed is running and the project picker is open.\n", url)
		} else {
		        RunClient(url, id, userKey, rootPath)
		}
	}
}
