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

	"portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/smtpcrypto"
	"portal_final_backend/internal/identity/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	gomail "github.com/wneessen/go-mail"
)

// SetSMTPEncryptionKey sets the AES-256 key used for encrypting SMTP passwords.
func (s *Service) SetSMTPEncryptionKey(key []byte) {
	s.smtpEncryptionKey = key
}

// SetOrganizationSMTP stores encrypted SMTP configuration for the organization.
func (s *Service) SetOrganizationSMTP(ctx context.Context, organizationID uuid.UUID, req transport.SetSMTPRequest) error {
	if len(s.smtpEncryptionKey) == 0 {
		return apperr.Internal("SMTP encryption not configured")
	}

	var encrypted string

	if req.Password != "" {
		// New password provided — encrypt it.
		enc, err := smtpcrypto.Encrypt(req.Password, s.smtpEncryptionKey)
		if err != nil {
			return apperr.Internal("failed to encrypt SMTP password")
		}
		encrypted = enc
	} else {
		// No password — reuse the existing one if SMTP is already configured.
		settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
		if err != nil {
			return err
		}
		if settings.SMTPPassword == nil || *settings.SMTPPassword == "" {
			return apperr.Validation("password is required for initial SMTP configuration")
		}
		encrypted = *settings.SMTPPassword
	}

	_, err := s.repo.UpsertOrganizationSMTP(ctx, organizationID, repository.OrganizationSMTPUpdate{
		SMTPHost:      req.Host,
		SMTPPort:      req.Port,
		SMTPUsername:  req.Username,
		SMTPPassword:  encrypted,
		SMTPFromEmail: req.FromEmail,
		SMTPFromName:  req.FromName,
	})
	return err
}

// GetOrganizationSMTPStatus returns the SMTP configuration status (password is never returned).
func (s *Service) GetOrganizationSMTPStatus(ctx context.Context, organizationID uuid.UUID) (transport.SMTPStatusResponse, error) {
	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return transport.SMTPStatusResponse{}, err
	}

	if settings.SMTPHost == nil || *settings.SMTPHost == "" {
		return transport.SMTPStatusResponse{Configured: false}, nil
	}

	return transport.SMTPStatusResponse{
		Configured: true,
		Host:       settings.SMTPHost,
		Port:       settings.SMTPPort,
		Username:   settings.SMTPUsername,
		FromEmail:  settings.SMTPFromEmail,
		FromName:   settings.SMTPFromName,
	}, nil
}

// ClearOrganizationSMTP removes the SMTP configuration for the organization.
func (s *Service) ClearOrganizationSMTP(ctx context.Context, organizationID uuid.UUID) error {
	return s.repo.ClearOrganizationSMTP(ctx, organizationID)
}

// ── SMTP auto-detection ──

// smtpProviderEntry holds pre-configured SMTP settings for a well-known provider.
type smtpProviderEntry struct {
	provider string
	host     string
	port     int
	security string // "STARTTLS" or "SSL/TLS"
}

const (
	securityStartTLS = "STARTTLS"
	securitySSL      = "SSL/TLS"

	smtpHostGmail     = "smtp.gmail.com"
	smtpHostOffice365 = "smtp.office365.com"
	smtpHostICloud    = "smtp.mail.me.com"
	smtpHostZohoEU    = "smtp.zoho.eu"
	smtpHostGmx       = "smtp.gmx.com"

	providerMicrosoft365 = "Microsoft 365"
)

// knownProviders maps email domains to their SMTP settings.
var knownProviders = map[string]smtpProviderEntry{
	// Google / Gmail
	"gmail.com":      {provider: "Gmail", host: smtpHostGmail, port: 587, security: securityStartTLS},
	"googlemail.com": {provider: "Gmail", host: smtpHostGmail, port: 587, security: securityStartTLS},
	// Microsoft
	"outlook.com": {provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS},
	"hotmail.com": {provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS},
	"live.com":    {provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS},
	"live.nl":     {provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS},
	// Yahoo
	"yahoo.com": {provider: "Yahoo", host: "smtp.mail.yahoo.com", port: 587, security: securityStartTLS},
	"yahoo.nl":  {provider: "Yahoo", host: "smtp.mail.yahoo.com", port: 587, security: securityStartTLS},
	// Apple iCloud
	"icloud.com": {provider: "iCloud", host: smtpHostICloud, port: 587, security: securityStartTLS},
	"me.com":     {provider: "iCloud", host: smtpHostICloud, port: 587, security: securityStartTLS},
	"mac.com":    {provider: "iCloud", host: smtpHostICloud, port: 587, security: securityStartTLS},
	// Dutch ISPs
	"ziggo.nl":   {provider: "Ziggo", host: "smtp.ziggo.nl", port: 587, security: securityStartTLS},
	"kpnmail.nl": {provider: "KPN", host: "smtp.kpnmail.nl", port: 587, security: securityStartTLS},
	"xs4all.nl":  {provider: "XS4ALL", host: "smtp.xs4all.nl", port: 587, security: securityStartTLS},
	// Other
	"zoho.com":       {provider: "Zoho", host: smtpHostZohoEU, port: 587, security: securityStartTLS},
	"zoho.eu":        {provider: "Zoho", host: smtpHostZohoEU, port: 587, security: securityStartTLS},
	"gmx.com":        {provider: "GMX", host: smtpHostGmx, port: 587, security: securityStartTLS},
	"gmx.net":        {provider: "GMX", host: smtpHostGmx, port: 587, security: securityStartTLS},
	"gmx.nl":         {provider: "GMX", host: smtpHostGmx, port: 587, security: securityStartTLS},
	"protonmail.com": {provider: "Proton Mail", host: "smtp.protonmail.ch", port: 587, security: securityStartTLS},
	"proton.me":      {provider: "Proton Mail", host: "smtp.protonmail.ch", port: 587, security: securityStartTLS},
}

// mxProviderMap maps MX hostname substrings to SMTP settings (for custom-domain detection).
var mxProviderMap = []struct {
	substring string
	entry     smtpProviderEntry
}{
	{"google.com", smtpProviderEntry{provider: "Google Workspace", host: smtpHostGmail, port: 587, security: securityStartTLS}},
	{"googlemail.com", smtpProviderEntry{provider: "Google Workspace", host: smtpHostGmail, port: 587, security: securityStartTLS}},
	{"outlook.com", smtpProviderEntry{provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS}},
	{"protection.outlook.com", smtpProviderEntry{provider: providerMicrosoft365, host: smtpHostOffice365, port: 587, security: securityStartTLS}},
	{"transip.email", smtpProviderEntry{provider: "TransIP", host: "smtp.transip.email", port: 587, security: securityStartTLS}},
	{"one.com", smtpProviderEntry{provider: "One.com", host: "send.one.com", port: 587, security: securityStartTLS}},
	{"zoho.eu", smtpProviderEntry{provider: "Zoho", host: smtpHostZohoEU, port: 587, security: securityStartTLS}},
	{"zoho.com", smtpProviderEntry{provider: "Zoho", host: "smtp.zoho.com", port: 587, security: securityStartTLS}},
	{"strato.de", smtpProviderEntry{provider: "Strato", host: "smtp.strato.de", port: 587, security: securityStartTLS}},
	{"mimecast.com", smtpProviderEntry{provider: "Mimecast", host: "", port: 587, security: securityStartTLS}},
	{"pphosted.com", smtpProviderEntry{provider: "Proofpoint", host: "", port: 587, security: securityStartTLS}},
	{"messagelabs.com", smtpProviderEntry{provider: "Symantec Email Security", host: "", port: 587, security: securityStartTLS}},
}

// ── Autoconfig XML structs (Thunderbird-style) ──

type autoconfigXML struct {
	XMLName  xml.Name          `xml:"clientConfig"`
	Provider []autoconfigEmail `xml:"emailProvider"`
}

type autoconfigEmail struct {
	Outgoing []autoconfigServer `xml:"outgoingServer"`
}

type autoconfigServer struct {
	Type       string `xml:"type,attr"`
	Hostname   string `xml:"hostname"`
	Port       int    `xml:"port"`
	SocketType string `xml:"socketType"`
}

// ── Detection helpers ──

type layerResult struct {
	priority int
	resp     *transport.DetectSMTPResponse
	mxHost   string
}

func parseSMTPEmail(email string) (string, string, bool) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	return domain, email, true
}

func runParallelDetectors(ctx context.Context, domain, username string) ([3]layerResult, string) {
	ch := make(chan layerResult, 3)

	go func() {
		ch <- layerResult{priority: 1, resp: detectViaSRV(domain, username)}
	}()
	go func() {
		ch <- layerResult{priority: 2, resp: detectViaAutoconfig(ctx, domain, username)}
	}()
	go func() {
		resp, mxHost := detectViaMX(domain, username)
		ch <- layerResult{priority: 3, resp: resp, mxHost: mxHost}
	}()

	var results [3]layerResult
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

func firstDetection(results [3]layerResult) *transport.DetectSMTPResponse {
	for _, r := range results {
		if r.resp != nil {
			return r.resp
		}
	}
	return nil
}

func fallbackSMTPResponse(domain, username string) transport.DetectSMTPResponse {
	fallbackHost := "smtp." + domain
	fallbackPort := 587
	return transport.DetectSMTPResponse{
		Detected: true,
		Host:     &fallbackHost,
		Port:     &fallbackPort,
		Username: &username,
	}
}

// detectViaKnownProvider checks the static knownProviders map for an exact domain match.
func detectViaKnownProvider(domain, username string) *transport.DetectSMTPResponse {
	entry, ok := knownProviders[domain]
	if !ok {
		return nil
	}
	port := entry.port
	sec := entry.security
	return &transport.DetectSMTPResponse{
		Detected: true,
		Provider: &entry.provider,
		Host:     &entry.host,
		Port:     &port,
		Username: &username,
		Security: &sec,
	}
}

// detectViaSRV queries DNS SRV records for _submission._tcp.<domain> (RFC 6186).
func detectViaSRV(domain, username string) *transport.DetectSMTPResponse {
	_, addrs, err := net.LookupSRV("submission", "tcp", domain)
	if err != nil || len(addrs) == 0 {
		return nil
	}

	best := addrs[0]
	host := strings.TrimSuffix(best.Target, ".")
	port := int(best.Port)
	sec := securityStartTLS
	return &transport.DetectSMTPResponse{
		Detected: true,
		Host:     &host,
		Port:     &port,
		Username: &username,
		Security: &sec,
	}
}

// detectViaAutoconfig tries Thunderbird-style autoconfig XML endpoints in parallel.
func detectViaAutoconfig(ctx context.Context, domain, username string) *transport.DetectSMTPResponse {
	urls := []string{
		"https://autoconfig." + domain + "/mail/config-v1.1.xml",
		"https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml",
		"https://autoconfig.thunderbird.net/v1.1/" + domain,
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	type acResult struct {
		idx  int
		resp *transport.DetectSMTPResponse
	}

	ch := make(chan acResult, len(urls))
	for i, u := range urls {
		go func(idx int, url string) {
			ch <- acResult{idx: idx, resp: fetchAutoconfig(ctx, url, username)}
		}(i, u)
	}

	var best *transport.DetectSMTPResponse
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

// fetchAutoconfig fetches and parses a single autoconfig XML URL.
func fetchAutoconfig(ctx context.Context, url, username string) *transport.DetectSMTPResponse {
	body, err := readAutoconfigBody(ctx, url)
	if err != nil {
		return nil
	}

	return parseAutoconfigBody(body, username)
}

func readAutoconfigBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("autoconfig http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	return body, nil
}

func parseAutoconfigBody(body []byte, username string) *transport.DetectSMTPResponse {
	var cfg autoconfigXML
	if err := xml.Unmarshal(body, &cfg); err != nil {
		return nil
	}

	return findAutoconfigSMTP(cfg, username)
}

func findAutoconfigSMTP(cfg autoconfigXML, username string) *transport.DetectSMTPResponse {
	for _, provider := range cfg.Provider {
		for _, srv := range provider.Outgoing {
			if srv.Type != "smtp" || srv.Hostname == "" {
				continue
			}
			port := srv.Port
			if port == 0 {
				port = 587
			}
			sec := socketTypeToSecurity(srv.SocketType)
			return &transport.DetectSMTPResponse{
				Detected: true,
				Host:     &srv.Hostname,
				Port:     &port,
				Username: &username,
				Security: &sec,
			}
		}
	}

	return nil
}

func socketTypeToSecurity(socketType string) string {
	if strings.EqualFold(socketType, "SSL") || strings.EqualFold(socketType, securitySSL) {
		return securitySSL
	}
	return securityStartTLS
}

// detectViaMX checks MX records against the mxProviderMap.
// Returns the detection result and the raw MX hostname (for reuse in the probe step).
func detectViaMX(domain, username string) (*transport.DetectSMTPResponse, string) {
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return nil, ""
	}

	mxHost := strings.ToLower(strings.TrimSuffix(mxRecords[0].Host, "."))
	for _, m := range mxProviderMap {
		if strings.Contains(mxHost, m.substring) {
			if m.entry.host == "" {
				provider := m.entry.provider
				return &transport.DetectSMTPResponse{
					Detected: true,
					Provider: &provider,
					Username: &username,
				}, mxHost
			}
			port := m.entry.port
			sec := m.entry.security
			return &transport.DetectSMTPResponse{
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

// probeSMTPHandshake attempts a raw TCP connect + EHLO to verify the host accepts SMTP.
// Uses IPv4 only to avoid IPv6 connectivity issues.
func probeSMTPHandshake(host string, port int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	var conn net.Conn
	var err error

	if port == 465 {
		conn, err = tls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp4", addr,
			&tls.Config{InsecureSkipVerify: true}, //nolint:gosec // probe only, no data sent
		)
	} else {
		conn, err = net.DialTimeout("tcp4", addr, 2*time.Second)
	}
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "220") {
		return false
	}

	_ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	_, err = fmt.Fprintf(conn, "EHLO probe\r\n")
	if err != nil {
		return false
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "250 ") { // final 250 line (space, not dash)
			// Send QUIT.
			_, _ = fmt.Fprintf(conn, "QUIT\r\n")
			return true
		}
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}

	return false
}

// mxBaseDomain extracts the registrable base domain from an MX hostname.
// e.g. "mailserver.purelymail.com" → "purelymail.com"
func mxBaseDomain(mx string) string {
	parts := strings.Split(mx, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return mx
}

// detectViaProbe tries candidate SMTP hostnames with a real TCP handshake.
// It builds candidates from:
//  1. The email domain itself (smtp.<domain>, mail.<domain>, <domain>)
//  2. The MX record's base domain (smtp.<mx-base>, mail.<mx-base>, <mx-host>)
//
// The mxHost parameter is the MX hostname already resolved in the MX detection step,
// avoiding a duplicate DNS lookup.
func detectViaProbe(domain, username, mxHost string) *transport.DetectSMTPResponse {
	for _, host := range buildProbeCandidates(domain, mxHost) {
		if r := probeSMTPHost(host, username); r != nil {
			return r
		}
	}

	return nil
}

func buildProbeCandidates(domain, mxHost string) []string {
	// Start with domain-based candidates.
	seen := map[string]bool{}
	candidates := []string{
		"smtp." + domain,
		"mail." + domain,
		domain,
	}
	for _, c := range candidates {
		seen[c] = true
	}

	// Add MX-derived candidates (reusing the MX host from step 4).
	if mxHost == "" {
		return candidates
	}

	base := mxBaseDomain(mxHost)
	mxCandidates := []string{
		"smtp." + base,
		"mail." + base,
		mxHost, // the MX host itself might accept submission
	}
	for _, c := range mxCandidates {
		if !seen[c] {
			candidates = append(candidates, c)
			seen[c] = true
		}
	}

	return candidates
}

func probeSMTPHost(host, username string) *transport.DetectSMTPResponse {
	// Try port 587 (STARTTLS) first, then 465 (implicit TLS).
	if probeSMTPHandshake(host, 587) {
		port := 587
		sec := securityStartTLS
		return &transport.DetectSMTPResponse{
			Detected: true,
			Host:     &host,
			Port:     &port,
			Username: &username,
			Security: &sec,
		}
	}
	if probeSMTPHandshake(host, 465) {
		port := 465
		sec := securitySSL
		return &transport.DetectSMTPResponse{
			Detected: true,
			Host:     &host,
			Port:     &port,
			Username: &username,
			Security: &sec,
		}
	}

	return nil
}

// DetectSMTPSettings auto-detects SMTP parameters from an email address using a
// multi-layered pipeline: known providers → SRV records (RFC 6186) → autoconfig XML
// (Thunderbird-style) → MX-based provider matching → live SMTP handshake probe → fallback.
func (s *Service) DetectSMTPSettings(ctx context.Context, email string) transport.DetectSMTPResponse {
	domain, username, ok := parseSMTPEmail(email)
	if !ok {
		return transport.DetectSMTPResponse{Detected: false}
	}

	// 1. Known providers by exact domain match (instant, no network).
	if r := detectViaKnownProvider(domain, username); r != nil {
		return *r
	}

	if r := s.detectSMTPWithNetwork(ctx, domain, username); r != nil {
		return *r
	}

	return fallbackSMTPResponse(domain, username)
}

func (s *Service) detectSMTPWithNetwork(ctx context.Context, domain, username string) *transport.DetectSMTPResponse {
	// Apply an overall deadline so the full detection never exceeds 10 seconds.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 2-4. Run SRV, autoconfig and MX in parallel for speed.
	results, mxHost := runParallelDetectors(ctx, domain, username)

	// Return the first successful result by priority order.
	if r := firstDetection(results); r != nil {
		return r
	}

	// 5. Live SMTP handshake probe (reuses MX host from step 4).
	return detectViaProbe(domain, username, mxHost)
}

// TestOrganizationSMTP sends a test email using the stored SMTP configuration.
func (s *Service) TestOrganizationSMTP(ctx context.Context, organizationID uuid.UUID, toEmail string) error {
	if len(s.smtpEncryptionKey) == 0 {
		return apperr.Internal("SMTP encryption not configured")
	}

	settings, err := s.repo.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		return err
	}
	if settings.SMTPHost == nil || *settings.SMTPHost == "" {
		return apperr.Validation("no SMTP configuration found")
	}
	if settings.SMTPPassword == nil {
		return apperr.Validation("SMTP password missing")
	}

	password, err := smtpcrypto.Decrypt(*settings.SMTPPassword, s.smtpEncryptionKey)
	if err != nil {
		return apperr.Internal("failed to decrypt SMTP password")
	}

	fromName := "Test"
	if settings.SMTPFromName != nil {
		fromName = *settings.SMTPFromName
	}
	fromEmail := *settings.SMTPFromEmail

	msg := gomail.NewMsg()
	if err := msg.FromFormat(fromName, fromEmail); err != nil {
		return apperr.Validation(fmt.Sprintf("invalid from address: %v", err))
	}
	if err := msg.To(toEmail); err != nil {
		return apperr.Validation(fmt.Sprintf("invalid to address: %v", err))
	}
	msg.Subject("SMTP Test — Portal")
	msg.SetBodyString(gomail.TypeTextHTML, "<h2>SMTP Test Geslaagd</h2><p>Uw SMTP-configuratie werkt correct.</p>")

	port := 587
	if settings.SMTPPort != nil {
		port = *settings.SMTPPort
	}

	client, err := gomail.NewClient(*settings.SMTPHost,
		gomail.WithPort(port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(*settings.SMTPUsername),
		gomail.WithPassword(password),
		gomail.WithTLSPortPolicy(gomail.TLSOpportunistic),
		gomail.WithTimeout(10*time.Second),
		gomail.WithDialContextFunc(func(dctx context.Context, _ string, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(dctx, "tcp4", addr)
		}),
	)
	if err != nil {
		return apperr.Internal(fmt.Sprintf("failed to create SMTP client: %v", err))
	}

	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		// Provide user-friendly error messages
		if netErr, ok := err.(*net.OpError); ok {
			return apperr.Validation(fmt.Sprintf("could not connect to %s:%s — %v", *settings.SMTPHost, strconv.Itoa(port), netErr))
		}
		return apperr.Validation(fmt.Sprintf("SMTP test failed: %v", err))
	}

	return nil
}
