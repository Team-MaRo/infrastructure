<?php
/**
 * Plugin Name: Complianz's files in the pod
 * Description: Keeps the files Complianz generates (its banner's CSS) in the pod instead of the uploads bucket.
 */

// Complianz writes its banner's CSS under the uploads and checks the file's age there on every page
// it builds - with S3-Uploads a request to the bucket per page, and one that hung kept the page
// waiting until nginx gave up (504, 2026-10-26). In wp-content/complianz/ (created at start) nginx
// serves it and Varnish keeps it by its ?ver=; after a restart Complianz rebuilds the missing file
// on the first page. Without Complianz nothing calls these filters.
add_filter( 'cmplz_upload_dir', fn() => WP_CONTENT_DIR );
add_filter( 'cmplz_upload_url', fn() => content_url() );
