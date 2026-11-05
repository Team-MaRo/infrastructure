<?php
/**
 * Plugin Name: WooCommerce logs to the container log
 * Description: Sends WooCommerce's log entries to php-fpm's log (the container's stderr) instead of files in the uploads bucket.
 */

// WooCommerce's log directory is under the uploads by default, which S3-Uploads puts in the bucket:
// every entry a write to S3, and WooCommerce's log viewer and its pruning cannot list s3:// (empty,
// never cleaned up). Defined before WooCommerce does, its own files - the directory's .htaccess -
// stay in the pod.
if ( ! defined( 'WC_LOG_DIR' ) ) {
	define( 'WC_LOG_DIR', '/tmp/wc-logs/' );
}

// The only handler: each entry into php-fpm's log, and so into Loki. WooCommerce's threshold
// setting still decides which levels reach it.
add_filter(
	'woocommerce_register_log_handlers',
	function () {
		return array(
			new class() implements WC_Log_Handler_Interface {
				public function handle( $timestamp, $level, $message, $context ) {
					$source = is_array( $context ) && ! empty( $context['source'] ) ? $context['source'] : 'log';
					error_log( sprintf( 'woocommerce %s [%s] %s', $level, $source, $message ) ); // phpcs:ignore WordPress.PHP.DevelopmentFunctions.error_log_error_log
					return true;
				}
			},
		);
	}
);
