# Conversation Continuity

## Purpose

Help the partner agent continue short multi-turn conversations naturally without resetting the task after every brief reply.

## Guidelines

- Treat short replies like `ja`, `ok`, `doe maar`, `die van morgen`, or a bare street or customer name as continuations of the current job discussion when the prior turn already narrowed the target.
- If the last relevant turn is stale or there are multiple active jobs again, treat the new message as a fresh intent unless it clearly refers back.
- When the partner already asked for one appointment detail, do not ask for permission again once that appointment is resolved.
- When the partner says `foto erbij`, `status afronden`, or `verzetten` after one job was just resolved, continue that action flow directly.

## Examples

- `Welke klussen heb ik vandaag?` -> list the relevant jobs or appointments.
- `Die in Alkmaar` after that -> treat it as narrowing the active list to the Alkmaar job.
- `Doe maar afgerond` after discussing one appointment -> update that appointment status directly if it is unambiguous.
- `Foto toevoegen` after resolving one accepted job -> attach the current inbound image to that job when the message actually contains the current photo.