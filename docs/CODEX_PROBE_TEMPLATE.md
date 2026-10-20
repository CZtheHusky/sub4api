# Codex degradation check probe template

Since 1.2.0 ticket harvesting has been removed. This JSONL initial context is
now used exclusively by Codex degradation (model intelligence) checks — both the
manual multi-model diagnostic and the automatic background check. Edit
**Codex degradation check template** in the administrator's gateway settings and
save the page. **Restore default** loads the built-in template into the editor;
save to apply it.

The [default JSONL](../backend/internal/service/codex_probe_template.jsonl) contains
English static instructions and anonymous paths. Memory and skill text contains
no personal history. Paths are prompt text only. The production service does not
read a local Codex recording or installation file.

Keep the seven records, five message roles and content segmentation, prompt
wrappers and environment XML hierarchy and attributes. The environment has three
workspace roots and eighteen permission entries. Only initial developer/user
`input_text` messages are accepted; assistant output, tools, reasoning and extra
recording metadata are rejected. Templates can be at most 256 KiB.

| Placeholder | Value |
| --- | --- |
| `{{TIMEZONE}}` | Target account's request timezone; default `Asia/Singapore` |
| `{{CURRENT_DATE}}` | Request start date in that timezone, `YYYY-MM-DD` |
| `{{MODEL}}` | Target model |
| `{{MODELTRACE_PROMPT}}` | Fresh ModelTrace challenge for this actual request |

The challenge placeholder must occur exactly once, as the entire final user
message. Challenges retain the existing Chinese instructions and expected-count
validation. Replacement happens after parsing, followed by JSON encoding, so
quotes and newlines remain valid. Unknown placeholders are rejected with a line
number and a reason that does not include template text.

Administrator settings GET and PUT use `openai_codex_ticket_prompt_template`
(the key name predates the removal of ticket harvesting and is kept for
compatibility). GET returns the effective template and the read-only
`openai_codex_ticket_prompt_template_default`. Omit the PUT field to keep its
stored value; send an empty string to use the built-in default. Invalid templates
are rejected before saving. The saving process invalidates its cache immediately;
other instances refresh within the five-second cache lifetime. Each diagnostic
model pins one snapshot for its check. The next model can
load newer settings. Audits record the changed field name, never the template.
Public settings do not expose either template field.

Initial installation identity is a stable UUIDv4 derived from the existing account
credential namespace. Each request creates fresh UUIDv7 session/thread, window and
turn identities shared by headers and body. Diagnostics enter the normal
`/v1/responses` gateway as official Codex client requests with a process-internal
diagnostic token, the pinned target account and normal account isolation and
fingerprint handling — they automatically inherit every fidelity enhancement of
the gateway forwarding path, and consume no API key quota or billing records.
Gateway errors produce a failed check. Invalid runtime templates
produce `template_invalid` and skip the model request; this is a configuration
failure, not a model degradation result.

Local tests verify configuration and request behavior with simulated upstream
responses. Actual diagnostic effectiveness requires real-account experiments.
