// Package notification provides event handlers for sending notifications
// (emails, SMS, push, etc.) in response to domain events.
// This module subscribes to events and inverts the dependency: domain modules
// no longer need to know about email providers or templates.
package notification

import (
	"context"
	"fmt"
	"strings"

	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
)

// Module handles all notification-related event subscriptions.
type Module struct {
	sender email.Sender
	cfg    config.NotificationConfig
	log    *logger.Logger
}

// New creates a new notification module.
func New(sender email.Sender, cfg config.NotificationConfig, log *logger.Logger) *Module {
	return &Module{
		sender: sender,
		cfg:    cfg,
		log:    log,
	}
}

// RegisterHandlers subscribes to all relevant domain events on the event bus.
func (m *Module) RegisterHandlers(bus *events.InMemoryBus) {
	// Auth domain events
	bus.Subscribe(events.UserSignedUp{}.EventName(), m)
	bus.Subscribe(events.EmailVerificationRequested{}.EventName(), m)
	bus.Subscribe(events.PasswordResetRequested{}.EventName(), m)

	// Leads domain events
	bus.Subscribe(events.VisitScheduled{}.EventName(), m)
	bus.Subscribe(events.VisitRescheduled{}.EventName(), m)

	m.log.Info("notification module registered event handlers")
}

// Handle routes events to the appropriate handler method.
func (m *Module) Handle(ctx context.Context, event events.Event) error {
	switch e := event.(type) {
	case events.UserSignedUp:
		return m.handleUserSignedUp(ctx, e)
	case events.EmailVerificationRequested:
		return m.handleEmailVerificationRequested(ctx, e)
	case events.PasswordResetRequested:
		return m.handlePasswordResetRequested(ctx, e)
	case events.VisitScheduled:
		return m.handleVisitScheduled(ctx, e)
	case events.VisitRescheduled:
		return m.handleVisitRescheduled(ctx, e)
	default:
		m.log.Warn("unhandled event type", "event", event.EventName())
		return nil
	}
}

func (m *Module) handleUserSignedUp(ctx context.Context, e events.UserSignedUp) error {
	verifyURL := m.buildURL("/verify-email", e.VerifyToken)
	if err := m.sender.SendVerificationEmail(ctx, e.Email, verifyURL); err != nil {
		m.log.Error("failed to send verification email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("verification email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handleEmailVerificationRequested(ctx context.Context, e events.EmailVerificationRequested) error {
	verifyURL := m.buildURL("/verify-email", e.VerifyToken)
	if err := m.sender.SendVerificationEmail(ctx, e.Email, verifyURL); err != nil {
		m.log.Error("failed to send verification email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("verification email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handlePasswordResetRequested(ctx context.Context, e events.PasswordResetRequested) error {
	resetURL := m.buildURL("/reset-password", e.ResetToken)
	if err := m.sender.SendPasswordResetEmail(ctx, e.Email, resetURL); err != nil {
		m.log.Error("failed to send password reset email",
			"userId", e.UserID,
			"email", e.Email,
			"error", err,
		)
		return err
	}
	m.log.Info("password reset email sent", "userId", e.UserID, "email", e.Email)
	return nil
}

func (m *Module) handleVisitScheduled(ctx context.Context, e events.VisitScheduled) error {
	if !e.SendInvite {
		return nil
	}

	if e.ConsumerEmail == nil || *e.ConsumerEmail == "" {
		m.log.Debug("skipping visit invite email - no consumer email", "leadId", e.LeadID)
		return nil
	}

	consumerName := e.ConsumerFirstName + " " + e.ConsumerLastName
	scheduledDateStr := e.ScheduledDate.Format("Monday, January 2, 2006 at 15:04")
	address := fmt.Sprintf("%s %s, %s %s",
		e.AddressStreet, e.AddressHouseNumber, e.AddressZipCode, e.AddressCity)

	if err := m.sender.SendVisitInviteEmail(ctx, *e.ConsumerEmail, consumerName, scheduledDateStr, address); err != nil {
		m.log.Error("failed to send visit invite email",
			"leadId", e.LeadID,
			"email", *e.ConsumerEmail,
			"error", err,
		)
		return err
	}
	m.log.Info("visit invite email sent", "leadId", e.LeadID, "email", *e.ConsumerEmail)
	return nil
}

func (m *Module) handleVisitRescheduled(ctx context.Context, e events.VisitRescheduled) error {
	if !e.SendInvite {
		return nil
	}

	if e.ConsumerEmail == nil || *e.ConsumerEmail == "" {
		m.log.Debug("skipping visit invite email - no consumer email", "leadId", e.LeadID)
		return nil
	}

	consumerName := e.ConsumerFirstName + " " + e.ConsumerLastName
	scheduledDateStr := e.NewScheduledDate.Format("Monday, January 2, 2006 at 15:04")
	address := fmt.Sprintf("%s %s, %s %s",
		e.AddressStreet, e.AddressHouseNumber, e.AddressZipCode, e.AddressCity)

	if err := m.sender.SendVisitInviteEmail(ctx, *e.ConsumerEmail, consumerName, scheduledDateStr, address); err != nil {
		m.log.Error("failed to send visit invite email",
			"leadId", e.LeadID,
			"email", *e.ConsumerEmail,
			"error", err,
		)
		return err
	}
	m.log.Info("visit reschedule invite email sent", "leadId", e.LeadID, "email", *e.ConsumerEmail)
	return nil
}

func (m *Module) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetAppBaseURL(), "/")
	return base + path + "?token=" + tokenValue
}
