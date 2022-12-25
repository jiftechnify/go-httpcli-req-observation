package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type reqPattern int

const (
	reqSinglePartWithLen reqPattern = iota + 1
	reqSinglePartWithoutLen
	reqSinglePartWithLen_wrong
	reqSinglePartWithBuffer
	reqSinglePartExplicitlyChunked
	reqMultipart
	reqPatternBound // sentinel value, invalid by itself
)

func (p reqPattern) String() string {
	switch p {
	case reqSinglePartWithLen:
		return "single-part with Content-Length"
	case reqSinglePartWithoutLen:
		return "single-part without Content-Length"
	case reqSinglePartWithLen_wrong:
		return "single-part with wrong Content-Length (setting the header directly)"
	case reqSinglePartWithBuffer:
		return "single-part without Content-Length, using *byets.Buffer"
	case reqSinglePartExplicitlyChunked:
		return "single-part using *bytes.Buffer, setting 'Transfer-Encding: chunked' explicitly"
	case reqMultipart:
		return "multipart"
	default:
		return ""
	}
}

func (p reqPattern) NeedsLen() bool {
	return p == reqSinglePartWithLen || p == reqSinglePartWithLen_wrong
}

const serverPort = 8080

var serverURL = fmt.Sprintf("http://localhost:%d", serverPort)

func main() {
	var (
		filename string
	)

	flag.StringVar(&filename, "f", "", "file name")
	flag.Parse()

	if filename == "" {
		filename = "photo.jpg"
	}

	l, err := startServer()
	if err != nil {
		log.Fatal(err)
	}

	for p := reqSinglePartWithLen; p < reqPatternBound; p++ {
		go serve(l)
		time.Sleep(100 * time.Millisecond)

		if err := request(p, filename); err != nil {
			msg := err.Error()
			if !strings.Contains(msg, "connection reset by peer") {
				log.Fatal(err)
			}
		}
		fmt.Println()
		fmt.Println("------")
		fmt.Println()
	}
}

func request(pat reqPattern, filename string) error {
	fmt.Printf("Request pattern: %v\n\n", pat)

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	var size int
	if pat.NeedsLen() {
		stat, err := os.Stat(filename)
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}
		size = int(stat.Size())
	}

	var req *http.Request
	switch pat {
	case reqSinglePartWithLen:
		req, err = singlepartWithLen(f, size)
	case reqSinglePartWithLen_wrong:
		req, err = singlepartWithLen_wrong(f, size)
	case reqSinglePartWithoutLen:
		req, err = singlepartWithoutLen(f)
	case reqSinglePartWithBuffer:
		req, err = singlepartWithBuffer(f)
	case reqSinglePartExplicitlyChunked:
		req, err = singlepartExplicitlyChunked(f)
	case reqMultipart:
		req, err = multipartReq(f, filename)
	}
	if err != nil {
		return err
	}

	return sendReq(req)
}

func sendReq(req *http.Request) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	return nil
}

// single-part PUT request, setting ContentLength field explicitly
func singlepartWithLen(body io.Reader, len int) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPut, serverURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.ContentLength = int64(len)
	return req, nil
}

// single-part PUT request, setting Content-Length header directly (and incorrectly)
func singlepartWithLen_wrong(body io.Reader, len int) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPut, serverURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Length", strconv.Itoa(len))
	return req, nil
}

// single-part PUT request, without setting ContentLength field
func singlepartWithoutLen(body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPut, serverURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	return req, nil
}

// single-part PUT request. Copy data to bytes.Buffer first, then send it without setting ContentLength field
func singlepartWithBuffer(body io.Reader) (*http.Request, error) {
	buf := new(bytes.Buffer)
	_, _ = io.Copy(buf, body)

	req, err := http.NewRequest(http.MethodPut, serverURL, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	return req, nil
}

// single-part PUT request. Copy data to bytes.Buffer first, then send it with setting "Transfer-Encoding: chunked" explicitly
func singlepartExplicitlyChunked(body io.Reader) (*http.Request, error) {
	buf := new(bytes.Buffer)
	_, _ = io.Copy(buf, body)

	req, err := http.NewRequest(http.MethodPut, serverURL, buf)
	req.TransferEncoding = []string{"chunked"}
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	return req, nil
}

// multipart request
func multipartReq(body io.Reader, filename string) (*http.Request, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	w, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create new part: %w", err)
	}
	_, _ = io.Copy(w, body)
	_ = mw.Close()

	fmt.Println(buf.Len())
	req, err := http.NewRequest(http.MethodPost, serverURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	return req, nil
}

/* server */
func startServer() (*net.TCPListener, error) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: serverPort})
	if err != nil {
		return nil, fmt.Errorf("failed to start listening: %v", err)
	}
	return l, nil
}

func serve(l net.Listener) {
	conn, err := l.Accept()
	if err != nil {
		log.Fatal(err)
	}

	// read and log first 1KiB, then disconnect
	_, _ = io.CopyN(os.Stdout, conn, 1024)
	fmt.Println()
	conn.Close()
}
