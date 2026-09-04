package mailer

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

// fakeSMTPServer parla il minimo indispensabile del protocollo SMTP su una
// porta locale: abbastanza per verificare che Send faccia l'handshake giusto
// e consegni il messaggio che ci aspettiamo, senza toccare la rete vera.
//
// Non annuncia STARTTLS: i test lo usano con TLSModeNone, perché cifrare
// non è quello che stiamo verificando qui.
type fakeSMTPServer struct {
	Addr string

	mu       sync.Mutex
	from     string
	rcpt     []string
	data     string
	authSeen bool

	listener net.Listener
	wg       sync.WaitGroup
}

func startFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTPServer{Addr: ln.Addr().String(), listener: ln}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.handle(conn)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *fakeSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(line string) { conn.Write([]byte(line + "\r\n")) }

	write("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		// raw conserva le maiuscole/minuscole originali, cmd serve solo a
		// riconoscere il comando: gli indirizzi si estraggono da raw, o
		// "serate@example.org" tornerebbe maiuscolo e nessuna asserzione
		// del test corrisponderebbe.
		raw := strings.TrimSpace(line)
		cmd := strings.ToUpper(raw)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			// Risposta multi-riga: tutte con "-" tranne l'ultima.
			write("250-fake hello")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "AUTH"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			write("235 2.7.0 accettata")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			s.mu.Lock()
			s.from = strings.TrimSpace(raw[len("MAIL FROM:"):])
			s.mu.Unlock()
			write("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(raw[len("RCPT TO:"):]))
			s.mu.Unlock()
			write("250 ok")
		case cmd == "DATA":
			write("354 manda pure")
			var body strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if dl == ".\r\n" {
					break
				}
				body.WriteString(dl)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			write("250 accettato")
		case cmd == "QUIT":
			write("221 ciao")
			return
		case cmd == "RSET", cmd == "NOOP":
			write("250 ok")
		default:
			write("500 comando sconosciuto")
		}
	}
}

func (s *fakeSMTPServer) snapshot() (from string, rcpt []string, data string, authSeen bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.from, append([]string(nil), s.rcpt...), s.data, s.authSeen
}

// hostPort spezza l'indirizzo del listener nei due valori che Config vuole.
func (s *fakeSMTPServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(s.Addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}
