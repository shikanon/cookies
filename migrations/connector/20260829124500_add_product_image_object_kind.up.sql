ALTER TABLE connector_platform_objects
  DROP CHECK chk_connector_platform_object_kind,
  ADD CONSTRAINT chk_connector_platform_object_kind CHECK (
    object_kind IN (
      'image_material',
      'product_image',
      'video_material',
      'aweme_photo_material',
      'marketing_product',
      'orange_landing_page',
      'optimization_target',
      'conversion_event_asset',
      'industry_category',
      'brand',
      'authorized_identity'
    )
  );
