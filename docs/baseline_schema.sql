--
-- PostgreSQL database dump
--

\restrict PclGHKpeDjdAKzBAEeRKQ7fzUJf12jv7pYMGseckg4mTiVvfVTryyuOBeSzDQi2

-- Dumped from database version 17.7
-- Dumped by pg_dump version 18.1

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--

-- *not* creating schema, since initdb creates it


--
-- Name: cube; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS cube WITH SCHEMA public;


--
-- Name: earthdistance; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS earthdistance WITH SCHEMA public;


--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: offer_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.offer_status AS ENUM (
    'pending',
    'sent',
    'accepted',
    'rejected',
    'expired'
);


--
-- Name: pipeline_stage; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.pipeline_stage AS ENUM (
    'Triage',
    'Nurturing',
    'Ready_For_Estimator',
    'Quote_Sent',
    'Ready_For_Partner',
    'Partner_Matching',
    'Partner_Assigned',
    'Manual_Intervention',
    'Completed',
    'Lost'
);


--
-- Name: pricing_source; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.pricing_source AS ENUM (
    'quote',
    'estimate'
);


--
-- Name: quote_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.quote_status AS ENUM (
    'Draft',
    'Sent',
    'Accepted',
    'Rejected',
    'Expired'
);


--
-- Name: rac_quote_attachment_source; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.rac_quote_attachment_source AS ENUM (
    'catalog',
    'manual'
);


SET default_table_access_method = heap;

--
-- Name: lead_timeline_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lead_timeline_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    service_id uuid,
    organization_id uuid NOT NULL,
    actor_type text NOT NULL,
    actor_name text NOT NULL,
    event_type text NOT NULL,
    title text NOT NULL,
    summary text,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: partner_invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.partner_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_service_id uuid NOT NULL,
    partner_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    status text NOT NULL,
    invited_at timestamp with time zone DEFAULT now() NOT NULL,
    responded_at timestamp with time zone,
    distance_km numeric(10,2),
    invite_metadata jsonb DEFAULT '{}'::jsonb,
    CONSTRAINT partner_invites_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'rejected'::text, 'expired'::text])))
);


--
-- Name: rac_appointment_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_appointment_attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    appointment_id uuid NOT NULL,
    file_key text NOT NULL,
    file_name text NOT NULL,
    content_type text,
    size_bytes bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL
);


--
-- Name: rac_appointment_availability_overrides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_appointment_availability_overrides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    date date NOT NULL,
    is_available boolean DEFAULT false NOT NULL,
    start_time time without time zone,
    end_time time without time zone,
    timezone text DEFAULT 'Europe/Amsterdam'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL,
    CONSTRAINT chk_availability_override_time_range CHECK (((end_time IS NULL) OR (start_time IS NULL) OR (end_time > start_time)))
);


--
-- Name: rac_appointment_availability_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_appointment_availability_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    weekday smallint NOT NULL,
    start_time time without time zone NOT NULL,
    end_time time without time zone NOT NULL,
    timezone text DEFAULT 'Europe/Amsterdam'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL,
    CONSTRAINT appointment_availability_rules_weekday_check CHECK (((weekday >= 0) AND (weekday <= 6))),
    CONSTRAINT chk_availability_time_range CHECK ((end_time > start_time))
);


--
-- Name: rac_appointment_visit_reports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_appointment_visit_reports (
    appointment_id uuid NOT NULL,
    measurements text,
    access_difficulty text,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL,
    CONSTRAINT appointment_visit_reports_access_difficulty_check CHECK (((access_difficulty IS NULL) OR (access_difficulty = ANY (ARRAY['Low'::text, 'Medium'::text, 'High'::text]))))
);


--
-- Name: rac_appointments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_appointments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    lead_id uuid,
    lead_service_id uuid,
    type text NOT NULL,
    title text NOT NULL,
    description text,
    location text,
    start_time timestamp with time zone NOT NULL,
    end_time timestamp with time zone NOT NULL,
    status text DEFAULT 'scheduled'::text NOT NULL,
    all_day boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL,
    meeting_link text,
    CONSTRAINT appointments_type_check CHECK ((type = ANY (ARRAY['lead_visit'::text, 'standalone'::text, 'blocked'::text]))),
    CONSTRAINT chk_lead_visit_refs CHECK (((type <> 'lead_visit'::text) OR ((lead_id IS NOT NULL) AND (lead_service_id IS NOT NULL)))),
    CONSTRAINT chk_time_range CHECK ((end_time > start_time)),
    CONSTRAINT rac_appointments_status_check CHECK ((status = ANY (ARRAY['scheduled'::text, 'requested'::text, 'completed'::text, 'cancelled'::text, 'no_show'::text])))
);


--
-- Name: rac_catalog_product_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_catalog_product_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    product_id uuid NOT NULL,
    asset_type text NOT NULL,
    file_key text,
    file_name text,
    content_type text,
    size_bytes bigint,
    url text,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT catalog_product_assets_asset_type_check CHECK ((asset_type = ANY (ARRAY['image'::text, 'document'::text, 'terms_url'::text]))),
    CONSTRAINT catalog_product_assets_storage_or_url_check CHECK ((((file_key IS NOT NULL) AND (url IS NULL)) OR ((file_key IS NULL) AND (url IS NOT NULL))))
);


--
-- Name: rac_catalog_product_materials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_catalog_product_materials (
    organization_id uuid NOT NULL,
    product_id uuid NOT NULL,
    material_id uuid NOT NULL
);


--
-- Name: rac_catalog_products; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_catalog_products (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    vat_rate_id uuid NOT NULL,
    title text NOT NULL,
    reference text NOT NULL,
    description text,
    price_cents bigint DEFAULT 0 NOT NULL,
    type text NOT NULL,
    period_count integer,
    period_unit text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    unit_price_cents bigint DEFAULT 0 NOT NULL,
    unit_label text,
    labor_time_text text,
    CONSTRAINT catalog_products_pricing_mode_check CHECK ((((price_cents > 0) AND (unit_price_cents = 0)) OR ((price_cents = 0) AND (unit_price_cents > 0)))),
    CONSTRAINT catalog_products_unit_label_check CHECK (((unit_price_cents = 0) OR ((unit_label IS NOT NULL) AND (btrim(unit_label) <> ''::text))))
);


--
-- Name: rac_catalog_vat_rates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_catalog_vat_rates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    name text NOT NULL,
    rate_bps integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_export_api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_export_api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    name text NOT NULL,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone
);


--
-- Name: rac_feed_comment_mentions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_feed_comment_mentions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    comment_id uuid NOT NULL,
    mentioned_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_feed_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_feed_comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_id text NOT NULL,
    event_source text NOT NULL,
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rac_feed_comments_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 2000)))
);


--
-- Name: rac_feed_pinned_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_feed_pinned_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    alert_type text NOT NULL,
    resolved_at timestamp with time zone,
    dismissed_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_feed_reactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_feed_reactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_id text NOT NULL,
    event_source text NOT NULL,
    reaction_type text NOT NULL,
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rac_feed_reactions_reaction_type_check CHECK ((reaction_type = ANY (ARRAY['thumbs-up'::text, 'heart'::text, 'party-popper'::text, 'flame'::text])))
);


--
-- Name: rac_feed_read_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_feed_read_state (
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    last_feed_viewed_at timestamp with time zone DEFAULT now() NOT NULL,
    feed_mode text DEFAULT 'all'::text NOT NULL,
    CONSTRAINT rac_feed_read_state_feed_mode_check CHECK ((feed_mode = ANY (ARRAY['all'::text, 'high_signal'::text])))
);


--
-- Name: rac_google_ads_exports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_google_ads_exports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    lead_service_id uuid NOT NULL,
    conversion_name text NOT NULL,
    conversion_time timestamp with time zone NOT NULL,
    conversion_value numeric(12,2),
    gclid text NOT NULL,
    order_id text NOT NULL,
    exported_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_google_lead_ids; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_google_lead_ids (
    lead_id text NOT NULL,
    organization_id uuid NOT NULL,
    lead_uuid uuid,
    is_test boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_google_webhook_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_google_webhook_configs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    google_key_hash text NOT NULL,
    google_key_prefix text NOT NULL,
    campaign_mappings jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_lead_activity; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_activity (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    user_id uuid NOT NULL,
    action text NOT NULL,
    meta jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL
);


--
-- Name: rac_lead_ai_analysis; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_ai_analysis (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    urgency_level text NOT NULL,
    urgency_reason text,
    summary text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    lead_service_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    lead_quality text DEFAULT 'Low'::text NOT NULL,
    recommended_action text DEFAULT 'RequestInfo'::text NOT NULL,
    missing_information jsonb DEFAULT '[]'::jsonb NOT NULL,
    preferred_contact_channel text DEFAULT 'WhatsApp'::text NOT NULL,
    suggested_contact_message text DEFAULT ''::text NOT NULL,
    CONSTRAINT rac_lead_ai_analysis_lead_quality_check CHECK ((lead_quality = ANY (ARRAY['Junk'::text, 'Low'::text, 'Potential'::text, 'High'::text, 'Urgent'::text]))),
    CONSTRAINT rac_lead_ai_analysis_preferred_contact_channel_check CHECK ((preferred_contact_channel = ANY (ARRAY['WhatsApp'::text, 'Email'::text]))),
    CONSTRAINT rac_lead_ai_analysis_recommended_action_check CHECK ((recommended_action = ANY (ARRAY['Reject'::text, 'RequestInfo'::text, 'ScheduleSurvey'::text, 'CallImmediately'::text]))),
    CONSTRAINT rac_lead_ai_analysis_urgency_level_check CHECK ((urgency_level = ANY (ARRAY['High'::text, 'Medium'::text, 'Low'::text])))
);


--
-- Name: rac_lead_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_notes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    author_id uuid NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    type text DEFAULT 'note'::text NOT NULL,
    organization_id uuid NOT NULL,
    parent_event_id uuid,
    parent_event_type text,
    service_id uuid,
    CONSTRAINT lead_notes_type_check CHECK ((type = ANY (ARRAY['note'::text, 'call'::text, 'text'::text, 'email'::text, 'system'::text]))),
    CONSTRAINT rac_lead_notes_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 2000))),
    CONSTRAINT rac_lead_notes_parent_event_type_check CHECK (((parent_event_type IS NULL) OR (parent_event_type = ANY (ARRAY['lead_activity'::text, 'quote_activity'::text, 'timeline_event'::text, 'appointment'::text]))))
);


--
-- Name: rac_lead_photo_analyses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_photo_analyses (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    service_id uuid NOT NULL,
    org_id uuid NOT NULL,
    summary text NOT NULL,
    observations jsonb DEFAULT '[]'::jsonb NOT NULL,
    scope_assessment character varying(20) NOT NULL,
    cost_indicators text,
    safety_concerns jsonb DEFAULT '[]'::jsonb,
    additional_info jsonb DEFAULT '[]'::jsonb,
    confidence_level character varying(10) NOT NULL,
    photo_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    measurements jsonb DEFAULT '[]'::jsonb,
    needs_onsite_measurement jsonb DEFAULT '[]'::jsonb,
    discrepancies jsonb DEFAULT '[]'::jsonb,
    extracted_text jsonb DEFAULT '[]'::jsonb,
    suggested_search_terms jsonb DEFAULT '[]'::jsonb,
    CONSTRAINT lead_photo_analyses_confidence_level_check CHECK (((confidence_level)::text = ANY ((ARRAY['High'::character varying, 'Medium'::character varying, 'Low'::character varying])::text[]))),
    CONSTRAINT lead_photo_analyses_scope_assessment_check CHECK (((scope_assessment)::text = ANY ((ARRAY['Small'::character varying, 'Medium'::character varying, 'Large'::character varying, 'Unclear'::character varying])::text[])))
);


--
-- Name: rac_lead_service_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_service_attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_service_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    file_key text NOT NULL,
    file_name text NOT NULL,
    content_type text,
    size_bytes bigint,
    uploaded_by uuid,
    created_at timestamp with time zone DEFAULT now()
);


--
-- Name: rac_lead_service_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_service_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    lead_service_id uuid NOT NULL,
    event_type text NOT NULL,
    status text,
    pipeline_stage text,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rac_lead_service_events_event_type_check CHECK ((event_type = ANY (ARRAY['status_changed'::text, 'pipeline_stage_changed'::text, 'service_created'::text])))
);


--
-- Name: rac_lead_services; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_lead_services (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lead_id uuid NOT NULL,
    status text DEFAULT 'New'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    service_type_id uuid NOT NULL,
    consumer_note text,
    source text DEFAULT 'manual'::text,
    organization_id uuid NOT NULL,
    pipeline_stage public.pipeline_stage DEFAULT 'Triage'::public.pipeline_stage NOT NULL,
    customer_preferences jsonb DEFAULT '{}'::jsonb,
    CONSTRAINT rac_lead_services_status_check CHECK ((status = ANY (ARRAY['New'::text, 'Attempted_Contact'::text, 'Scheduled'::text, 'Surveyed'::text, 'Bad_Lead'::text, 'Needs_Rescheduling'::text, 'Closed'::text])))
);


--
-- Name: rac_leads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_leads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    consumer_first_name text NOT NULL,
    consumer_last_name text NOT NULL,
    consumer_phone text NOT NULL,
    consumer_email text,
    consumer_role text DEFAULT 'Owner'::text NOT NULL,
    address_street text NOT NULL,
    address_house_number text NOT NULL,
    address_zip_code text NOT NULL,
    address_city text NOT NULL,
    assigned_agent_id uuid,
    viewed_by_id uuid,
    viewed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    source text DEFAULT 'manual'::text,
    projected_value_cents bigint DEFAULT 0 NOT NULL,
    latitude double precision,
    longitude double precision,
    organization_id uuid NOT NULL,
    energy_class text,
    energy_index double precision,
    energy_bouwjaar integer,
    energy_gebouwtype text,
    energy_label_valid_until timestamp with time zone,
    energy_label_registered_at timestamp with time zone,
    energy_primair_fossiel double precision,
    energy_bag_verblijfsobject_id text,
    energy_label_fetched_at timestamp with time zone,
    lead_enrichment_source text,
    lead_enrichment_postcode6 text,
    lead_enrichment_buurtcode text,
    lead_enrichment_gem_aardgasverbruik double precision,
    lead_enrichment_huishouden_grootte double precision,
    lead_enrichment_koopwoningen_pct double precision,
    lead_enrichment_bouwjaar_vanaf2000_pct double precision,
    lead_enrichment_mediaan_vermogen_x1000 double precision,
    lead_enrichment_huishoudens_met_kinderen_pct double precision,
    lead_enrichment_confidence double precision,
    lead_enrichment_fetched_at timestamp with time zone,
    lead_score integer,
    lead_score_pre_ai integer,
    lead_score_factors jsonb,
    lead_score_version text,
    lead_score_updated_at timestamp with time zone,
    lead_enrichment_postcode4 text,
    lead_enrichment_data_year integer,
    lead_enrichment_gem_elektriciteitsverbruik double precision,
    lead_enrichment_woz_waarde double precision,
    lead_enrichment_gem_inkomen double precision,
    lead_enrichment_pct_hoog_inkomen double precision,
    lead_enrichment_pct_laag_inkomen double precision,
    lead_enrichment_stedelijkheid integer,
    public_token text,
    public_token_expires_at timestamp with time zone,
    raw_form_data jsonb,
    webhook_source_domain text,
    is_incomplete boolean DEFAULT false NOT NULL,
    gclid text,
    utm_source text,
    utm_medium text,
    utm_campaign text,
    utm_content text,
    utm_term text,
    ad_landing_page text,
    referrer_url text,
    whatsapp_opted_in boolean DEFAULT true NOT NULL,
    google_campaign_id bigint,
    google_adgroup_id bigint,
    google_creative_id bigint,
    google_form_id bigint,
    CONSTRAINT rac_leads_consumer_role_check CHECK ((consumer_role = ANY (ARRAY['Owner'::text, 'Tenant'::text, 'Landlord'::text])))
);


--
-- Name: rac_organization_invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_organization_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    used_at timestamp with time zone,
    used_by uuid
);


--
-- Name: rac_organization_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_organization_members (
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_organization_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_organization_settings (
    organization_id uuid NOT NULL,
    quote_payment_days integer DEFAULT 7 NOT NULL,
    quote_valid_days integer DEFAULT 14 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    whatsapp_device_id text,
    smtp_host text,
    smtp_port integer,
    smtp_username text,
    smtp_password text,
    smtp_from_email text,
    smtp_from_name text
);


--
-- Name: rac_organizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_organizations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    email text,
    phone text,
    vat_number text,
    kvk_number text,
    address_line1 text,
    address_line2 text,
    postal_code text,
    city text,
    country text,
    logo_file_key text,
    logo_file_name text,
    logo_content_type text,
    logo_size_bytes bigint
);


--
-- Name: rac_partner_invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_partner_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    partner_id uuid NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    used_at timestamp with time zone,
    used_by uuid,
    lead_id uuid,
    lead_service_id uuid
);


--
-- Name: rac_partner_leads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_partner_leads (
    organization_id uuid NOT NULL,
    partner_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_partner_offers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_partner_offers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    partner_id uuid NOT NULL,
    lead_service_id uuid NOT NULL,
    public_token text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    pricing_source public.pricing_source NOT NULL,
    customer_price_cents bigint NOT NULL,
    vakman_price_cents bigint NOT NULL,
    status public.offer_status DEFAULT 'pending'::public.offer_status NOT NULL,
    accepted_at timestamp with time zone,
    rejected_at timestamp with time zone,
    rejection_reason text,
    inspection_availability jsonb,
    job_availability jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    job_summary_short text,
    builder_summary text,
    CONSTRAINT rac_partner_offers_customer_price_cents_check CHECK ((customer_price_cents >= 0)),
    CONSTRAINT rac_partner_offers_vakman_price_cents_check CHECK ((vakman_price_cents >= 0))
);


--
-- Name: rac_partner_service_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_partner_service_types (
    partner_id uuid NOT NULL,
    service_type_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_partners; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_partners (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    business_name text NOT NULL,
    kvk_number text NOT NULL,
    vat_number text NOT NULL,
    address_line1 text NOT NULL,
    address_line2 text,
    postal_code text NOT NULL,
    city text NOT NULL,
    country text NOT NULL,
    contact_name text NOT NULL,
    contact_email text NOT NULL,
    contact_phone text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    logo_file_key text,
    logo_file_name text,
    logo_content_type text,
    logo_size_bytes bigint,
    latitude double precision,
    longitude double precision,
    house_number text,
    whatsapp_opted_in boolean DEFAULT true NOT NULL
);


--
-- Name: rac_quote_activity; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_activity (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    quote_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    event_type text NOT NULL,
    message text NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_quote_annotations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_annotations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    quote_item_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    author_type text NOT NULL,
    author_id uuid,
    text text NOT NULL,
    is_resolved boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rac_quote_annotations_author_type_check CHECK ((author_type = ANY (ARRAY['customer'::text, 'agent'::text])))
);


--
-- Name: rac_quote_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    quote_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    filename text NOT NULL,
    file_key text NOT NULL,
    source public.rac_quote_attachment_source DEFAULT 'catalog'::public.rac_quote_attachment_source NOT NULL,
    catalog_product_id uuid,
    enabled boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_quote_counters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_counters (
    organization_id uuid NOT NULL,
    last_number integer DEFAULT 0 NOT NULL
);


--
-- Name: rac_quote_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    quote_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    description text NOT NULL,
    quantity text DEFAULT '1 x'::text NOT NULL,
    quantity_numeric numeric(12,3) DEFAULT 1 NOT NULL,
    unit_price_cents bigint DEFAULT 0 NOT NULL,
    tax_rate integer DEFAULT 2100 NOT NULL,
    is_optional boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_selected boolean DEFAULT true NOT NULL,
    catalog_product_id uuid
);


--
-- Name: rac_quote_urls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quote_urls (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    quote_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    label text NOT NULL,
    href text NOT NULL,
    accepted boolean DEFAULT false NOT NULL,
    catalog_product_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_quotes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_quotes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    lead_service_id uuid,
    quote_number text NOT NULL,
    status public.quote_status DEFAULT 'Draft'::public.quote_status NOT NULL,
    valid_until timestamp with time zone,
    notes text,
    pricing_mode text DEFAULT 'exclusive'::text NOT NULL,
    discount_type text DEFAULT 'percentage'::text NOT NULL,
    discount_value bigint DEFAULT 0 NOT NULL,
    subtotal_cents bigint DEFAULT 0 NOT NULL,
    discount_amount_cents bigint DEFAULT 0 NOT NULL,
    tax_total_cents bigint DEFAULT 0 NOT NULL,
    total_cents bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    public_token text,
    public_token_expires_at timestamp with time zone,
    viewed_at timestamp with time zone,
    accepted_at timestamp with time zone,
    rejected_at timestamp with time zone,
    rejection_reason text,
    signature_name text,
    signature_data text,
    signature_ip text,
    pdf_file_key text,
    preview_token text,
    preview_token_expires_at timestamp with time zone,
    created_by_id uuid,
    financing_disclaimer boolean DEFAULT false NOT NULL
);


--
-- Name: rac_refresh_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_roles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL
);


--
-- Name: rac_service_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_service_types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    icon text,
    color text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    organization_id uuid NOT NULL,
    intake_guidelines text
);


--
-- Name: rac_user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_user_roles (
    user_id uuid NOT NULL,
    role_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_user_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_user_settings (
    user_id uuid NOT NULL,
    preferred_language text DEFAULT 'nl'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_user_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_user_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    type text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    is_email_verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    first_name text,
    last_name text,
    onboarding_completed_at timestamp with time zone
);


--
-- Name: rac_webhook_api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rac_webhook_api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    allowed_domains text[] DEFAULT '{}'::text[] NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: rac_appointment_attachments appointment_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_attachments
    ADD CONSTRAINT appointment_attachments_pkey PRIMARY KEY (id);


--
-- Name: rac_appointment_availability_overrides appointment_availability_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_overrides
    ADD CONSTRAINT appointment_availability_overrides_pkey PRIMARY KEY (id);


--
-- Name: rac_appointment_availability_rules appointment_availability_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_rules
    ADD CONSTRAINT appointment_availability_rules_pkey PRIMARY KEY (id);


--
-- Name: rac_appointment_visit_reports appointment_visit_reports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_visit_reports
    ADD CONSTRAINT appointment_visit_reports_pkey PRIMARY KEY (appointment_id);


--
-- Name: rac_appointments appointments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointments
    ADD CONSTRAINT appointments_pkey PRIMARY KEY (id);


--
-- Name: rac_catalog_product_assets catalog_product_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_assets
    ADD CONSTRAINT catalog_product_assets_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_photo_analyses lead_photo_analyses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_photo_analyses
    ADD CONSTRAINT lead_photo_analyses_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_service_attachments lead_service_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_attachments
    ADD CONSTRAINT lead_service_attachments_pkey PRIMARY KEY (id);


--
-- Name: lead_timeline_events lead_timeline_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_timeline_events
    ADD CONSTRAINT lead_timeline_events_pkey PRIMARY KEY (id);


--
-- Name: rac_organization_invites organization_invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_invites
    ADD CONSTRAINT organization_invites_pkey PRIMARY KEY (id);


--
-- Name: rac_organization_invites organization_invites_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_invites
    ADD CONSTRAINT organization_invites_token_hash_key UNIQUE (token_hash);


--
-- Name: rac_organization_members organization_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_members
    ADD CONSTRAINT organization_members_pkey PRIMARY KEY (organization_id, user_id);


--
-- Name: rac_organizations organizations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);


--
-- Name: partner_invites partner_invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partner_invites
    ADD CONSTRAINT partner_invites_pkey PRIMARY KEY (id);


--
-- Name: rac_catalog_product_materials rac_catalog_product_materials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_materials
    ADD CONSTRAINT rac_catalog_product_materials_pkey PRIMARY KEY (organization_id, product_id, material_id);


--
-- Name: rac_catalog_products rac_catalog_products_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_products
    ADD CONSTRAINT rac_catalog_products_pkey PRIMARY KEY (id);


--
-- Name: rac_catalog_vat_rates rac_catalog_vat_rates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_vat_rates
    ADD CONSTRAINT rac_catalog_vat_rates_pkey PRIMARY KEY (id);


--
-- Name: rac_export_api_keys rac_export_api_keys_key_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_export_api_keys
    ADD CONSTRAINT rac_export_api_keys_key_hash_key UNIQUE (key_hash);


--
-- Name: rac_export_api_keys rac_export_api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_export_api_keys
    ADD CONSTRAINT rac_export_api_keys_pkey PRIMARY KEY (id);


--
-- Name: rac_feed_comment_mentions rac_feed_comment_mentions_comment_id_mentioned_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comment_mentions
    ADD CONSTRAINT rac_feed_comment_mentions_comment_id_mentioned_user_id_key UNIQUE (comment_id, mentioned_user_id);


--
-- Name: rac_feed_comment_mentions rac_feed_comment_mentions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comment_mentions
    ADD CONSTRAINT rac_feed_comment_mentions_pkey PRIMARY KEY (id);


--
-- Name: rac_feed_comments rac_feed_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comments
    ADD CONSTRAINT rac_feed_comments_pkey PRIMARY KEY (id);


--
-- Name: rac_feed_pinned_alerts rac_feed_pinned_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_pinned_alerts
    ADD CONSTRAINT rac_feed_pinned_alerts_pkey PRIMARY KEY (id);


--
-- Name: rac_feed_reactions rac_feed_reactions_event_id_event_source_reaction_type_user_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_reactions
    ADD CONSTRAINT rac_feed_reactions_event_id_event_source_reaction_type_user_key UNIQUE (event_id, event_source, reaction_type, user_id);


--
-- Name: rac_feed_reactions rac_feed_reactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_reactions
    ADD CONSTRAINT rac_feed_reactions_pkey PRIMARY KEY (id);


--
-- Name: rac_feed_read_state rac_feed_read_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_read_state
    ADD CONSTRAINT rac_feed_read_state_pkey PRIMARY KEY (user_id, organization_id);


--
-- Name: rac_google_ads_exports rac_google_ads_exports_organization_id_order_id_conversion__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_ads_exports
    ADD CONSTRAINT rac_google_ads_exports_organization_id_order_id_conversion__key UNIQUE (organization_id, order_id, conversion_name);


--
-- Name: rac_google_ads_exports rac_google_ads_exports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_ads_exports
    ADD CONSTRAINT rac_google_ads_exports_pkey PRIMARY KEY (id);


--
-- Name: rac_google_lead_ids rac_google_lead_ids_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_lead_ids
    ADD CONSTRAINT rac_google_lead_ids_pkey PRIMARY KEY (lead_id);


--
-- Name: rac_google_webhook_configs rac_google_webhook_configs_google_key_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_webhook_configs
    ADD CONSTRAINT rac_google_webhook_configs_google_key_hash_key UNIQUE (google_key_hash);


--
-- Name: rac_google_webhook_configs rac_google_webhook_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_webhook_configs
    ADD CONSTRAINT rac_google_webhook_configs_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_activity rac_lead_activity_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_activity
    ADD CONSTRAINT rac_lead_activity_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_ai_analysis rac_lead_ai_analysis_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_ai_analysis
    ADD CONSTRAINT rac_lead_ai_analysis_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_notes rac_lead_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_notes
    ADD CONSTRAINT rac_lead_notes_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_service_events rac_lead_service_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_events
    ADD CONSTRAINT rac_lead_service_events_pkey PRIMARY KEY (id);


--
-- Name: rac_lead_services rac_lead_services_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_services
    ADD CONSTRAINT rac_lead_services_pkey PRIMARY KEY (id);


--
-- Name: rac_leads rac_leads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_leads
    ADD CONSTRAINT rac_leads_pkey PRIMARY KEY (id);


--
-- Name: rac_leads rac_leads_public_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_leads
    ADD CONSTRAINT rac_leads_public_token_key UNIQUE (public_token);


--
-- Name: rac_organization_settings rac_organization_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_settings
    ADD CONSTRAINT rac_organization_settings_pkey PRIMARY KEY (organization_id);


--
-- Name: rac_partner_invites rac_partner_invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_pkey PRIMARY KEY (id);


--
-- Name: rac_partner_invites rac_partner_invites_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_token_hash_key UNIQUE (token_hash);


--
-- Name: rac_partner_leads rac_partner_leads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_leads
    ADD CONSTRAINT rac_partner_leads_pkey PRIMARY KEY (organization_id, partner_id, lead_id);


--
-- Name: rac_partner_offers rac_partner_offers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_offers
    ADD CONSTRAINT rac_partner_offers_pkey PRIMARY KEY (id);


--
-- Name: rac_partner_offers rac_partner_offers_public_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_offers
    ADD CONSTRAINT rac_partner_offers_public_token_key UNIQUE (public_token);


--
-- Name: rac_partner_service_types rac_partner_service_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_service_types
    ADD CONSTRAINT rac_partner_service_types_pkey PRIMARY KEY (partner_id, service_type_id);


--
-- Name: rac_partners rac_partners_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partners
    ADD CONSTRAINT rac_partners_pkey PRIMARY KEY (id);


--
-- Name: rac_quote_activity rac_quote_activity_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_activity
    ADD CONSTRAINT rac_quote_activity_pkey PRIMARY KEY (id);


--
-- Name: rac_quote_annotations rac_quote_annotations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_annotations
    ADD CONSTRAINT rac_quote_annotations_pkey PRIMARY KEY (id);


--
-- Name: rac_quote_attachments rac_quote_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_attachments
    ADD CONSTRAINT rac_quote_attachments_pkey PRIMARY KEY (id);


--
-- Name: rac_quote_counters rac_quote_counters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_counters
    ADD CONSTRAINT rac_quote_counters_pkey PRIMARY KEY (organization_id);


--
-- Name: rac_quote_items rac_quote_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_items
    ADD CONSTRAINT rac_quote_items_pkey PRIMARY KEY (id);


--
-- Name: rac_quote_urls rac_quote_urls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_urls
    ADD CONSTRAINT rac_quote_urls_pkey PRIMARY KEY (id);


--
-- Name: rac_quotes rac_quotes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_pkey PRIMARY KEY (id);


--
-- Name: rac_quotes rac_quotes_preview_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_preview_token_key UNIQUE (preview_token);


--
-- Name: rac_quotes rac_quotes_public_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_public_token_key UNIQUE (public_token);


--
-- Name: rac_refresh_tokens rac_refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_refresh_tokens
    ADD CONSTRAINT rac_refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: rac_refresh_tokens rac_refresh_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_refresh_tokens
    ADD CONSTRAINT rac_refresh_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: rac_roles rac_roles_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_roles
    ADD CONSTRAINT rac_roles_name_key UNIQUE (name);


--
-- Name: rac_roles rac_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_roles
    ADD CONSTRAINT rac_roles_pkey PRIMARY KEY (id);


--
-- Name: rac_service_types rac_service_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_service_types
    ADD CONSTRAINT rac_service_types_pkey PRIMARY KEY (id);


--
-- Name: rac_user_roles rac_user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_roles
    ADD CONSTRAINT rac_user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- Name: rac_user_settings rac_user_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_settings
    ADD CONSTRAINT rac_user_settings_pkey PRIMARY KEY (user_id);


--
-- Name: rac_user_tokens rac_user_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_tokens
    ADD CONSTRAINT rac_user_tokens_pkey PRIMARY KEY (id);


--
-- Name: rac_user_tokens rac_user_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_tokens
    ADD CONSTRAINT rac_user_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: rac_users rac_users_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_users
    ADD CONSTRAINT rac_users_email_key UNIQUE (email);


--
-- Name: rac_users rac_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_users
    ADD CONSTRAINT rac_users_pkey PRIMARY KEY (id);


--
-- Name: rac_webhook_api_keys rac_webhook_api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_webhook_api_keys
    ADD CONSTRAINT rac_webhook_api_keys_pkey PRIMARY KEY (id);


--
-- Name: idx_appointment_attachments_appointment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_attachments_appointment_id ON public.rac_appointment_attachments USING btree (appointment_id);


--
-- Name: idx_appointment_attachments_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_attachments_org ON public.rac_appointment_attachments USING btree (organization_id);


--
-- Name: idx_appointment_availability_overrides_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_availability_overrides_org ON public.rac_appointment_availability_overrides USING btree (organization_id);


--
-- Name: idx_appointment_availability_rules_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_availability_rules_org ON public.rac_appointment_availability_rules USING btree (organization_id);


--
-- Name: idx_appointment_visit_reports_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointment_visit_reports_org ON public.rac_appointment_visit_reports USING btree (organization_id);


--
-- Name: idx_appointments_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_lead_id ON public.rac_appointments USING btree (lead_id) WHERE (lead_id IS NOT NULL);


--
-- Name: idx_appointments_lead_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_lead_service_id ON public.rac_appointments USING btree (lead_service_id) WHERE (lead_service_id IS NOT NULL);


--
-- Name: idx_appointments_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_org ON public.rac_appointments USING btree (organization_id);


--
-- Name: idx_appointments_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_org_time ON public.rac_appointments USING btree (organization_id, start_time, end_time);


--
-- Name: idx_appointments_org_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_org_user ON public.rac_appointments USING btree (organization_id, user_id);


--
-- Name: idx_appointments_start_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_start_time ON public.rac_appointments USING btree (start_time);


--
-- Name: idx_appointments_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_status ON public.rac_appointments USING btree (status);


--
-- Name: idx_appointments_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_type ON public.rac_appointments USING btree (type);


--
-- Name: idx_appointments_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_user_id ON public.rac_appointments USING btree (user_id);


--
-- Name: idx_appointments_user_time_range; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_appointments_user_time_range ON public.rac_appointments USING btree (user_id, start_time, end_time);


--
-- Name: idx_availability_overrides_user_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_availability_overrides_user_date ON public.rac_appointment_availability_overrides USING btree (user_id, date);


--
-- Name: idx_availability_rules_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_availability_rules_user_id ON public.rac_appointment_availability_rules USING btree (user_id);


--
-- Name: idx_catalog_product_assets_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_assets_org ON public.rac_catalog_product_assets USING btree (organization_id);


--
-- Name: idx_catalog_product_assets_product; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_assets_product ON public.rac_catalog_product_assets USING btree (product_id);


--
-- Name: idx_catalog_product_assets_product_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_assets_product_org ON public.rac_catalog_product_assets USING btree (product_id, organization_id);


--
-- Name: idx_catalog_product_assets_product_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_assets_product_type ON public.rac_catalog_product_assets USING btree (product_id, asset_type);


--
-- Name: idx_catalog_product_materials_material; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_materials_material ON public.rac_catalog_product_materials USING btree (material_id);


--
-- Name: idx_catalog_product_materials_product; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_product_materials_product ON public.rac_catalog_product_materials USING btree (product_id);


--
-- Name: idx_catalog_products_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_products_org_id ON public.rac_catalog_products USING btree (organization_id);


--
-- Name: idx_catalog_products_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_products_type ON public.rac_catalog_products USING btree (type);


--
-- Name: idx_catalog_products_vat_rate_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_products_vat_rate_id ON public.rac_catalog_products USING btree (vat_rate_id);


--
-- Name: idx_catalog_vat_rates_org_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_catalog_vat_rates_org_name ON public.rac_catalog_vat_rates USING btree (organization_id, name);


--
-- Name: idx_export_keys_hash_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_export_keys_hash_active ON public.rac_export_api_keys USING btree (key_hash) WHERE (is_active = true);


--
-- Name: idx_export_keys_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_export_keys_org ON public.rac_export_api_keys USING btree (organization_id);


--
-- Name: idx_feed_comment_mentions_comment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_comment_mentions_comment ON public.rac_feed_comment_mentions USING btree (comment_id);


--
-- Name: idx_feed_comment_mentions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_comment_mentions_user ON public.rac_feed_comment_mentions USING btree (mentioned_user_id);


--
-- Name: idx_feed_comments_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_comments_event ON public.rac_feed_comments USING btree (event_id, event_source);


--
-- Name: idx_feed_comments_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_comments_org ON public.rac_feed_comments USING btree (org_id);


--
-- Name: idx_feed_pinned_alerts_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_pinned_alerts_org ON public.rac_feed_pinned_alerts USING btree (organization_id) WHERE (resolved_at IS NULL);


--
-- Name: idx_feed_reactions_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_reactions_event ON public.rac_feed_reactions USING btree (event_id, event_source);


--
-- Name: idx_feed_reactions_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_reactions_org ON public.rac_feed_reactions USING btree (org_id);


--
-- Name: idx_gads_exports_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gads_exports_org_time ON public.rac_google_ads_exports USING btree (organization_id, exported_at DESC);


--
-- Name: idx_google_lead_ids_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_google_lead_ids_created ON public.rac_google_lead_ids USING btree (created_at DESC);


--
-- Name: idx_google_lead_ids_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_google_lead_ids_org ON public.rac_google_lead_ids USING btree (organization_id);


--
-- Name: idx_google_webhook_configs_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_google_webhook_configs_key_hash ON public.rac_google_webhook_configs USING btree (google_key_hash) WHERE (is_active = true);


--
-- Name: idx_google_webhook_configs_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_google_webhook_configs_org ON public.rac_google_webhook_configs USING btree (organization_id);


--
-- Name: idx_invites_by_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invites_by_org ON public.partner_invites USING btree (organization_id, invited_at DESC);


--
-- Name: idx_invites_by_partner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invites_by_partner ON public.partner_invites USING btree (partner_id, status, invited_at DESC);


--
-- Name: idx_invites_by_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invites_by_service ON public.partner_invites USING btree (lead_service_id, status);


--
-- Name: idx_lead_activity_cluster; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_activity_cluster ON public.rac_lead_activity USING btree (organization_id, lead_id, action, created_at DESC);


--
-- Name: idx_lead_activity_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_activity_lead_id ON public.rac_lead_activity USING btree (lead_id);


--
-- Name: idx_lead_activity_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_activity_org ON public.rac_lead_activity USING btree (organization_id);


--
-- Name: idx_lead_activity_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_activity_org_created ON public.rac_lead_activity USING btree (organization_id, created_at DESC);


--
-- Name: idx_lead_ai_analysis_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_ai_analysis_created_at ON public.rac_lead_ai_analysis USING btree (created_at DESC);


--
-- Name: idx_lead_ai_analysis_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_ai_analysis_lead_id ON public.rac_lead_ai_analysis USING btree (lead_id);


--
-- Name: idx_lead_ai_analysis_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_ai_analysis_org ON public.rac_lead_ai_analysis USING btree (organization_id);


--
-- Name: idx_lead_ai_analysis_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_ai_analysis_service_id ON public.rac_lead_ai_analysis USING btree (lead_service_id);


--
-- Name: idx_lead_notes_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_notes_created_at ON public.rac_lead_notes USING btree (created_at DESC);


--
-- Name: idx_lead_notes_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_notes_lead_id ON public.rac_lead_notes USING btree (lead_id);


--
-- Name: idx_lead_notes_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_notes_org ON public.rac_lead_notes USING btree (organization_id);


--
-- Name: idx_lead_notes_parent_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_notes_parent_event ON public.rac_lead_notes USING btree (parent_event_id) WHERE (parent_event_id IS NOT NULL);


--
-- Name: idx_lead_notes_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_notes_service ON public.rac_lead_notes USING btree (lead_id, service_id, created_at DESC);


--
-- Name: idx_lead_service_attachments_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_service_attachments_org ON public.rac_lead_service_attachments USING btree (organization_id);


--
-- Name: idx_lead_service_attachments_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_service_attachments_service ON public.rac_lead_service_attachments USING btree (lead_service_id);


--
-- Name: idx_lead_service_attachments_service_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_service_attachments_service_org ON public.rac_lead_service_attachments USING btree (lead_service_id, organization_id);


--
-- Name: idx_lead_service_events_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_service_events_org_time ON public.rac_lead_service_events USING btree (organization_id, occurred_at DESC);


--
-- Name: idx_lead_service_events_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_service_events_service ON public.rac_lead_service_events USING btree (lead_service_id, occurred_at DESC);


--
-- Name: idx_lead_services_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_services_created_at ON public.rac_lead_services USING btree (created_at DESC);


--
-- Name: idx_lead_services_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_services_lead_id ON public.rac_lead_services USING btree (lead_id);


--
-- Name: idx_lead_services_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_services_org ON public.rac_lead_services USING btree (organization_id);


--
-- Name: idx_lead_services_pipeline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_services_pipeline ON public.rac_lead_services USING btree (pipeline_stage);


--
-- Name: idx_lead_services_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_services_status ON public.rac_lead_services USING btree (status);


--
-- Name: idx_leads_address_city; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_address_city ON public.rac_leads USING btree (address_city);


--
-- Name: idx_leads_address_house_number; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_address_house_number ON public.rac_leads USING btree (address_house_number);


--
-- Name: idx_leads_address_street; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_address_street ON public.rac_leads USING btree (address_street);


--
-- Name: idx_leads_address_zip_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_address_zip_code ON public.rac_leads USING btree (address_zip_code);


--
-- Name: idx_leads_assigned_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_assigned_agent ON public.rac_leads USING btree (assigned_agent_id);


--
-- Name: idx_leads_assigned_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_assigned_agent_id ON public.rac_leads USING btree (assigned_agent_id);


--
-- Name: idx_leads_consumer_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_consumer_email ON public.rac_leads USING btree (consumer_email);


--
-- Name: idx_leads_consumer_first_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_consumer_first_name ON public.rac_leads USING btree (consumer_first_name);


--
-- Name: idx_leads_consumer_last_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_consumer_last_name ON public.rac_leads USING btree (consumer_last_name);


--
-- Name: idx_leads_consumer_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_consumer_role ON public.rac_leads USING btree (consumer_role);


--
-- Name: idx_leads_coordinates; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_coordinates ON public.rac_leads USING btree (latitude, longitude) WHERE ((latitude IS NOT NULL) AND (longitude IS NOT NULL));


--
-- Name: idx_leads_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_created_at ON public.rac_leads USING btree (created_at DESC);


--
-- Name: idx_leads_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_deleted_at ON public.rac_leads USING btree (deleted_at);


--
-- Name: idx_leads_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_email ON public.rac_leads USING btree (consumer_email) WHERE (consumer_email IS NOT NULL);


--
-- Name: idx_leads_energy_class; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_energy_class ON public.rac_leads USING btree (energy_class) WHERE (energy_class IS NOT NULL);


--
-- Name: idx_leads_gclid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_gclid ON public.rac_leads USING btree (gclid) WHERE (gclid IS NOT NULL);


--
-- Name: idx_leads_lead_score; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_lead_score ON public.rac_leads USING btree (lead_score) WHERE (lead_score IS NOT NULL);


--
-- Name: idx_leads_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_org ON public.rac_leads USING btree (organization_id);


--
-- Name: idx_leads_org_assigned; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_org_assigned ON public.rac_leads USING btree (organization_id, assigned_agent_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_leads_org_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_org_created_at ON public.rac_leads USING btree (organization_id, created_at DESC);


--
-- Name: idx_leads_org_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_org_deleted ON public.rac_leads USING btree (organization_id, deleted_at) WHERE (deleted_at IS NULL);


--
-- Name: idx_leads_phone; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_phone ON public.rac_leads USING btree (consumer_phone);


--
-- Name: idx_leads_phone_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_phone_email ON public.rac_leads USING btree (consumer_phone, consumer_email);


--
-- Name: idx_leads_public_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_public_token ON public.rac_leads USING btree (public_token) WHERE (public_token IS NOT NULL);


--
-- Name: idx_organization_invites_active_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_organization_invites_active_email ON public.rac_organization_invites USING btree (organization_id, lower(email)) WHERE (used_at IS NULL);


--
-- Name: idx_organization_members_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_organization_members_user_id ON public.rac_organization_members USING btree (user_id);


--
-- Name: idx_partner_invites_active_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_partner_invites_active_email ON public.rac_partner_invites USING btree (organization_id, partner_id, lower(email)) WHERE (used_at IS NULL);


--
-- Name: idx_partner_invites_lead; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_invites_lead ON public.rac_partner_invites USING btree (lead_id);


--
-- Name: idx_partner_invites_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_invites_org ON public.rac_partner_invites USING btree (organization_id);


--
-- Name: idx_partner_invites_partner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_invites_partner ON public.rac_partner_invites USING btree (partner_id);


--
-- Name: idx_partner_leads_lead; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_leads_lead ON public.rac_partner_leads USING btree (lead_id);


--
-- Name: idx_partner_leads_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_leads_org ON public.rac_partner_leads USING btree (organization_id);


--
-- Name: idx_partner_leads_partner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_leads_partner ON public.rac_partner_leads USING btree (partner_id);


--
-- Name: idx_partner_offers_exclusive_acceptance; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_partner_offers_exclusive_acceptance ON public.rac_partner_offers USING btree (lead_service_id) WHERE (status = 'accepted'::public.offer_status);


--
-- Name: idx_partner_offers_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_offers_expiry ON public.rac_partner_offers USING btree (status, expires_at) WHERE (status = ANY (ARRAY['pending'::public.offer_status, 'sent'::public.offer_status]));


--
-- Name: idx_partner_offers_partner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_offers_partner ON public.rac_partner_offers USING btree (partner_id);


--
-- Name: idx_partner_offers_service; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_offers_service ON public.rac_partner_offers USING btree (lead_service_id);


--
-- Name: idx_partner_offers_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_offers_token ON public.rac_partner_offers USING btree (public_token);


--
-- Name: idx_partner_service_types_service_type_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partner_service_types_service_type_id ON public.rac_partner_service_types USING btree (service_type_id);


--
-- Name: idx_partners_contact_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partners_contact_email ON public.rac_partners USING btree (organization_id, lower(contact_email));


--
-- Name: idx_partners_coordinates; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partners_coordinates ON public.rac_partners USING btree (latitude, longitude) WHERE ((latitude IS NOT NULL) AND (longitude IS NOT NULL));


--
-- Name: idx_partners_house_number; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partners_house_number ON public.rac_partners USING btree (house_number);


--
-- Name: idx_partners_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partners_org ON public.rac_partners USING btree (organization_id);


--
-- Name: idx_partners_org_business_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_partners_org_business_name ON public.rac_partners USING btree (organization_id, lower(business_name));


--
-- Name: idx_partners_org_kvk; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_partners_org_kvk ON public.rac_partners USING btree (organization_id, kvk_number);


--
-- Name: idx_partners_org_vat; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_partners_org_vat ON public.rac_partners USING btree (organization_id, vat_number);


--
-- Name: idx_photo_analyses_lead_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_photo_analyses_lead_id ON public.rac_lead_photo_analyses USING btree (lead_id);


--
-- Name: idx_photo_analyses_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_photo_analyses_org_id ON public.rac_lead_photo_analyses USING btree (org_id);


--
-- Name: idx_photo_analyses_service_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_photo_analyses_service_id ON public.rac_lead_photo_analyses USING btree (service_id);


--
-- Name: idx_quote_activity_cluster; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_activity_cluster ON public.rac_quote_activity USING btree (organization_id, event_type, created_at DESC);


--
-- Name: idx_quote_activity_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_activity_org_id ON public.rac_quote_activity USING btree (organization_id);


--
-- Name: idx_quote_activity_quote_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_activity_quote_id ON public.rac_quote_activity USING btree (quote_id);


--
-- Name: idx_quote_annotations_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_annotations_item ON public.rac_quote_annotations USING btree (quote_item_id);


--
-- Name: idx_quote_attachments_quote_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_attachments_quote_id ON public.rac_quote_attachments USING btree (quote_id);


--
-- Name: idx_quote_attachments_unique_file_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_quote_attachments_unique_file_key ON public.rac_quote_attachments USING btree (quote_id, file_key);


--
-- Name: idx_quote_items_catalog_product; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_items_catalog_product ON public.rac_quote_items USING btree (catalog_product_id) WHERE (catalog_product_id IS NOT NULL);


--
-- Name: idx_quote_items_quote; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_items_quote ON public.rac_quote_items USING btree (quote_id);


--
-- Name: idx_quote_urls_quote_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quote_urls_quote_id ON public.rac_quote_urls USING btree (quote_id);


--
-- Name: idx_quotes_created_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotes_created_by ON public.rac_quotes USING btree (created_by_id);


--
-- Name: idx_quotes_lead; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotes_lead ON public.rac_quotes USING btree (lead_id);


--
-- Name: idx_quotes_number_org; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_quotes_number_org ON public.rac_quotes USING btree (organization_id, quote_number);


--
-- Name: idx_quotes_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotes_org ON public.rac_quotes USING btree (organization_id);


--
-- Name: idx_quotes_preview_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotes_preview_token ON public.rac_quotes USING btree (preview_token) WHERE (preview_token IS NOT NULL);


--
-- Name: idx_quotes_public_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quotes_public_token ON public.rac_quotes USING btree (public_token) WHERE (public_token IS NOT NULL);


--
-- Name: idx_refresh_tokens_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refresh_tokens_expires ON public.rac_refresh_tokens USING btree (expires_at);


--
-- Name: idx_refresh_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refresh_tokens_user_id ON public.rac_refresh_tokens USING btree (user_id);


--
-- Name: idx_service_types_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_types_active ON public.rac_service_types USING btree (is_active) WHERE (is_active = true);


--
-- Name: idx_service_types_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_types_org ON public.rac_service_types USING btree (organization_id);


--
-- Name: idx_service_types_org_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_service_types_org_name ON public.rac_service_types USING btree (organization_id, name);


--
-- Name: idx_service_types_org_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_service_types_org_slug ON public.rac_service_types USING btree (organization_id, slug);


--
-- Name: idx_service_types_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_types_slug ON public.rac_service_types USING btree (slug);


--
-- Name: idx_timeline_events_cluster; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_timeline_events_cluster ON public.lead_timeline_events USING btree (organization_id, lead_id, event_type, created_at DESC);


--
-- Name: idx_timeline_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_timeline_lookup ON public.lead_timeline_events USING btree (lead_id, created_at DESC);


--
-- Name: idx_unique_partner_service_invite; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_unique_partner_service_invite ON public.partner_invites USING btree (lead_service_id, partner_id);


--
-- Name: idx_user_tokens_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_tokens_expires ON public.rac_user_tokens USING btree (expires_at);


--
-- Name: idx_user_tokens_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_tokens_type ON public.rac_user_tokens USING btree (type);


--
-- Name: idx_user_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_tokens_user_id ON public.rac_user_tokens USING btree (user_id);


--
-- Name: idx_webhook_api_keys_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_api_keys_org ON public.rac_webhook_api_keys USING btree (organization_id);


--
-- Name: idx_webhook_api_keys_prefix; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_api_keys_prefix ON public.rac_webhook_api_keys USING btree (key_prefix);


--
-- Name: rac_appointment_attachments appointment_attachments_appointment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_attachments
    ADD CONSTRAINT appointment_attachments_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES public.rac_appointments(id) ON DELETE CASCADE;


--
-- Name: rac_appointment_attachments appointment_attachments_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_attachments
    ADD CONSTRAINT appointment_attachments_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_appointment_availability_overrides appointment_availability_overrides_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_overrides
    ADD CONSTRAINT appointment_availability_overrides_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_appointment_availability_overrides appointment_availability_overrides_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_overrides
    ADD CONSTRAINT appointment_availability_overrides_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_appointment_availability_rules appointment_availability_rules_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_rules
    ADD CONSTRAINT appointment_availability_rules_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_appointment_availability_rules appointment_availability_rules_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_availability_rules
    ADD CONSTRAINT appointment_availability_rules_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_appointment_visit_reports appointment_visit_reports_appointment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_visit_reports
    ADD CONSTRAINT appointment_visit_reports_appointment_id_fkey FOREIGN KEY (appointment_id) REFERENCES public.rac_appointments(id) ON DELETE CASCADE;


--
-- Name: rac_appointment_visit_reports appointment_visit_reports_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointment_visit_reports
    ADD CONSTRAINT appointment_visit_reports_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_appointments appointments_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointments
    ADD CONSTRAINT appointments_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE SET NULL;


--
-- Name: rac_appointments appointments_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointments
    ADD CONSTRAINT appointments_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE SET NULL;


--
-- Name: rac_appointments appointments_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointments
    ADD CONSTRAINT appointments_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_appointments appointments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_appointments
    ADD CONSTRAINT appointments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_product_assets catalog_product_assets_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_assets
    ADD CONSTRAINT catalog_product_assets_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.rac_catalog_products(id) ON DELETE CASCADE;


--
-- Name: partner_invites fk_organization; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partner_invites
    ADD CONSTRAINT fk_organization FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_lead_photo_analyses lead_photo_analyses_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_photo_analyses
    ADD CONSTRAINT lead_photo_analyses_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_photo_analyses lead_photo_analyses_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_photo_analyses
    ADD CONSTRAINT lead_photo_analyses_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_lead_photo_analyses lead_photo_analyses_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_photo_analyses
    ADD CONSTRAINT lead_photo_analyses_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_lead_service_attachments lead_service_attachments_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_attachments
    ADD CONSTRAINT lead_service_attachments_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_lead_service_attachments lead_service_attachments_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_attachments
    ADD CONSTRAINT lead_service_attachments_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.rac_users(id);


--
-- Name: lead_timeline_events lead_timeline_events_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_timeline_events
    ADD CONSTRAINT lead_timeline_events_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: lead_timeline_events lead_timeline_events_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_timeline_events
    ADD CONSTRAINT lead_timeline_events_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_organization_invites organization_invites_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_invites
    ADD CONSTRAINT organization_invites_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.rac_users(id) ON DELETE RESTRICT;


--
-- Name: rac_organization_invites organization_invites_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_invites
    ADD CONSTRAINT organization_invites_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_organization_invites organization_invites_used_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_invites
    ADD CONSTRAINT organization_invites_used_by_fkey FOREIGN KEY (used_by) REFERENCES public.rac_users(id) ON DELETE RESTRICT;


--
-- Name: rac_organization_members organization_members_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_members
    ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_organization_members organization_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_members
    ADD CONSTRAINT organization_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_organizations organizations_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organizations
    ADD CONSTRAINT organizations_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.rac_users(id) ON DELETE RESTRICT;


--
-- Name: partner_invites partner_invites_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partner_invites
    ADD CONSTRAINT partner_invites_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: partner_invites partner_invites_partner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partner_invites
    ADD CONSTRAINT partner_invites_partner_id_fkey FOREIGN KEY (partner_id) REFERENCES public.rac_partners(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_product_materials rac_catalog_product_materials_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_materials
    ADD CONSTRAINT rac_catalog_product_materials_material_id_fkey FOREIGN KEY (material_id) REFERENCES public.rac_catalog_products(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_product_materials rac_catalog_product_materials_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_materials
    ADD CONSTRAINT rac_catalog_product_materials_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_product_materials rac_catalog_product_materials_product_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_product_materials
    ADD CONSTRAINT rac_catalog_product_materials_product_id_fkey FOREIGN KEY (product_id) REFERENCES public.rac_catalog_products(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_products rac_catalog_products_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_products
    ADD CONSTRAINT rac_catalog_products_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_catalog_products rac_catalog_products_vat_rate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_products
    ADD CONSTRAINT rac_catalog_products_vat_rate_id_fkey FOREIGN KEY (vat_rate_id) REFERENCES public.rac_catalog_vat_rates(id) ON DELETE RESTRICT;


--
-- Name: rac_catalog_vat_rates rac_catalog_vat_rates_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_catalog_vat_rates
    ADD CONSTRAINT rac_catalog_vat_rates_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_export_api_keys rac_export_api_keys_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_export_api_keys
    ADD CONSTRAINT rac_export_api_keys_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.rac_users(id);


--
-- Name: rac_export_api_keys rac_export_api_keys_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_export_api_keys
    ADD CONSTRAINT rac_export_api_keys_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_feed_comment_mentions rac_feed_comment_mentions_comment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comment_mentions
    ADD CONSTRAINT rac_feed_comment_mentions_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES public.rac_feed_comments(id) ON DELETE CASCADE;


--
-- Name: rac_feed_comment_mentions rac_feed_comment_mentions_mentioned_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comment_mentions
    ADD CONSTRAINT rac_feed_comment_mentions_mentioned_user_id_fkey FOREIGN KEY (mentioned_user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_feed_comments rac_feed_comments_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comments
    ADD CONSTRAINT rac_feed_comments_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_feed_comments rac_feed_comments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_comments
    ADD CONSTRAINT rac_feed_comments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_feed_pinned_alerts rac_feed_pinned_alerts_dismissed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_pinned_alerts
    ADD CONSTRAINT rac_feed_pinned_alerts_dismissed_by_fkey FOREIGN KEY (dismissed_by) REFERENCES public.rac_users(id);


--
-- Name: rac_feed_pinned_alerts rac_feed_pinned_alerts_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_pinned_alerts
    ADD CONSTRAINT rac_feed_pinned_alerts_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_feed_pinned_alerts rac_feed_pinned_alerts_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_pinned_alerts
    ADD CONSTRAINT rac_feed_pinned_alerts_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_feed_reactions rac_feed_reactions_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_reactions
    ADD CONSTRAINT rac_feed_reactions_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_feed_reactions rac_feed_reactions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_reactions
    ADD CONSTRAINT rac_feed_reactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_feed_read_state rac_feed_read_state_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_read_state
    ADD CONSTRAINT rac_feed_read_state_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_feed_read_state rac_feed_read_state_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_feed_read_state
    ADD CONSTRAINT rac_feed_read_state_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_google_ads_exports rac_google_ads_exports_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_ads_exports
    ADD CONSTRAINT rac_google_ads_exports_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_google_ads_exports rac_google_ads_exports_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_ads_exports
    ADD CONSTRAINT rac_google_ads_exports_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_google_ads_exports rac_google_ads_exports_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_ads_exports
    ADD CONSTRAINT rac_google_ads_exports_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_google_lead_ids rac_google_lead_ids_lead_uuid_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_lead_ids
    ADD CONSTRAINT rac_google_lead_ids_lead_uuid_fkey FOREIGN KEY (lead_uuid) REFERENCES public.rac_leads(id) ON DELETE SET NULL;


--
-- Name: rac_google_lead_ids rac_google_lead_ids_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_lead_ids
    ADD CONSTRAINT rac_google_lead_ids_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_google_webhook_configs rac_google_webhook_configs_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_google_webhook_configs
    ADD CONSTRAINT rac_google_webhook_configs_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_lead_activity rac_lead_activity_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_activity
    ADD CONSTRAINT rac_lead_activity_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_activity rac_lead_activity_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_activity
    ADD CONSTRAINT rac_lead_activity_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_lead_activity rac_lead_activity_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_activity
    ADD CONSTRAINT rac_lead_activity_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_lead_ai_analysis rac_lead_ai_analysis_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_ai_analysis
    ADD CONSTRAINT rac_lead_ai_analysis_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_ai_analysis rac_lead_ai_analysis_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_ai_analysis
    ADD CONSTRAINT rac_lead_ai_analysis_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_lead_ai_analysis rac_lead_ai_analysis_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_ai_analysis
    ADD CONSTRAINT rac_lead_ai_analysis_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_lead_notes rac_lead_notes_author_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_notes
    ADD CONSTRAINT rac_lead_notes_author_id_fkey FOREIGN KEY (author_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_lead_notes rac_lead_notes_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_notes
    ADD CONSTRAINT rac_lead_notes_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_notes rac_lead_notes_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_notes
    ADD CONSTRAINT rac_lead_notes_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_lead_notes rac_lead_notes_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_notes
    ADD CONSTRAINT rac_lead_notes_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_lead_service_events rac_lead_service_events_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_events
    ADD CONSTRAINT rac_lead_service_events_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_service_events rac_lead_service_events_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_events
    ADD CONSTRAINT rac_lead_service_events_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_lead_service_events rac_lead_service_events_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_service_events
    ADD CONSTRAINT rac_lead_service_events_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_lead_services rac_lead_services_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_services
    ADD CONSTRAINT rac_lead_services_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_lead_services rac_lead_services_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_services
    ADD CONSTRAINT rac_lead_services_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_lead_services rac_lead_services_service_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_lead_services
    ADD CONSTRAINT rac_lead_services_service_type_id_fkey FOREIGN KEY (service_type_id) REFERENCES public.rac_service_types(id);


--
-- Name: rac_leads rac_leads_assigned_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_leads
    ADD CONSTRAINT rac_leads_assigned_agent_id_fkey FOREIGN KEY (assigned_agent_id) REFERENCES public.rac_users(id) ON DELETE SET NULL;


--
-- Name: rac_leads rac_leads_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_leads
    ADD CONSTRAINT rac_leads_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_leads rac_leads_viewed_by_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_leads
    ADD CONSTRAINT rac_leads_viewed_by_id_fkey FOREIGN KEY (viewed_by_id) REFERENCES public.rac_users(id) ON DELETE SET NULL;


--
-- Name: rac_organization_settings rac_organization_settings_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_organization_settings
    ADD CONSTRAINT rac_organization_settings_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_partner_invites rac_partner_invites_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.rac_users(id) ON DELETE RESTRICT;


--
-- Name: rac_partner_invites rac_partner_invites_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE SET NULL;


--
-- Name: rac_partner_invites rac_partner_invites_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE SET NULL;


--
-- Name: rac_partner_invites rac_partner_invites_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_partner_invites rac_partner_invites_partner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_partner_id_fkey FOREIGN KEY (partner_id) REFERENCES public.rac_partners(id) ON DELETE CASCADE;


--
-- Name: rac_partner_invites rac_partner_invites_used_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_invites
    ADD CONSTRAINT rac_partner_invites_used_by_fkey FOREIGN KEY (used_by) REFERENCES public.rac_users(id) ON DELETE RESTRICT;


--
-- Name: rac_partner_leads rac_partner_leads_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_leads
    ADD CONSTRAINT rac_partner_leads_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_partner_leads rac_partner_leads_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_leads
    ADD CONSTRAINT rac_partner_leads_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_partner_leads rac_partner_leads_partner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_leads
    ADD CONSTRAINT rac_partner_leads_partner_id_fkey FOREIGN KEY (partner_id) REFERENCES public.rac_partners(id) ON DELETE CASCADE;


--
-- Name: rac_partner_offers rac_partner_offers_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_offers
    ADD CONSTRAINT rac_partner_offers_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE CASCADE;


--
-- Name: rac_partner_offers rac_partner_offers_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_offers
    ADD CONSTRAINT rac_partner_offers_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_partner_offers rac_partner_offers_partner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_offers
    ADD CONSTRAINT rac_partner_offers_partner_id_fkey FOREIGN KEY (partner_id) REFERENCES public.rac_partners(id) ON DELETE CASCADE;


--
-- Name: rac_partner_service_types rac_partner_service_types_partner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_service_types
    ADD CONSTRAINT rac_partner_service_types_partner_id_fkey FOREIGN KEY (partner_id) REFERENCES public.rac_partners(id) ON DELETE CASCADE;


--
-- Name: rac_partner_service_types rac_partner_service_types_service_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partner_service_types
    ADD CONSTRAINT rac_partner_service_types_service_type_id_fkey FOREIGN KEY (service_type_id) REFERENCES public.rac_service_types(id) ON DELETE RESTRICT;


--
-- Name: rac_partners rac_partners_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_partners
    ADD CONSTRAINT rac_partners_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_quote_activity rac_quote_activity_quote_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_activity
    ADD CONSTRAINT rac_quote_activity_quote_id_fkey FOREIGN KEY (quote_id) REFERENCES public.rac_quotes(id) ON DELETE CASCADE;


--
-- Name: rac_quote_annotations rac_quote_annotations_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_annotations
    ADD CONSTRAINT rac_quote_annotations_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_quote_annotations rac_quote_annotations_quote_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_annotations
    ADD CONSTRAINT rac_quote_annotations_quote_item_id_fkey FOREIGN KEY (quote_item_id) REFERENCES public.rac_quote_items(id) ON DELETE CASCADE;


--
-- Name: rac_quote_attachments rac_quote_attachments_quote_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_attachments
    ADD CONSTRAINT rac_quote_attachments_quote_id_fkey FOREIGN KEY (quote_id) REFERENCES public.rac_quotes(id) ON DELETE CASCADE;


--
-- Name: rac_quote_counters rac_quote_counters_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_counters
    ADD CONSTRAINT rac_quote_counters_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_quote_items rac_quote_items_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_items
    ADD CONSTRAINT rac_quote_items_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_quote_items rac_quote_items_quote_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_items
    ADD CONSTRAINT rac_quote_items_quote_id_fkey FOREIGN KEY (quote_id) REFERENCES public.rac_quotes(id) ON DELETE CASCADE;


--
-- Name: rac_quote_urls rac_quote_urls_quote_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quote_urls
    ADD CONSTRAINT rac_quote_urls_quote_id_fkey FOREIGN KEY (quote_id) REFERENCES public.rac_quotes(id) ON DELETE CASCADE;


--
-- Name: rac_quotes rac_quotes_created_by_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_created_by_id_fkey FOREIGN KEY (created_by_id) REFERENCES public.rac_users(id) ON DELETE SET NULL;


--
-- Name: rac_quotes rac_quotes_lead_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES public.rac_leads(id) ON DELETE CASCADE;


--
-- Name: rac_quotes rac_quotes_lead_service_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_lead_service_id_fkey FOREIGN KEY (lead_service_id) REFERENCES public.rac_lead_services(id) ON DELETE SET NULL;


--
-- Name: rac_quotes rac_quotes_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_quotes
    ADD CONSTRAINT rac_quotes_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- Name: rac_refresh_tokens rac_refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_refresh_tokens
    ADD CONSTRAINT rac_refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_service_types rac_service_types_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_service_types
    ADD CONSTRAINT rac_service_types_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id);


--
-- Name: rac_user_roles rac_user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_roles
    ADD CONSTRAINT rac_user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.rac_roles(id) ON DELETE CASCADE;


--
-- Name: rac_user_roles rac_user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_roles
    ADD CONSTRAINT rac_user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_user_settings rac_user_settings_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_settings
    ADD CONSTRAINT rac_user_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_user_tokens rac_user_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_user_tokens
    ADD CONSTRAINT rac_user_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.rac_users(id) ON DELETE CASCADE;


--
-- Name: rac_webhook_api_keys rac_webhook_api_keys_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rac_webhook_api_keys
    ADD CONSTRAINT rac_webhook_api_keys_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.rac_organizations(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict PclGHKpeDjdAKzBAEeRKQ7fzUJf12jv7pYMGseckg4mTiVvfVTryyuOBeSzDQi2

