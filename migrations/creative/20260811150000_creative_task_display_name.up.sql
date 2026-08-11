ALTER TABLE creative_tasks
  ADD COLUMN display_name VARCHAR(160) NOT NULL DEFAULT '' AFTER id;

UPDATE creative_tasks
SET display_name = CASE
  WHEN creative_format = 'video' THEN '未命名品牌广告'
  ELSE '未命名创意任务'
END
WHERE display_name = '';
