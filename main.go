package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/sdk"
)

var (
	l  net.Listener
	mu sync.Mutex
)

func main() {
	h := sdk.NewHandler(`{"Implements": ["MetricsCollector"]}`)
	handlers(&h)
	if err := h.ServeUnix("metrics", 0); err != nil {
		panic(err)
	}
}

func accept(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			logrus.Error(err)
			continue
		}

		proxyConn, err := net.Dial("unix", "/run/docker/metrics.sock")
		if err != nil {
			logrus.Error(err)
			continue
		}

		go func() {
			io.Copy(proxyConn, conn)
			conn.(*net.TCPConn).CloseRead()
			proxyConn.(*net.UnixConn).CloseWrite()
		}()
		go func() {
			io.Copy(conn, proxyConn)
			proxyConn.(*net.UnixConn).CloseRead()
			conn.(*net.TCPConn).CloseWrite()
		}()
	}
}

func handlers(h *sdk.Handler) {
	h.HandleFunc("/MetricsCollector.StartMetrics", func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer func() {
			var res struct{ Err string }
			if err != nil {
				res.Err = err.Error()
			}
			json.NewEncoder(w).Encode(&res)
		}()

		mu.Lock()
		defer mu.Unlock()

		if l == nil {
			l, err = net.Listen("tcp", "127.0.0.1:19393")
			if err != nil {
				return
			}
			go accept(l)
		}
	})

	h.HandleFunc("/MetricsCollector.StopMetrics", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		l.Close()
		l = nil
		mu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{})
	})
}
