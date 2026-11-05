<?php
/**
 * Plugin Name: WPConsent's cache in the pod
 * Description: Keeps WPConsent's file cache (data it fetches from its own servers) in the pod instead of the uploads bucket.
 */

// WPConsent checks and reads its cache under the uploads on every page - with S3-Uploads four
// requests to the bucket, 61 ms (Query Monitor's S3 panel, 2026-11-05). It has no filter for the
// path: WPConsent_File_Cache builds it from wp_upload_dir() in a private method. So the uploads'
// base folder becomes the pod's /tmp only when that class asks; its thumbnails and every other
// plugin keep the bucket. The cache holds only data WPConsent fetches again when it is missing,
// so a restart costs one fetch. Registered only where WPConsent is installed.
add_action(
	'plugins_loaded',
	function () {
		if ( ! class_exists( 'WPConsent_File_Cache' ) ) {
			return;
		}
		add_filter(
			'upload_dir',
			function ( $uploads ) {
				// This closure, WP_Hook, apply_filters(), wp_upload_dir(), then its caller - a few
				// frames more in case PHP lists the callback's invocation on its own.
				foreach ( debug_backtrace( DEBUG_BACKTRACE_IGNORE_ARGS, 8 ) as $frame ) {
					if ( 'WPConsent_File_Cache' === ( $frame['class'] ?? '' ) ) {
						$uploads['basedir'] = sys_get_temp_dir();
						break;
					}
				}
				return $uploads;
			},
			// After S3-Uploads' own filter, which points the uploads at the bucket.
			20
		);
	},
	PHP_INT_MIN
);
