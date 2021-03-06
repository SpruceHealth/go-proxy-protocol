package proxyproto

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListener(t *testing.T) {
	ln, err := Listen("tcp", "127.0.0.1:7791")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ch := make(chan string, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Fatal(err)
				return
			}
			ch <- conn.RemoteAddr().String()
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			return
		}
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:7791")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	src := "127.0.0.1:111"
	h, err := HeaderFromTCPAddr(src, "127.0.0.2:222")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.WriteV1(conn); err != nil {
		t.Fatal(err)
	}
	select {
	case addr := <-ch:
		if addr != src {
			t.Fatalf("Expected %s, got %s", src, addr)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out")
	}
}

func TestEOF(t *testing.T) {
	// Make sure a bad or missing header (EOF) does not cause Accept to error.
	ln, err := Listen("tcp", "127.0.0.1:7791")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ch := make(chan string, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				ch <- fmt.Sprintf("Accept returned unexpected error: %s", err)
				return
			}
			defer conn.Close()
			b := make([]byte, 1)
			if _, err := conn.Read(b); err == nil {
				ch <- "Read should have returned an error"
				return
			}
			ch <- ""
		}
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:7791")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	select {
	case e := <-ch:
		if e != "" {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out")
	}
}

func testListenerHTTP(t *testing.T, useTLS bool) {
	headerName := "X-RemoteAddr"
	serv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set(headerName, req.RemoteAddr)
		w.Write([]byte("HELLO"))
	}))

	ln, err := Listen("tcp", "127.0.0.1:7792")
	if err != nil {
		t.Fatal(err)
	}
	serv.Listener = ln
	if useTLS {
		serv.StartTLS()
	} else {
		serv.Start()
	}
	defer serv.Close()

	src := "1.2.3.4:1234"

	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			conn, err := net.Dial(network, addr)
			if err != nil {
				return nil, err
			}
			h, err := HeaderFromTCPAddr(src, "127.0.0.2:222")
			if err != nil {
				return nil, err
			}
			if _, err := h.WriteV1(conn); err != nil {
				return nil, err
			}
			return conn, err
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	cli := &http.Client{
		Transport: transport,
	}
	for i := 0; i < 8; i++ {
		res, err := cli.Get(serv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("Bad StatusCode %d", res.StatusCode)
		}
		if ra := res.Header.Get(headerName); ra != src {
			t.Fatalf("Expected %s, got %s", src, ra)
		}
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "HELLO" {
			t.Fatalf("Expected 'HELLO' got '%s'", string(b))
		}
		res.Body.Close()
	}
}

func TestListenerHTTP(t *testing.T) {
	testListenerHTTP(t, false)
}

func TestListenerHTTPS(t *testing.T) {
	testListenerHTTP(t, true)
}
