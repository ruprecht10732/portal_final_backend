// Package sse provides Server-Sent Events support for real-time notifications.
package sse

import (
	"encoding/json"
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

// Service manages SSE connections and event broadcasting
type Service struct {
	mu      sync.RWMutex
	clients map[uuid.UUID][]*client   // userID -> clients
	orgMap  map[uuid.UUID][]uuid.UUID // orgID -> userIDs
}

// New creates a new SSE service
func New() *Service {
	return &Service{
		clients: make(map[uuid.UUID][]*client),
		orgMap:  make(map[uuid.UUID][]uuid.UUID),
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
// event to every connected agent in the organisation.
func (s *Service) PublishQuoteEvent(orgID uuid.UUID, eventType EventType, quoteID uuid.UUID, data interface{}) {
	s.PublishToOrganization(orgID, Event{
		Type: eventType,
		Data: map[string]interface{}{
			"quoteId": quoteID,
			"payload": data,
		},
	})
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

		// Set SSE headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("X-Accel-Buffering", "no")

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

		// Listen for events
		clientGone := c.Request.Context().Done()
		for {
			select {
			case <-clientGone:
				log.Printf("SSE: Client disconnected - user %s", userID)
				return
			case event, ok := <-cl.events:
				if !ok {
					return
				}
				data, _ := json.Marshal(event)
				c.SSEvent(string(event.Type), string(data))
				c.Writer.Flush()
			}
		}
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
	s.clients = make(map[uuid.UUID][]*client)
	s.orgMap = make(map[uuid.UUID][]uuid.UUID)
}
