<?php
/**
 * Plugin Name: S3-Uploads for Hetzner Object Storage
 * Description: Loads S3-Uploads as a must-use plugin (always on, no activation in the database) and
 * points it at Hetzner. The bucket, key and ACL are constants from WORDPRESS_CONFIG_EXTRA.
 */

// The plugin loads its autoloader only on plugins_loaded, but registers its wp-cli command when the
// file loads - wp-cli then cannot find the class and every `wp` command fails. Loading the
// autoloader first fixes that (S3-Uploads 3.0.10's manual-install package).
require WPMU_PLUGIN_DIR . '/../plugins/s3-uploads/vendor/autoload.php';
require WPMU_PLUGIN_DIR . '/../plugins/s3-uploads/s3-uploads.php';

add_filter( 's3_uploads_s3_client_params', function ( $params ) {
	$params['endpoint']                = 'https://nbg1.your-objectstorage.com';
	$params['use_path_style_endpoint'] = true;
	// Newer AWS SDKs add checksums to every upload, which S3-compatible stores may reject.
	$params['request_checksum_calculation'] = 'when_required';
	$params['response_checksum_validation'] = 'when_required';
	return $params;
} );
