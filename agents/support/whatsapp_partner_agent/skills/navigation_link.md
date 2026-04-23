# Tool: GetNavigationLink

## Purpose
Generate and provide a direct navigation or route link for a specific, fully resolved partner job or appointment.

## Execution Logic & Triggers
Execute this tool proactively when the partner asks for:
- The address or exact location of a job.
- A route, navigation, or instructions on "how to get there."
- A command like "Stuur me de navigatie."

**Strict Prerequisite:** You MUST have a fully resolved target job or appointment ID before triggering this tool. Refer to the **Job Resolution** workflow if the target is not yet clear.

## Autonomy & Workflow
1. **Resolve:** Identify the target via recent context or `GetMyJobs`.
2. **Execute:** Trigger `GetNavigationLink` silently once the target is unambiguous.
3. **Dispatch:** Return the resulting link in a short, practical Dutch reply. 
   - *Example:* "Hier is de route naar de klus in [Stad]: [Link]"

## Ambiguity & Failure Policy
- **Unresolved Target:** If the partner asks for navigation but you cannot isolate the specific job, PAUSE. Ask exactly ONE short clarifying question (e.g., "Voor welke klus heb je de navigatie nodig?").
- **No Link Available:** If the tool fails to generate a link or returns an empty result, state this plainly and provide the text-based address details (if already known from a prior tool result). 
   - *Dutch:* "Ik kan geen navigatielink genereren voor deze locatie. Het adres is: [Adres]."
- **Backend Error:** If the tool technical fails, respond: "Het ophalen van de navigatielink is mislukt. Probeer het later opnieuw."

## Hard Safety Constraints
- **Zero Fabrication:** NEVER invent, guess, or "hallucinate" an address or a navigation URL. Use only what the tool provides.
- **Strict Scoping:** NEVER provide navigation links or location data for jobs or leads that fall outside the authenticated partner's current scope.
- **No Narration:** Do not explain that you are "looking up the link" or "generating a route." Just provide the final link.