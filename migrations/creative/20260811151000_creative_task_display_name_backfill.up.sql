UPDATE creative_tasks
SET display_name = LEFT(COALESCE(
  NULLIF(JSON_UNQUOTE(JSON_EXTRACT(direction_payload, '$.focus')), ''),
  NULLIF(JSON_UNQUOTE(JSON_EXTRACT(direction_payload, '$.concept')), ''),
  display_name
), 160)
WHERE display_name IN ('未命名品牌广告', '未命名创意任务');
