-- 1.2.0: Codex 打票（ticket harvesting）功能整体移除后的一次性清理。
-- 保留：codex_ticket_attempts（含分区，改作降智检测历史表）与
-- openai_codex_ticket_prompt_template 设置键（降智检测探测模板仍使用）。

-- 纯打票遗留事件表：1.2.0 起无任何写入方。
DROP TABLE IF EXISTS codex_ticket_invalidations;

-- 打票设置键（prompt_template 保留）。
DELETE FROM settings
WHERE key IN (
    'openai_codex_ticket_enabled',
    'openai_codex_ticket_allow_without_ticket',
    'openai_codex_ticket_harvest_proxy_url',
    'openai_codex_ticket_proxy_pool'
);

-- accounts.extra 打票键清理：逐键重建，仅触碰确实含打票键的行。
UPDATE accounts
SET extra = (
    SELECT COALESCE(jsonb_object_agg(e.k, e.v), '{}'::jsonb)
    FROM jsonb_each(extra) AS e(k, v)
    WHERE e.k NOT IN (
        'codex_ticket_harvest_enabled',
        'codex_ticket_harvest_models',
        'codex_allow_without_ticket',
        'codex_ticket_ready_models',
        'codex_harvest_proxy_url'
    )
    AND e.k NOT LIKE 'codex\_turn\_ticket:%'
)
WHERE extra IS NOT NULL
  AND (
      extra ?| ARRAY[
          'codex_ticket_harvest_enabled',
          'codex_ticket_harvest_models',
          'codex_allow_without_ticket',
          'codex_ticket_ready_models',
          'codex_harvest_proxy_url'
      ]
      OR EXISTS (
          SELECT 1 FROM jsonb_object_keys(extra) AS k
          WHERE k LIKE 'codex\_turn\_ticket:%'
      )
  );
