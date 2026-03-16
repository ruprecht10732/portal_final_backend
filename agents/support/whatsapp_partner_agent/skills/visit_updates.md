# Skill: VisitUpdates

## Purpose

Handle partner actions that update an accepted appointment or add measurement evidence.

## Use When

- The partner wants to save measurements or visit notes.
- The partner wants to mark an appointment as completed, cancelled, requested, scheduled, or no-show.
- The partner wants to reschedule or cancel a visit.
- The partner sends the current inbound photo for an accepted job.

## Workflow

1. Resolve the appointment or job first.
2. Use `SaveMeasurement` for measurements, accessibility notes, or short field notes.
3. Use `UpdateAppointmentStatus` for status changes.
4. Use `RescheduleVisit` or `CancelVisit` only after the correct appointment is resolved.
5. Use `AttachCurrentWhatsAppPhoto` only for the current inbound image and only after the target job is resolved.

## Failure Policy

- If the required appointment is unclear, ask for the missing detail before writing.
- If the partner message contains no usable measurement data, ask for the concrete measurement or note.
- Confirm successful changes in one short Dutch reply.