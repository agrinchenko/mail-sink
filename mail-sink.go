// Very simple server that mimics a smtp-server, it is very forgiving
// about what you send to and tends to agree (OK) most of the time.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	listenPort   = flag.Int("p", 25, "listen port")
	listenIntf   = flag.String("i", "localhost", "listen on interface")
	heloHostname = flag.String("H", "localhost", "hostname to greet with")
	logBody      = flag.Bool("v", false, "log the mail body")
	saveAttached = flag.Bool("s", false, "save attached files to current dir")
)

type SinkServer struct {
	service  string
	listener *net.TCPListener
	Stats    *SinkStats
}

type SinkStats struct {
	AcceptedConnetions int
}

type SinkClient struct {
	conn           *textproto.Conn
	dataSent       bool
	attachmentData []string
}

func NewSinkClient(conn *textproto.Conn) *SinkClient {
	return &SinkClient{conn: conn, dataSent: false}
}

func NewSinkServer(listenIntf string, listenPort int) (*SinkServer, error) {
	s := new(SinkServer)
	stats := new(SinkStats)
	service := fmt.Sprintf("%s:%v", listenIntf, listenPort)
	tcpAddr, err := net.ResolveTCPAddr("tcp", service)
	if err != nil {
		log.Fatalln("Could not start server: ", err)
		return nil, err
	}
	s.listener, _ = net.ListenTCP("tcp", tcpAddr)
	s.Stats = stats
	return s, nil
}

func (s *SinkServer) ListenAndServe() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Fatalln("error accepting client: ", err)
			continue
		}
		s.Stats.AcceptedConnetions += 1
		go s.HandleClient(conn)
	}
}

func (s *SinkServer) HandleClient(conn net.Conn) {
	sc := NewSinkClient(textproto.NewConn(conn))
	remoteClient := conn.RemoteAddr().String()

	defer func() {
		log.Printf("Closing connection to %s\r\n", remoteClient)
		sc.conn.Close()
	}()

	// Start off greeting our client
	greet := fmt.Sprintf("%s SMTP mail-sink", *heloHostname)
	respond(sc.conn, 220, greet)

	for {
		line, err := sc.conn.ReadLine()
		if err != nil {
			log.Println("error ReadLine:", remoteClient, err.Error())
			return
		}

		// valid base64 line, do not transform it!
		logLine := line
		line = strings.ToLower(strings.TrimSpace(line))
		if !sc.dataSent {
			log.Println(remoteClient + ": " + strconv.Quote(line))
		} else if *logBody {
			log.Println(remoteClient + ": " + logLine)
		}

		// We handle quit as a special case here..
		if line == "quit" {
			respond(sc.conn, 221, "Bye")
			return
		}

		code, msg := sc.handleQuery(line, logLine)
		if code != 0 {
			respond(sc.conn, code, msg)
		}
	}
}

func (s *SinkClient) handleQuery(line string, origLine string) (code int, reply string) {
	if strings.HasPrefix(line, "data") {
		s.dataSent = true
		return 354, "End data with <CR><LF>.<CR><LF>"
	}

	if s.dataSent {
		if line == "." {
			s.dataSent = false
			if *saveAttached {
				go s.processAttachments()
			}
			return 250, "Ok, queued as 31337"
		}
		s.attachmentData = append(s.attachmentData, origLine)
		return 0, ""
	}

	return 250, "Ok"
}

func respond(conn *textproto.Conn, code int, msg string) error {
	return conn.PrintfLine("%d %s", code, msg)
}

func (s *SinkClient) processAttachments() {
	var filename string
	var inAttachment bool
	var b64Lines []string

	filenameRegex := regexp.MustCompile(`filename="([^"]+)"`)
	encodingRegex := regexp.MustCompile(`(?i)Content-Transfer-Encoding:\s*base64`)

	for _, line := range s.attachmentData {
		// Detect filename
		if matches := filenameRegex.FindStringSubmatch(line); matches != nil {
			filename = matches[1]
			continue
		}

		// Detect start of base64 content
		if encodingRegex.MatchString(line) {
			inAttachment = true
			b64Lines = nil
			continue
		}

		if inAttachment {

			if strings.TrimSpace(line) == "" {
			}
			// Base64 lines usually stop at an empty line or a boundary, we’ll be naive for now
			if strings.HasPrefix(line, "--") {
				inAttachment = false
				s.saveAttachment(filename, b64Lines)
			}
			b64Lines = append(b64Lines, strings.TrimSpace(line))
		}
	}

	// Save last attachment if not closed by boundary
	if inAttachment && filename != "" {
		s.saveAttachment(filename, b64Lines)
		log.Printf("saving last attachment b64lines %s", b64Lines)
	}
}

func (s *SinkClient) saveAttachment(filename string, b64Lines []string) {
	if filename == "" || len(b64Lines) == 0 {
		return
	}

	data, err := base64.StdEncoding.DecodeString(strings.Join(b64Lines, ""))
	if err != nil {
		log.Printf("Error decoding base64 for %s: %v", filename, err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("Error saving attachment %s: %v", filename, err)
		return
	}

	log.Printf("Saved attachment: %s", filename)
}

func main() {
	flag.Parse()
	log.Printf("Starting mail-sink on %s:%d", *listenIntf, *listenPort)
	sink, err := NewSinkServer(*listenIntf, *listenPort)
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				log.Printf("Stats: %d connetions", sink.Stats.AcceptedConnetions)
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	sink.ListenAndServe()
}
