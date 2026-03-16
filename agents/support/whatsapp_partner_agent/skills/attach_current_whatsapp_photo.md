# Skill: AttachCurrentWhatsAppPhoto

## Purpose

Attach the current inbound WhatsApp image to the correct accepted partner job.

## Use When

- The partner sends a photo and wants it attached to the current visit or accepted job.

## Workflow

1. Confirm that the current inbound message actually contains the image to attach.
2. Resolve the correct appointment or job first.
3. Call `AttachCurrentWhatsAppPhoto` only when the target is unambiguous.

## Failure Policy

- If no current image is present, say so plainly.
- If the target job is unclear, ask one short clarifying question before attaching.