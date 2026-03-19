# Skill: AcceptSubsidySuggestion

## Purpose

Persist and return the LLM's structured suggestion for subsidy calculation parameters.

## Tool Contract

```
AcceptSubsidySuggestion(
  measure_type_id: string (UUID),
  installation_meldcode_id: string (UUID),
  confidence: "high" | "medium" | "low",
  reasoning: string (Dutch explanation)
)
```

## Usage Guidelines

- **When to use**: After analyzing quote line items and matching to ISDE measures, call this tool exactly once to finalize the suggestion.
- **measure_type_id**: UUID of the selected ISDE measure definition (e.g., "Solar Installation"). Must match an entry in RAC_isde_measure_definitions.
- **installation_meldcode_id**: UUID of the selected installation meldcode (e.g., "Standaard zonnefloor"). Must match an entry in RAC_isde_installation_meldcodes.
- **confidence**: One of:
  - `high`: Multiple signals align; keywords, category, and specifications all support the match.
  - `medium`: Main signals align but some details are ambiguous or missing.
  - `low`: Limited context or conflicting signals; user should verify.
- **reasoning**: Clear Dutch explanation so the user understands the match and can override if needed. Example:
  ```
  "Onze analyse geeft aan dat uw offerte plaats biedt voor een Zonnestroom-installatie volgens de standaard meldcode 203. 
  Dit is gebaseerd op de beschrijving 'HR++ glas' in item 1 en categorie 'Duurzame Energie'. 
  De huistype (vrijstaand, 2000-gebouwd) past bij dit programma."
  ```

## Error Handling

- If **no matching measure** is found:
  - Call tool with `confidence: "low"` and `reasoning` explaining why no match was made
  - Set `measure_type_id` and `installation_meldcode_id` to empty/null (or the first available if required)
  - Frontend will receive this and show "no suggestion available" state
  
- If **measure is ambiguous**:
  - Return the highest-confidence option
  - Explain ambiguity in `reasoning` so user can verify and override

## Downstream Handling (Frontend)

- Frontend receives the result JSON
- If `confidence` is "high", modal opens with prefill and user can review/edit before clicking Calculate
- If `confidence` is "medium" or "low", modal opens with prefill but prominently displays the reasoning so user knows to double-check
- User can override measure or meldcode before clicking "Berekenen"
