# Subsidy Analyzer Integration

## Trigger

- User clicks "Bereken subsidie" button in the quote detail component
- Frontend calls `POST /api/v1/quotes/{quoteId}/analyze-subsidy`

## Preconditions

- Quote exists and has at least one line item
- Quote is in an editable state (not yet finalized/sent)
- ISDE rules are available in the database (RAC_isde_measure_definitions table)
- Moonshot/Kimi LLM is configured

## Execution Model

1. **Backend receives request**
   - Extract `quoteId` from route parameter
   - Validate user has access to the quote
   - Create a new `RAC_subsidy_analyzer_jobs` record with status `pending`
   - Enqueue async task via asynq scheduler

2. **Async Task Processing**
   - Load quote + line items from database
   - Load ISDE measure definitions and rules
   - Construct LLM prompt with quote context and ISDE rules
   - Call Moonshot/Kimi LLM with structured context
   - LLM responds with `AcceptSubsidySuggestion` tool call
   - Persist result to `RAC_subsidy_analyzer_jobs.result` (JSONB)
   - Publish SSE event with progress/status updates

3. **Frontend Polling**
   - On job creation return, store `jobId`
   - Poll `GET /api/v1/subsidy-analysis-jobs/{jobId}` every 500ms
   - Subscribe to SSE events for real-time progress updates
   - When status = "completed", read `result` and apply to subsidy modal signals
   - When status = "failed", show error toast (no modal open)

## Outputs

- Analysis job record with `status`, `progress_percent`, `result` (ISDECalculationRequest JSON)
- SSE events: `ai_job_progress` with job ID, step, progress %
- Frontend modal is **not** auto-opened; user reviews and confirms

## Downstream Effects

- User reviews prefilled suggestion in modal
- User may override measure type or installation
- User clicks "Berekenen" manually to execute calculation
- If calculation succeeds, result replaces prefill suggestion
- If calculation fails, error is shown in modal
