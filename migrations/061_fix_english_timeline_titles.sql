-- +goose Up
-- Fix legacy English timeline titles → Dutch (ILIKE for case-insensitive match)
UPDATE lead_timeline_events SET title = 'Fase bijgewerkt'               WHERE title ILIKE 'Stage Updated';
UPDATE lead_timeline_events SET title = 'Gatekeeper analyse voltooid'   WHERE title ILIKE 'Gatekeeper Analysis Complete';
UPDATE lead_timeline_events SET title = 'Leadscore bijgewerkt'          WHERE title ILIKE 'Lead Score Updated';
UPDATE lead_timeline_events SET title = 'Leadgegevens bijgewerkt'       WHERE title ILIKE 'Lead Data Updated';
UPDATE lead_timeline_events SET title = 'Partnerzoekactie'              WHERE title ILIKE 'Partner search';
UPDATE lead_timeline_events SET title = 'Schatting opgeslagen'          WHERE title ILIKE 'Estimation saved';
UPDATE lead_timeline_events SET title = 'Gesprek geregistreerd'         WHERE title ILIKE 'Call Logged';
UPDATE lead_timeline_events SET title = 'Notitie toegevoegd'            WHERE title ILIKE 'Note Added';
UPDATE lead_timeline_events SET title = 'Handmatige interventie vereist' WHERE title ILIKE 'Manual intervention required';

-- Also fix English summaries on the orchestrator alert events
UPDATE lead_timeline_events
   SET summary = 'Geautomatiseerde verwerking vereist menselijke beoordeling'
 WHERE event_type = 'alert'
   AND summary ILIKE 'Automated processing requires human review';

-- +goose Down
UPDATE lead_timeline_events SET title = 'Stage Updated' WHERE title = 'Fase bijgewerkt';
UPDATE lead_timeline_events SET title = 'Gatekeeper Analysis Complete' WHERE title = 'Gatekeeper analyse voltooid';
UPDATE lead_timeline_events SET title = 'Lead Score Updated' WHERE title = 'Leadscore bijgewerkt';
UPDATE lead_timeline_events SET title = 'Lead Data Updated' WHERE title = 'Leadgegevens bijgewerkt';
UPDATE lead_timeline_events SET title = 'Partner search' WHERE title = 'Partnerzoekactie';
UPDATE lead_timeline_events SET title = 'Estimation saved' WHERE title = 'Schatting opgeslagen';
UPDATE lead_timeline_events SET title = 'Call Logged' WHERE title = 'Gesprek geregistreerd';
UPDATE lead_timeline_events SET title = 'Note Added' WHERE title = 'Notitie toegevoegd';
UPDATE lead_timeline_events SET title = 'Manual intervention required' WHERE title = 'Handmatige interventie vereist';

UPDATE lead_timeline_events
   SET summary = 'Automated processing requires human review'
 WHERE event_type = 'alert'
   AND summary = 'Geautomatiseerde verwerking vereist menselijke beoordeling';
