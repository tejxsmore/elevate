CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE call_status AS ENUM ('queued','dialing','ringing','in_progress','completed','failed','no_answer','busy','canceled');
CREATE TYPE call_direction AS ENUM ('outbound','inbound');
CREATE TYPE language_code AS ENUM ('en','hi','te','mixed','unknown');
CREATE TYPE speaker_role AS ENUM ('agent','lead');
CREATE TYPE message_role AS ENUM ('system','assistant','user','tool');
CREATE TYPE classification_label AS ENUM ('hot','warm','cold','unclassified');
CREATE TYPE barrier_type AS ENUM ('budget','timing','decision_maker','trust','other');
CREATE TYPE action_type AS ENUM ('whatsapp_mid_call','whatsapp_followup','whatsapp_brochure','whatsapp_resume','schedule_callback','place_callback_call','update_classification','end_call','transfer_call');
CREATE TYPE action_status AS ENUM ('pending','executing','completed','failed','skipped');
CREATE TYPE action_trigger AS ENUM ('intent_detected','call_ended','user_requested_time','manual','scheduled_job');
CREATE TYPE callback_status AS ENUM ('needs_confirmation','scheduled','completed','missed','canceled','rescheduled');
CREATE TYPE whatsapp_message_type AS ENUM ('mid_call_intent','post_call_followup','resume_send','brochure','callback_confirmation','reminder');
CREATE TYPE whatsapp_status AS ENUM ('queued','sent','delivered','read','failed','undelivered');
CREATE TYPE latency_stage AS ENUM ('stt_partial','stt_final','llm_first_token','llm_complete','tts_first_byte','tts_complete','full_turn');

CREATE TABLE leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_e164 TEXT NOT NULL UNIQUE CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    name TEXT,
    preferred_language language_code NOT NULL DEFAULT 'unknown',
    source TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    storage_provider TEXT NOT NULL DEFAULT 'supabase',
    storage_path TEXT NOT NULL,
    mime_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    system_prompt TEXT,
    voice_config JSONB NOT NULL DEFAULT '{}',
    whatsapp_templates JSONB NOT NULL DEFAULT '{}',
    default_resume_asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
    default_diagram_asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
    agent_phone_number TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    version INT NOT NULL DEFAULT 1,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_campaigns_active ON campaigns(id) WHERE active AND archived_at IS NULL;

CREATE TABLE campaign_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id),
    version INT NOT NULL,
    system_prompt TEXT,
    voice_config JSONB NOT NULL DEFAULT '{}',
    whatsapp_templates JSONB NOT NULL DEFAULT '{}',
    default_resume_asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
    default_diagram_asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
    agent_phone_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, version)
);

CREATE TABLE calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    campaign_id UUID,
    campaign_version INT,
    parent_call_id UUID REFERENCES calls(id) ON DELETE SET NULL,
    provider TEXT NOT NULL DEFAULT 'twilio',
    provider_call_id TEXT,
    direction call_direction NOT NULL DEFAULT 'outbound',
    status call_status NOT NULL DEFAULT 'queued',
    attempt_number INT NOT NULL DEFAULT 1,
    primary_language language_code NOT NULL DEFAULT 'unknown',
    current_classification classification_label NOT NULL DEFAULT 'unclassified',
    classification_confidence NUMERIC(4,3) CHECK (classification_confidence IS NULL OR classification_confidence BETWEEN 0 AND 1),
    classification_sequence_number INT,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    scheduled_for TIMESTAMPTZ,
    dialed_at TIMESTAMPTZ,
    answered_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    duration_seconds INT CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    ended_reason TEXT,
    failure_code TEXT,
    provider_error_code TEXT,
    retry_after TIMESTAMPTZ,
    recording_url TEXT,
    recording_sid TEXT,
    twiml_stream_sid TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (answered_at IS NULL OR dialed_at IS NULL OR answered_at >= dialed_at),
    CHECK (ended_at IS NULL OR answered_at IS NULL OR ended_at >= answered_at),
    CHECK ((campaign_id IS NULL) = (campaign_version IS NULL)),
    CHECK (attempt_number > 0),
    FOREIGN KEY (campaign_id, campaign_version) REFERENCES campaign_versions(campaign_id, version) ON DELETE SET NULL
);

CREATE UNIQUE INDEX uq_calls_id_lead ON calls(id, lead_id);
CREATE UNIQUE INDEX uq_calls_provider_call_id ON calls(provider, provider_call_id) WHERE provider_call_id IS NOT NULL;
CREATE INDEX idx_calls_lead_id ON calls(lead_id);
CREATE INDEX idx_calls_campaign_id ON calls(campaign_id);
CREATE INDEX idx_calls_status ON calls(status);
CREATE INDEX idx_calls_created_at_desc ON calls(created_at DESC);
CREATE INDEX idx_calls_active ON calls(created_at DESC) WHERE status IN ('queued','dialing','ringing','in_progress');

CREATE TABLE lead_profiles (
    lead_id UUID PRIMARY KEY REFERENCES leads(id) ON DELETE CASCADE,
    business_niche TEXT,
    products_sold TEXT,
    product_count_estimate TEXT,
    budget_min NUMERIC(12,2),
    budget_max NUMERIC(12,2),
    currency TEXT NOT NULL DEFAULT 'INR',
    timeline_text TEXT,
    features_requested JSONB NOT NULL DEFAULT '[]',
    last_updated_call_id UUID REFERENCES calls(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (budget_min IS NULL OR budget_max IS NULL OR budget_min <= budget_max)
);

CREATE TABLE lead_communication_preferences (
    lead_id UUID PRIMARY KEY REFERENCES leads(id) ON DELETE CASCADE,
    calls_allowed BOOLEAN NOT NULL DEFAULT true,
    whatsapp_allowed BOOLEAN NOT NULL DEFAULT true,
    do_not_contact BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE conversation_turns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    sequence_number INT NOT NULL CHECK (sequence_number >= 0),
    extracted_context JSONB NOT NULL DEFAULT '{}',
    model TEXT,
    provider_request_id TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    latency_ms INT CHECK (latency_ms IS NULL OR latency_ms >= 0),
    UNIQUE (call_id, sequence_number)
);

CREATE UNIQUE INDEX uq_turns_id_call ON conversation_turns(id, call_id);
CREATE INDEX idx_turns_call_id ON conversation_turns(call_id);

CREATE TABLE call_transcript_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    turn_id UUID,
    segment_sequence INT NOT NULL CHECK (segment_sequence >= 0),
    revision INT NOT NULL DEFAULT 0,
    stt_provider TEXT NOT NULL DEFAULT 'deepgram',
    provider_segment_id TEXT,
    speaker speaker_role NOT NULL,
    text TEXT NOT NULL,
    language_detected language_code NOT NULL DEFAULT 'unknown',
    detected_languages TEXT[] NOT NULL DEFAULT '{}',
    confidence NUMERIC(4,3) CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    is_final BOOLEAN NOT NULL DEFAULT false,
    is_interrupted BOOLEAN NOT NULL DEFAULT false,
    started_at_ms INT,
    ended_at_ms INT,
    provider_request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (call_id, segment_sequence, revision),
    CHECK (ended_at_ms IS NULL OR started_at_ms IS NULL OR ended_at_ms >= started_at_ms),
    FOREIGN KEY (turn_id, call_id) REFERENCES conversation_turns(id, call_id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX uq_segments_id_call ON call_transcript_segments(id, call_id);
CREATE INDEX idx_transcript_turn_id ON call_transcript_segments(turn_id);
CREATE INDEX idx_transcript_text_trgm ON call_transcript_segments USING gin (text gin_trgm_ops);
CREATE UNIQUE INDEX uq_transcript_provider_revision ON call_transcript_segments(call_id, provider_segment_id, revision) WHERE provider_segment_id IS NOT NULL;

CREATE TABLE call_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    turn_id UUID,
    sequence_number INT NOT NULL CHECK (sequence_number >= 0),
    role message_role NOT NULL,
    content TEXT NOT NULL,
    input JSONB,
    output JSONB,
    tool_name TEXT,
    tool_call_id TEXT,
    finish_reason TEXT,
    provider_request_id TEXT,
    model TEXT,
    prompt_tokens INT,
    completion_tokens INT,
    latency_ms INT CHECK (latency_ms IS NULL OR latency_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (call_id, sequence_number),
    FOREIGN KEY (turn_id, call_id) REFERENCES conversation_turns(id, call_id) ON DELETE SET NULL
);

CREATE INDEX idx_messages_call_id ON call_messages(call_id);
CREATE INDEX idx_messages_turn_id ON call_messages(turn_id);

CREATE TABLE discovery_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL UNIQUE REFERENCES calls(id) ON DELETE CASCADE,
    business_niche TEXT,
    products_sold TEXT,
    product_count_estimate TEXT,
    budget_range TEXT,
    budget_raw_text TEXT,
    timeline TEXT,
    timeline_raw_text TEXT,
    features_requested JSONB NOT NULL DEFAULT '[]',
    extra_notes TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE lead_barriers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    barrier_type barrier_type NOT NULL,
    detail TEXT,
    raw_quote TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_barriers_call_id ON lead_barriers(call_id);

CREATE TABLE lead_classifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    sequence_number INT NOT NULL CHECK (sequence_number >= 0),
    classification classification_label NOT NULL,
    confidence NUMERIC(4,3) CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    classification_summary TEXT,
    signals JSONB NOT NULL DEFAULT '{}',
    triggering_segment_id UUID,
    classified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (call_id, sequence_number),
    FOREIGN KEY (triggering_segment_id, call_id) REFERENCES call_transcript_segments(id, call_id) ON DELETE SET NULL
);

CREATE INDEX idx_classifications_call_id ON lead_classifications(call_id);
CREATE INDEX idx_classifications_call_seq_desc ON lead_classifications(call_id, sequence_number DESC);

CREATE TABLE call_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    action_type action_type NOT NULL,
    status action_status NOT NULL DEFAULT 'pending',
    trigger action_trigger NOT NULL,
    trigger_segment_id UUID,
    payload JSONB NOT NULL DEFAULT '{}',
    idempotency_key TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    lock_token UUID,
    lock_expires_at TIMESTAMPTZ,
    priority SMALLINT NOT NULL DEFAULT 100,
    last_error JSONB,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    latency_ms INT CHECK (latency_ms IS NULL OR latency_ms >= 0),
    error_message TEXT,
    CHECK (attempt_count <= max_attempts),
    CHECK (max_attempts > 0),
    CHECK (priority >= 0),
    CHECK (status NOT IN ('completed','failed','skipped') OR (locked_at IS NULL AND locked_by IS NULL AND lock_token IS NULL AND lock_expires_at IS NULL)),
    FOREIGN KEY (trigger_segment_id, call_id) REFERENCES call_transcript_segments(id, call_id) ON DELETE SET NULL
);

CREATE INDEX idx_actions_call_id ON call_actions(call_id);
CREATE INDEX idx_actions_status ON call_actions(status);
CREATE UNIQUE INDEX uq_actions_idempotency ON call_actions(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_actions_once_per_call ON call_actions(call_id, action_type) WHERE action_type IN ('whatsapp_mid_call','whatsapp_resume');
CREATE INDEX idx_actions_ready ON call_actions(available_at, priority) WHERE status = 'pending';
CREATE INDEX idx_actions_lock_expiry ON call_actions(lock_expires_at) WHERE status = 'executing';

CREATE TABLE scheduled_callbacks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    requested_time_text TEXT NOT NULL,
    scheduled_for TIMESTAMPTZ,
    timezone TEXT NOT NULL DEFAULT 'Asia/Kolkata',
    resolution_confidence NUMERIC(4,3) CHECK (resolution_confidence IS NULL OR resolution_confidence BETWEEN 0 AND 1),
    resolution_source TEXT,
    resolved_from JSONB NOT NULL DEFAULT '{}',
    status callback_status NOT NULL DEFAULT 'needs_confirmation',
    reminder_sent BOOLEAN NOT NULL DEFAULT false,
    callback_action_id UUID REFERENCES call_actions(id) ON DELETE SET NULL,
    follow_up_call_id UUID REFERENCES calls(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status <> 'scheduled' OR scheduled_for IS NOT NULL),
    FOREIGN KEY (call_id, lead_id) REFERENCES calls(id, lead_id)
);

CREATE INDEX idx_callbacks_lead_id ON scheduled_callbacks(lead_id);
CREATE INDEX idx_callbacks_due ON scheduled_callbacks(scheduled_for) WHERE status = 'scheduled';

CREATE TABLE whatsapp_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID,
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    action_id UUID REFERENCES call_actions(id) ON DELETE SET NULL,
    message_type whatsapp_message_type NOT NULL,
    provider TEXT NOT NULL DEFAULT 'twilio',
    to_number TEXT NOT NULL,
    body TEXT NOT NULL,
    idempotency_key TEXT,
    provider_message_id TEXT,
    status whatsapp_status NOT NULL DEFAULT 'queued',
    sent_during_call BOOLEAN NOT NULL DEFAULT false,
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (call_id, lead_id) REFERENCES calls(id, lead_id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX uq_wa_provider_message_id ON whatsapp_messages(provider, provider_message_id) WHERE provider_message_id IS NOT NULL;
CREATE INDEX idx_wa_call_id ON whatsapp_messages(call_id);
CREATE INDEX idx_wa_lead_id ON whatsapp_messages(lead_id);
CREATE INDEX idx_wa_status ON whatsapp_messages(status);
CREATE UNIQUE INDEX uq_wa_idempotency ON whatsapp_messages(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE whatsapp_message_assets (
    whatsapp_message_id UUID NOT NULL REFERENCES whatsapp_messages(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    PRIMARY KEY (whatsapp_message_id, asset_id)
);

CREATE TABLE call_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_data JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_call_id ON call_events(call_id, occurred_at);

CREATE TABLE latency_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    turn_sequence_number INT,
    stage latency_stage NOT NULL,
    duration_ms INT NOT NULL CHECK (duration_ms >= 0),
    measured_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_latency_call_id ON latency_metrics(call_id, stage);

CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NOT NULL,
    provider_event_id TEXT,
    payload_hash TEXT,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    signature_valid BOOLEAN,
    related_call_id UUID REFERENCES calls(id) ON DELETE SET NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    lock_token UUID,
    lock_expires_at TIMESTAMPTZ,
    CHECK (attempt_count <= max_attempts),
    CHECK (max_attempts > 0)
);

CREATE UNIQUE INDEX uq_webhook_provider_event ON webhook_events(provider, provider_event_id) WHERE provider_event_id IS NOT NULL;
CREATE UNIQUE INDEX uq_webhook_provider_hash ON webhook_events(provider, payload_hash) WHERE provider_event_id IS NULL AND payload_hash IS NOT NULL;
CREATE INDEX idx_webhooks_status ON webhook_events(status) WHERE status = 'pending';
CREATE INDEX idx_webhooks_ready ON webhook_events(next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_webhooks_lock_expiry ON webhook_events(lock_expires_at) WHERE status = 'processing';
CREATE INDEX idx_webhooks_call_id ON webhook_events(related_call_id);

CREATE TABLE api_usage_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id UUID REFERENCES calls(id) ON DELETE SET NULL,
    provider TEXT NOT NULL,
    operation TEXT NOT NULL,
    request_id UUID,
    provider_request_id TEXT,
    trace_id UUID,
    units_consumed NUMERIC(12,4),
    unit_type TEXT,
    cost_usd NUMERIC(12,6),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_usage_call_id ON api_usage_log(call_id);
CREATE INDEX idx_api_usage_provider ON api_usage_log(provider);
CREATE INDEX idx_api_usage_trace ON api_usage_log(trace_id);

CREATE TABLE system_settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_leads_updated_at BEFORE UPDATE ON leads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_campaigns_updated_at BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_calls_updated_at BEFORE UPDATE ON calls
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_lead_profiles_updated_at BEFORE UPDATE ON lead_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_comm_prefs_updated_at BEFORE UPDATE ON lead_communication_preferences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_callbacks_updated_at BEFORE UPDATE ON scheduled_callbacks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE FUNCTION sync_call_classification()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE calls
    SET current_classification = NEW.classification,
        classification_confidence = NEW.confidence,
        classification_sequence_number = NEW.sequence_number,
        updated_at = now()
    WHERE id = NEW.call_id
      AND (classification_sequence_number IS NULL OR NEW.sequence_number >= classification_sequence_number);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_classification AFTER INSERT ON lead_classifications
    FOR EACH ROW EXECUTE FUNCTION sync_call_classification();

CREATE OR REPLACE FUNCTION create_default_lead_rows()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO lead_profiles (lead_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
    INSERT INTO lead_communication_preferences (lead_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_leads_default_rows AFTER INSERT ON leads
    FOR EACH ROW EXECUTE FUNCTION create_default_lead_rows();

CREATE OR REPLACE FUNCTION snapshot_campaign_version()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.system_prompt IS DISTINCT FROM OLD.system_prompt
       OR NEW.voice_config IS DISTINCT FROM OLD.voice_config
       OR NEW.whatsapp_templates IS DISTINCT FROM OLD.whatsapp_templates
       OR NEW.default_resume_asset_id IS DISTINCT FROM OLD.default_resume_asset_id
       OR NEW.default_diagram_asset_id IS DISTINCT FROM OLD.default_diagram_asset_id
       OR NEW.agent_phone_number IS DISTINCT FROM OLD.agent_phone_number
    THEN
        NEW.version := OLD.version + 1;
        INSERT INTO campaign_versions (
            campaign_id, version, system_prompt, voice_config, whatsapp_templates,
            default_resume_asset_id, default_diagram_asset_id, agent_phone_number
        ) VALUES (
            NEW.id, NEW.version, NEW.system_prompt, NEW.voice_config, NEW.whatsapp_templates,
            NEW.default_resume_asset_id, NEW.default_diagram_asset_id, NEW.agent_phone_number
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_campaign_versioning BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION snapshot_campaign_version();

CREATE OR REPLACE FUNCTION block_campaign_version_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'campaign_versions is append-only; % not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_campaign_versions_immutable
    BEFORE UPDATE OR DELETE ON campaign_versions
    FOR EACH ROW EXECUTE FUNCTION block_campaign_version_mutation();

CREATE OR REPLACE FUNCTION seed_initial_campaign_version()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO campaign_versions (
        campaign_id, version, system_prompt, voice_config, whatsapp_templates,
        default_resume_asset_id, default_diagram_asset_id, agent_phone_number
    ) VALUES (
        NEW.id, NEW.version, NEW.system_prompt, NEW.voice_config, NEW.whatsapp_templates,
        NEW.default_resume_asset_id, NEW.default_diagram_asset_id, NEW.agent_phone_number
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_campaign_initial_version AFTER INSERT ON campaigns
    FOR EACH ROW EXECUTE FUNCTION seed_initial_campaign_version();

CREATE OR REPLACE FUNCTION set_call_campaign_version()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.campaign_version IS NULL AND NEW.campaign_id IS NOT NULL THEN
        SELECT version INTO NEW.campaign_version FROM campaigns WHERE id = NEW.campaign_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_calls_campaign_version BEFORE INSERT ON calls
    FOR EACH ROW EXECUTE FUNCTION set_call_campaign_version();

CREATE OR REPLACE FUNCTION clear_action_lock_on_finish()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('completed','failed','skipped') THEN
        NEW.locked_at := NULL;
        NEW.locked_by := NULL;
        NEW.lock_token := NULL;
        NEW.lock_expires_at := NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_actions_clear_lock BEFORE UPDATE ON call_actions
    FOR EACH ROW EXECUTE FUNCTION clear_action_lock_on_finish();

CREATE OR REPLACE FUNCTION clear_webhook_lock_on_finish()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('processed','failed','skipped') THEN
        NEW.locked_at := NULL;
        NEW.locked_by := NULL;
        NEW.lock_token := NULL;
        NEW.lock_expires_at := NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_webhooks_clear_lock BEFORE UPDATE ON webhook_events
    FOR EACH ROW EXECUTE FUNCTION clear_webhook_lock_on_finish();

INSERT INTO campaigns (id, name, active) VALUES
    ('00000000-0000-0000-0000-000000000001', 'default', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO system_settings (key, value) VALUES
    ('default_campaign_id', '"00000000-0000-0000-0000-000000000001"'),
    ('whatsapp_business_number', '""'),
    ('target_test_number', '"+918688664337"')
ON CONFLICT (key) DO NOTHING;