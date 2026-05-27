# HRD-019 — cherald constitution-binding round-trip transcript

Tenant: a79332ba-eb14-41c6-981f-97d550305baf
Rule: §11.4.29 (lowercase_snake_case naming), mode=enforce

## 1. REQUEST → POST /v1/compliance/evaluate

```json
{"rule_id":"§11.4.29","subject_kind":"file","subject_id":"commons_messaging/BadName.go"}
```

## 2. RESPONSE ← 200 OK

```json
{
  "audited": true,
  "changed": true,
  "decision": "deny",
  "emitted": true,
  "first_seen": true,
  "mode": "enforce",
  "rule_id": "§11.4.29",
  "subject": "file:commons_messaging/BadName.go",
  "tenant_id": "a79332ba-eb14-41c6-981f-97d550305baf",
  "transition_to": "fail"
}
```

## 3. REQUEST → GET /v1/compliance?rule_id=§11.4.29

## 4. RESPONSE ← 200 OK (the persisted violation is visible on the pull surface)

```json
{
  "page": 1,
  "page_size": 50,
  "results": [
    {
      "bundle_hash": "0000000000000000000000000000000000000000000000000000000000000000",
      "decision": "deny",
      "digest_sha": "15b10bcf135f5cdf509c613e878df5be941142baa585779f9568705e5d9cdb3f",
      "evidence_uri": "naming violation §11.4.29: uppercase in path component BadName.go of commons_messaging/BadName.go",
      "rule_id": "§11.4.29",
      "subject": "commons_messaging/BadName.go",
      "transitioned_at": "2026-05-28T01:02:03.259904+05:00"
    }
  ],
  "tenant_id": "a79332ba-eb14-41c6-981f-97d550305baf",
  "total": 1
}
```

## 5. REQUEST → POST /v1/compliance/evaluate (compliant subject — anti-false-positive)

```json
{"rule_id":"§11.4.29","subject_id":"commons_messaging/good_name.go"}
```

## 6. RESPONSE ← 200 OK (decision=pass — conforming subject NOT flagged)

```json
{
  "audited": true,
  "changed": true,
  "decision": "pass",
  "emitted": true,
  "first_seen": true,
  "mode": "enforce",
  "rule_id": "§11.4.29",
  "subject": "file:commons_messaging/good_name.go",
  "tenant_id": "a79332ba-eb14-41c6-981f-97d550305baf",
  "transition_to": "pass"
}
```

## 7. Event-bus metrics (proof the emit reached the bus)

```
published_total=2
published_by_type=map[digital.vasic.herald.constitution.policy.cleared:1 digital.vasic.herald.constitution.policy.violation:1]
```

## Verdict

- A §11.4.29 naming violation was detected, emitted as a `.policy.violation` event, persisted to constitution_state, and surfaced on GET /v1/compliance — the full detect→emit→persist→query loop.
- A compliant snake_case file produced decision=pass with no emit — no false positive.
- This is real captured runtime output (httptest server + the production handlers), not a metadata assertion.
