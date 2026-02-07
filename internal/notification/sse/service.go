// Package sse provides Server-Sent Events support for real-time notifications.
package sse

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EventType represents different types of SSE events
type EventType string

const (
	EventAnalysisComplete      EventType = "analysis_complete"
	EventPhotoAnalysisComplete EventType = "photo_analysis_complete"
	EventLeadUpdated           EventType = "lead_updated"

	// Quote events (pushed to agents watching a quote)
	EventQuoteSent        EventType = "quote_sent"
	EventQuoteViewed      EventType = "quote_viewed"
	EventQuoteItemToggled EventType = "quote_item_toggled"
	EventQuoteAnnotated   EventType = "quote_annotated"
	EventQuoteAccepted    EventType = "quote_accepted"
	EventQuoteRejected    EventType = "quote_rejected"

	// Appointment events (pushed to org members)
	EventAppointmentCreated       EventType = "appointment_created"
	EventAppointmentUpdated       EventType = "appointment_updated"
	EventAppointmentStatusChanged EventType = "appointment_status_changed"
)

// Event represents an SSE event payload
type Event struct {
	Type      EventType   `json:"type"`
	LeadID    uuid.UUID   `json:"leadId,omitempty"`
	ServiceID uuid.UUID   `json:"serviceId,omitempty"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// client represents a connected SSE client
type client struct {
	userID uuid.UUID
	orgID  uuid.UUID
	events chan Event
}

// quoteClient represents a public (unauthenticated) SSE viewer on a quote page.
type quoteClient struct {
	quoteID uuid.UUID
	events  chan Event
}

// Service manages SSE connections and event broadcasting
type Service struct {
	mu           sync.RWMutex
	clients      map[uuid.UUID][]*client      // userID -> clients
	orgMap       map[uuid.UUID][]uuid.UUID    // orgID -> userIDs
	quoteClients map[uuid.UUID][]*quoteClient // quoteID -> public viewers
}

// New creates a new SSE service
func New() *Service {
	return &Service{
		clients:      make(map[uuid.UUID][]*client),
		orgMap:       make(map[uuid.UUID][]uuid.UUID),
		quoteClients: make(map[uuid.UUID][]*quoteClient),
	}
}

// addClient registers a new client connection
func (s *Service) addClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[c.userID] = append(s.clients[c.userID], c)

	// Track org membership
	if c.orgID != uuid.Nil {
		s.orgMap[c.orgID] = append(s.orgMap[c.orgID], c.userID)
	}
}

// removeClient unregisters a client connection
func (s *Service) removeClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clients := s.clients[c.userID]
	for i, cl := range clients {
		if cl == c {
			s.clients[c.userID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(s.clients[c.userID]) == 0 {
		delete(s.clients, c.userID)
	}

	close(c.events)
}

// Publish sends an event to a specific user
func (s *Service) Publish(userID uuid.UUID, event Event) {
	s.mu.RLock()
	clients := s.clients[userID]
	s.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.events <- event:
		default:
			log.Printf("SSE: Event buffer full for user %s", userID)
		}
	}

	log.Printf("SSE: Published event %s to user %s (%d clients)", event.Type, userID, len(clients))
}

// PublishToOrganization broadcasts an event to all org members
func (s *Service) PublishToOrganization(orgID uuid.UUID, event Event) {
	s.mu.RLock()
	userIDs := make([]uuid.UUID, len(s.orgMap[orgID]))
	copy(userIDs, s.orgMap[orgID])
	s.mu.RUnlock()

	// Deduplicate and send
	seen := make(map[uuid.UUID]bool)
	for _, userID := range userIDs {
		if seen[userID] {
			continue
		}
		seen[userID] = true
		s.Publish(userID, event)
	}

	log.Printf("SSE: Published event %s to org %s (%d RAC_users)", event.Type, orgID, len(seen))
}

// PublishQuoteEvent is a convenience wrapper that broadcasts a quote-related
// event to every connected agent in the organisation AND to any public viewers.
func (s *Service) PublishQuoteEvent(orgID uuid.UUID, eventType EventType, quoteID uuid.UUID, data interface{}) {
	evt := Event{
		Type: eventType,
		Data: map[string]interface{}{
			"quoteId": quoteID,
			"payload": data,
		},
	}
	s.PublishToOrganization(orgID, evt)
	s.PublishToQuote(quoteID, evt)
}

// PublishToQuote sends an event to all public viewers of a quote.
func (s *Service) PublishToQuote(quoteID uuid.UUID, event Event) {
	s.mu.RLock()
	viewers := make([]*quoteClient, len(s.quoteClients[quoteID]))
	copy(viewers, s.quoteClients[quoteID])
	s.mu.RUnlock()

	for _, v := range viewers {
		select {
		case v.events <- event:
		default:
			log.Printf("SSE: Quote event buffer full for quote %s", quoteID)
		}
	}

	if len(viewers) > 0 {
		log.Printf("SSE: Published event %s to quote %s (%d public viewers)", event.Type, quoteID, len(viewers))
	}
}

// setSSEHeaders configures the standard SSE response headers.
func setSSEHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

// deregisterQuoteClient removes a public viewer for a quote.
func (s *Service) deregisterQuoteClient(quoteID uuid.UUID, qc *quoteClient) {
	s.mu.Lock()
	defer s.mu.Unlock()

	viewers := s.quoteClients[quoteID]
	for i, v := range viewers {
		if v == qc {
			s.quoteClients[quoteID] = append(viewers[:i], viewers[i+1:]...)
			break
		}
	}
	if len(s.quoteClients[quoteID]) == 0 {
		delete(s.quoteClients, quoteID)
	}
	close(qc.events)
}

// streamEvents writes SSE events from the channel until the client disconnects or
// the channel is closed. It is used by both the authenticated and public handlers.
func streamEvents(c *gin.Context, events <-chan Event, disconnectLog string) {
	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			log.Print(disconnectLog)
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			c.SSEvent(string(event.Type), string(data))
			c.Writer.Flush()
		}
	}
}

// PublicQuoteHandler returns a Gin handler for unauthenticated SSE connections
// on a specific quote (used by the public proposal page).
func (s *Service) PublicQuoteHandler(resolveQuoteID func(token string) (uuid.UUID, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
			return
		}

		quoteID, err := resolveQuoteID(token)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "quote not found"})
			return
		}

		setSSEHeaders(c)

		qc := &quoteClient{
			quoteID: quoteID,
			events:  make(chan Event, 32),
		}

		// Register
		s.mu.Lock()
		s.quoteClients[quoteID] = append(s.quoteClients[quoteID], qc)
		s.mu.Unlock()

		// Deregister on disconnect
		defer s.deregisterQuoteClient(quoteID, qc)

		// Connected signal
		c.SSEvent("connected", gin.H{"quoteId": quoteID})
		c.Writer.Flush()

		log.Printf("SSE: Public viewer connected for quote %s", quoteID)

		streamEvents(c, qc.events, fmt.Sprintf("SSE: Public viewer disconnected for quote %s", quoteID))
	}
}

// Handler returns a Gin handler for SSE connections
func (s *Service) Handler(getUserID func(*gin.Context) (uuid.UUID, bool), getOrgID func(*gin.Context) (uuid.UUID, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := getUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		orgID, _ := getOrgID(c)

		setSSEHeaders(c)

		// Create client
		cl := &client{
			userID: userID,
			orgID:  orgID,
			events: make(chan Event, 32),
		}
		s.addClient(cl)
		defer s.removeClient(cl)

		// Send connection event
		c.SSEvent("connected", gin.H{"userId": userID, "orgId": orgID})
		c.Writer.Flush()

		log.Printf("SSE: Client connected - user %s, org %s", userID, orgID)

		streamEvents(c, cl.events, fmt.Sprintf("SSE: Client disconnected - user %s", userID))
	}
}

// Close shuts down the SSE service
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, clients := range s.clients {
		for _, c := range clients {
			close(c.events)
		}
	}
	for _, viewers := range s.quoteClients {
		for _, v := range viewers {
			close(v.events)
		}
	}
	s.clients = make(map[uuid.UUID][]*client)
	s.orgMap = make(map[uuid.UUID][]uuid.UUID)
	s.quoteClients = make(map[uuid.UUID][]*quoteClient)
}
