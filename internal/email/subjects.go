// Package email provides constants and logic for outgoing communications.
package email

// Subject constants for the email domain.
// These are used as templates for fmt.Sprintf where format specifiers (%s) are present.
const (
	// Authentication
	subjectVerification  = "Verifieer uw e-mailadres"
	subjectPasswordReset = "Wachtwoord opnieuw instellen"

	// Invitations
	subjectVisitInvite           = "Uw bezoek is ingepland"
	subjectOrganizationInviteFmt = "U bent uitgenodigd voor %s"
	subjectPartnerInviteFmt      = "Nieuw werkaanbod van %s"

	// Quotes & Proposals
	subjectQuoteProposalFmt         = "Offerte %s van %s"
	subjectQuoteAcceptedFmt         = "Offerte %s geaccepteerd"
	subjectQuoteAcceptedThankYouFmt = "Bedankt voor uw akkoord — offerte %s"

	// Partner Offers
	subjectPartnerOfferAcceptedFmt          = "Werkaanbod geaccepteerd door %s"
	subjectPartnerOfferAcceptedConfirmation = "Bevestiging: uw acceptatie is ontvangen"
	subjectPartnerOfferRejectedFmt          = "Werkaanbod afgewezen door %s"
)
