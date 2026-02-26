package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/imap/transport"
)

type imapProviderEntry struct {
	provider string
	host     string
	port     int
	security string // "STARTTLS" or "SSL/TLS"
}

const (
	imapSecurityStartTLS = "STARTTLS"
	imapSecuritySSL      = "SSL/TLS"
)

var knownIMAPProviders = map[string]imapProviderEntry{
	"gmail.com":      {provider: "Gmail", host: "imap.gmail.com", port: 993, security: imapSecuritySSL},
	"googlemail.com": {provider: "Gmail", host: "imap.gmail.com", port: 993, security: imapSecuritySSL},
	"outlook.com":    {provider: "Microsoft 365", host: "outlook.office365.com", port: 993, security: imapSecuritySSL},
	"hotmail.com":    {provider: "Microsoft 365", host: "outlook.office365.com", port: 993, security: imapSecuritySSL},
	"live.com":       {provider: "Microsoft 365", host: "outlook.office365.com", port: 993, security: imapSecuritySSL},
	"yahoo.com":      {provider: "Yahoo", host: "imap.mail.yahoo.com", port: 993, security: imapSecuritySSL},
	"yahoo.nl":       {provider: "Yahoo", host: "imap.mail.yahoo.com", port: 993, security: imapSecuritySSL},
	"icloud.com":     {provider: "iCloud", host: "imap.mail.me.com", port: 993, security: imapSecuritySSL},
	"me.com":         {provider: "iCloud", host: "imap.mail.me.com", port: 993, security: imapSecuritySSL},
	"mac.com":        {provider: "iCloud", host: "imap.mail.me.com", port: 993, security: imapSecuritySSL},
	"zoho.com":       {provider: "Zoho", host: "imap.zoho.com", port: 993, security: imapSecuritySSL},
	"zoho.eu":        {provider: "Zoho", host: "imap.zoho.eu", port: 993, security: imapSecuritySSL},
	"gmx.com":        {provider: "GMX", host: "imap.gmx.com", port: 993, security: imapSecuritySSL},
	"gmx.net":        {provider: "GMX", host: "imap.gmx.net", port: 993, security: imapSecuritySSL},
	"gmx.nl":         {provider: "GMX", host: "imap.gmx.net", port: 993, security: imapSecuritySSL},
	"protonmail.com": {provider: "Proton Mail", host: "imap.protonmail.ch", port: 993, security: imapSecuritySSL},
	"proton.me":      {provider: "Proton Mail", host: "imap.protonmail.ch", port: 993, security: imapSecuritySSL},
}

var imapMXProviderMap = []struct {
	substring string
	entry     imapProviderEntry
}{
	{"google.com", imapProviderEntry{provider: "Google Workspace", host: "imap.gmail.com", port: 993, security: imapSecuritySSL}},
	{"googlemail.com", imapProviderEntry{provider: "Google Workspace", host: "imap.gmail.com", port: 993, security: imapSecuritySSL}},
	{"outlook.com", imapProviderEntry{provider: "Microsoft 365", host: "outlook.office365.com", port: 993, security: imapSecuritySSL}},
	{"protection.outlook.com", imapProviderEntry{provider: "Microsoft 365", host: "outlook.office365.com", port: 993, security: imapSecuritySSL}},
	{"zoho.eu", imapProviderEntry{provider: "Zoho", host: "imap.zoho.eu", port: 993, security: imapSecuritySSL}},
	{"zoho.com", imapProviderEntry{provider: "Zoho", host: "imap.zoho.com", port: 993, security: imapSecuritySSL}},
	{"one.com", imapProviderEntry{provider: "One.com", host: "imap.one.com", port: 993, security: imapSecuritySSL}},
	{"transip.email", imapProviderEntry{provider: "TransIP", host: "imap.transip.email", port: 993, security: imapSecuritySSL}},
}

type imapAutoconfigXML struct {
	XMLName  xml.Name               `xml:"clientConfig"`
	Provider []imapAutoconfigEmail  `xml:"emailProvider"`
}

type imapAutoconfigEmail struct {
	Incoming []imapAutoconfigServer `xml:"incomingServer"`
}

type imapAutoconfigServer struct {
	Type       string `xml:"type,attr"`
	Hostname   string `xml:"hostname"`
	Port       int    `xml:"port"`
	SocketType string `xml:"socketType"`
}

type imapLayerResult struct {
	priority int
	resp     *transport.DetectAccountResponse
	mxHost   string
}

func parseIMAPEmail(email string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	return domain, strings.TrimSpace(email), true
}

func detectIMAPViaKnownProvider(domain, username string) *transport.DetectAccountResponse {
	entry, ok := knownIMAPProviders[domain]
	if !ok {
		return nil
	}
	port := entry.port
	sec := entry.security
	return &transport.DetectAccountResponse{
		Detected: true,
		Provider: &entry.provider,
		Host:     &entry.host,
		Port:     &port,
		Username: &username,
		Security: &sec,
	}
}

func detectIMAPViaSRV(domain, username string) *transport.DetectAccountResponse {
	if r := detectSingleIMAPSRV(domain, username, "imaps", imapSecuritySSL); r != nil {
		return r
	}
	return detectSingleIMAPSRV(domain, username, "imap", imapSecurityStartTLS)
}

func detectSingleIMAPSRV(domain, username, serviceName, security string) *transport.DetectAccountResponse {
	_, addrs, err := net.LookupSRV(serviceName, "tcp", domain)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	best := addrs[0]
	host := strings.TrimSuffix(best.Target, ".")
	port := int(best.Port)
	sec := security
	return &transport.DetectAccountResponse{
		Detected: true,
		Host:     &host,
		Port:     &port,
		Username: &username,
		Security: &sec,
	}
}

func detectIMAPViaAutoconfig(ctx context.Context, domain, username string) *transport.DetectAccountResponse {
	urls := []string{
		"https://autoconfig." + domain + "/mail/config-v1.1.xml",
		"https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml",
		"https://autoconfig.thunderbird.net/v1.1/" + domain,
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	type acResult struct {
		idx  int
		resp *transport.DetectAccountResponse
	}
	ch := make(chan acResult, len(urls))
	for i, u := range urls {
		go func(idx int, url string) {
			ch <- acResult{idx: idx, resp: fetchIMAPAutoconfig(ctx, url, username)}
		}(i, u)
	}

	var best *transport.DetectAccountResponse
	bestIdx := len(urls)
	for range urls {
		r := <-ch
		if r.resp != nil && r.idx < bestIdx {
			best = r.resp
			bestIdx = r.idx
		}
	}
	return best
}

func fetchIMAPAutoconfig(ctx context.Context, url, username string) *transport.DetectAccountResponse {
	body, err := readIMAPAutoconfigBody(ctx, url)
	if err != nil {
		return nil
	}
	var cfg imapAutoconfigXML
	if err := xml.Unmarshal(body, &cfg); err != nil {
		return nil
	}
	for _, provider := range cfg.Provider {
		for _, srv := range provider.Incoming {
			if srv.Type != "imap" || strings.TrimSpace(srv.Hostname) == "" {
				continue
			}
			host := strings.TrimSpace(srv.Hostname)
			port := srv.Port
			if port == 0 {
				port = 993
			}
			sec := imapSocketTypeToSecurity(srv.SocketType, port)
			return &transport.DetectAccountResponse{
				Detected: true,
				Host:     &host,
				Port:     &port,
				Username: &username,
				Security: &sec,
			}
		}
	}
	return nil
}

func readIMAPAutoconfigBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("autoconfig http status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64*1024))
}

func imapSocketTypeToSecurity(socketType string, port int) string {
	if strings.EqualFold(socketType, "SSL") || strings.EqualFold(socketType, imapSecuritySSL) || port == 993 {
		return imapSecuritySSL
	}
	return imapSecurityStartTLS
}

func detectIMAPViaMX(domain, username string) (*transport.DetectAccountResponse, string) {
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return nil, ""
	}
	mxHost := strings.ToLower(strings.TrimSuffix(mxRecords[0].Host, "."))
	for _, m := range imapMXProviderMap {
		if strings.Contains(mxHost, m.substring) {
			port := m.entry.port
			sec := m.entry.security
			return &transport.DetectAccountResponse{
				Detected: true,
				Provider: &m.entry.provider,
				Host:     &m.entry.host,
				Port:     &port,
				Username: &username,
				Security: &sec,
			}, mxHost
		}
	}
	return nil, mxHost
}

func detectIMAPViaProbe(domain, username, mxHost string) *transport.DetectAccountResponse {
	for _, host := range buildIMAPProbeCandidates(domain, mxHost) {
		if r := probeIMAPHost(host, username); r != nil {
			return r
		}
	}
	return nil
}

func buildIMAPProbeCandidates(domain, mxHost string) []string {
	seen := map[string]bool{}
	candidates := []string{
		"imap." + domain,
		"mail." + domain,
		domain,
	}
	for _, c := range candidates {
		seen[c] = true
	}
	if mxHost == "" {
		return candidates
	}
	base := mxBaseDomain(mxHost)
	mxCandidates := []string{
		"imap." + base,
		"mail." + base,
		mxHost,
	}
	for _, c := range mxCandidates {
		if !seen[c] {
			candidates = append(candidates, c)
			seen[c] = true
		}
	}
	return candidates
}

func probeIMAPHost(host, username string) *transport.DetectAccountResponse {
	if probeIMAPHandshake(host, 993, true) {
		port := 993
		sec := imapSecuritySSL
		return &transport.DetectAccountResponse{
			Detected: true,
			Host:     &host,
			Port:     &port,
			Username: &username,
			Security: &sec,
		}
	}
	if probeIMAPHandshake(host, 143, false) {
		port := 143
		sec := imapSecurityStartTLS
		return &transport.DetectAccountResponse{
			Detected: true,
			Host:     &host,
			Port:     &port,
			Username: &username,
			Security: &sec,
		}
	}
	return nil
}

func probeIMAPHandshake(host string, port int, useTLS bool) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp4",
			addr,
			&tls.Config{InsecureSkipVerify: true}, //nolint:gosec // probe only
		)
	} else {
		conn, err = net.DialTimeout("tcp4", addr, 2*time.Second)
	}
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return false
	}
	greeting := scanner.Text()
	if !strings.HasPrefix(strings.ToUpper(greeting), "* OK") {
		return false
	}

	_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if _, err := fmt.Fprintf(conn, "a1 CAPABILITY\r\n"); err != nil {
		return false
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for scanner.Scan() {
		line := strings.ToUpper(scanner.Text())
		if strings.HasPrefix(line, "A1 OK") {
			_, _ = fmt.Fprintf(conn, "a2 LOGOUT\r\n")
			return true
		}
		if strings.HasPrefix(line, "A1 NO") || strings.HasPrefix(line, "A1 BAD") {
			return false
		}
	}
	return false
}

func runParallelIMAPDetectors(ctx context.Context, domain, username string) ([3]imapLayerResult, string) {
	ch := make(chan imapLayerResult, 3)
	go func() {
		ch <- imapLayerResult{priority: 1, resp: detectIMAPViaSRV(domain, username)}
	}()
	go func() {
		ch <- imapLayerResult{priority: 2, resp: detectIMAPViaAutoconfig(ctx, domain, username)}
	}()
	go func() {
		resp, mxHost := detectIMAPViaMX(domain, username)
		ch <- imapLayerResult{priority: 3, resp: resp, mxHost: mxHost}
	}()

	var results [3]imapLayerResult
	var mxHost string
	for i := 0; i < 3; i++ {
		r := <-ch
		results[r.priority-1] = r
		if r.mxHost != "" {
			mxHost = r.mxHost
		}
	}
	return results, mxHost
}

func firstIMAPDetection(results [3]imapLayerResult) *transport.DetectAccountResponse {
	for _, r := range results {
		if r.resp != nil {
			return r.resp
		}
	}
	return nil
}

func fallbackIMAPResponse(domain, username string) transport.DetectAccountResponse {
	host := "imap." + domain
	port := 993
	sec := imapSecuritySSL
	return transport.DetectAccountResponse{
		Detected: true,
		Host:     &host,
		Port:     &port,
		Username: &username,
		Security: &sec,
	}
}

func (s *Service) DetectAccountSettings(ctx context.Context, email string) transport.DetectAccountResponse {
	domain, username, ok := parseIMAPEmail(email)
	if !ok {
		return transport.DetectAccountResponse{Detected: false}
	}
	if r := detectIMAPViaKnownProvider(domain, username); r != nil {
		return *r
	}
	if r := s.detectIMAPWithNetwork(ctx, domain, username); r != nil {
		return *r
	}
	return fallbackIMAPResponse(domain, username)
}

func (s *Service) detectIMAPWithNetwork(ctx context.Context, domain, username string) *transport.DetectAccountResponse {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	results, mxHost := runParallelIMAPDetectors(ctx, domain, username)
	if r := firstIMAPDetection(results); r != nil {
		return r
	}
	return detectIMAPViaProbe(domain, username, mxHost)
}

func mxBaseDomain(mx string) string {
	parts := strings.Split(mx, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return mx
}
