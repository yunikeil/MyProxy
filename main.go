package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
)

const logLevel = DEBUG

func logMessage(level int, format string, v ...interface{}) {
	if level >= logLevel {
		log.Printf(format, v...)
	}
}

func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	reader := bufio.NewReader(clientConn)

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			logMessage(INFO, "Connection closed by client before request line was read.\n")
		} else {
			logMessage(ERROR, "Failed to read request line: %v\n", err)
			sendErrorResponse(clientConn, "Failed to read request line")
		}
		return
	}

	method, url, err := parseRequestLine(requestLine)
	if err != nil {
		logMessage(ERROR, "Invalid request line: %s - %v\n", requestLine, err)
		sendErrorResponse(clientConn, "Invalid request line")
		return
	}

	if isLocalRequest(url) {
		sendNotFoundResponse(clientConn)
		logMessage(DEBUG, "Returned 404 for GET request to %s\n", url)
	} else if method == "CONNECT" {
		logMessage(DEBUG, "New HTTPS connection to %s\n", url)
		handleHTTPS(clientConn, reader, url)
	} else {
		logMessage(DEBUG, "New HTTP connection to %s\n", url)
		handleHTTP(clientConn, reader, method, url, requestLine)
	}
}

func isLocalRequest(url string) bool {
	return strings.HasPrefix(url, "/")
}

func parseRequestLine(requestLine string) (method string, url string, err error) {
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("request line has fewer than 2 parts")
	}
	return parts[0], parts[1], nil
}

func sendNotFoundResponse(conn net.Conn) {
	response := "HTTP/1.1 404 Not Found\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 23\r\n" +
		"\r\n" +
		"This is a proxy server."

	conn.Write([]byte(response))
}

func sendErrorResponse(conn net.Conn, message string) {
	response := "HTTP/1.1 400 Bad Request\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: " + strconv.Itoa(len(message)) + "\r\n" +
		"\r\n" +
		message

	conn.Write([]byte(response))
}

func handleHTTP(clientConn net.Conn, reader *bufio.Reader, method, url, requestLine string) {
	// Remove protocol prefix
	url = strings.TrimPrefix(url, "http://")

	// Split the domain:port from the path
	hostPort := url
	if idx := strings.Index(url, "/"); idx != -1 {
		hostPort = url[:idx]
	}

	// If no port is specified, default to port 80
	if !strings.Contains(hostPort, ":") {
		hostPort = net.JoinHostPort(hostPort, "80")
	}

	// Connect to the remote server
	serverConn, err := net.Dial("tcp", hostPort)
	if err != nil {
		logMessage(ERROR, "Unable to connect to remote server: %s\n", err)
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
	} else if strings.Count(hostPort, ":") != 1 {
		hostPort = "[" + hostPort + "]:443"
	}

	// Connect to the remote server
	serverConn, err := net.Dial("tcp", hostPort)
	if err != nil {
		logMessage(ERROR, "Unable to connect to remote server (s): %s\n", err)
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
	listenAddr := "0.0.0.0:8080"

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logMessage(ERROR, "Unable to listen on %s: %s\n", listenAddr, err)
	}
	defer listener.Close()

	logMessage(INFO, "TCP proxy listening on %s\n", listenAddr)

	// Accept incoming connections
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			logMessage(ERROR, "Unable to accept connection: %s\n", err)
			continue
		}

		go handleConnection(clientConn)
	}
}
