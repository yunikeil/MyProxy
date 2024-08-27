package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"strings"
)

func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Read the first line from the client to determine the request type
	reader := bufio.NewReader(clientConn)
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Error reading request line: %s\n", err)
		return
	}

	// Parse the request line
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		log.Printf("Invalid request line: %s\n", requestLine)
		return
	}

	method := parts[0]
	url := parts[1]

	if method == "CONNECT" {
		// Handle HTTPS connection
		log.Printf("New https connection to %s\n", url)
		handleHTTPS(clientConn, reader, url)
	} else {
		// Handle HTTP connection
		log.Printf("New http connection to %s\n", url)
		handleHTTP(clientConn, reader, method, url, requestLine)
	}
}

func handleHTTP(clientConn net.Conn, reader *bufio.Reader, method, url, requestLine string) {
	// Remove protocol prefix
	url = strings.TrimPrefix(url, "http://")

	log.Printf("url: %s\n", url)

	// Split the domain:port from the path
	hostPort := url
	if idx := strings.Index(url, "/"); idx != -1 {
		hostPort = url[:idx]
	}
	log.Printf("hostPort: %s\n", hostPort)

	// If no port is specified, default to port 80
	if !strings.Contains(hostPort, ":") {
		hostPort = net.JoinHostPort(hostPort, "80")
	}

	log.Printf("new hostPort: %s\n", hostPort)

	// Connect to the remote server
	serverConn, err := net.Dial("tcp", hostPort)
	if err != nil {
		log.Printf("Unable to connect to remote server: %s\n", err)
		return
	}
	defer serverConn.Close()

	// Forward the original request line and any buffered data to the remote server
	serverConn.Write([]byte(requestLine))
	go io.Copy(serverConn, reader)

	// Forward data between client and server
	go io.Copy(serverConn, clientConn)
	io.Copy(clientConn, serverConn)
}

func handleHTTPS(clientConn net.Conn, reader *bufio.Reader, url string) {
	// CONNECT method usually specifies the host:port in the URL
	hostPort := url

	// If no port is specified, default to port 443 for HTTPS
	if !strings.Contains(hostPort, ":") {
		hostPort = net.JoinHostPort(hostPort, "443")
	}

	// Connect to the remote server
	serverConn, err := net.Dial("tcp", hostPort)
	if err != nil {
		log.Printf("Unable to connect to remote server: %s\n", err)
		return
	}
	defer serverConn.Close()

	// Respond to the client indicating the tunnel is established
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Forward data between client and server (tunneling)
	go io.Copy(serverConn, clientConn)
	io.Copy(clientConn, serverConn)
}

func main() {
	// Listen on a local port
	listenAddr := "127.0.0.1:8080"

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Unable to listen on %s: %s\n", listenAddr, err)
	}
	defer listener.Close()

	log.Printf("TCP proxy listening on %s\n", listenAddr)

	// Accept incoming connections
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Unable to accept connection: %s\n", err)
			continue
		}

		go handleConnection(clientConn)
	}
}
