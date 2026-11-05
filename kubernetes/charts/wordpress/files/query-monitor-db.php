<?php
/**
 * Query Monitor's database class, only while it measures (README "Measuring a request").
 *
 * Installed as wp-content/db.php. Query Monitor's own drop-in replaces wpdb for every request,
 * whether the plugin is active or not; this one hands over to it only while the plugin is active
 * (the marker its mu-plugin keeps, query-monitor-panels.php) and only for a logged-in request.
 * Every other request gets WordPress's own wpdb.
 */

if ( ! defined( 'QUERY_MONITOR_MARKER' ) || ! file_exists( QUERY_MONITOR_MARKER ) ) {
	return;
}

$query_monitor_db = WP_CONTENT_DIR . '/plugins/query-monitor/wp-content/db.php';
if ( ! is_readable( $query_monitor_db ) ) {
	return;
}

foreach ( array_keys( $_COOKIE ) as $query_monitor_cookie ) {
	if ( str_starts_with( $query_monitor_cookie, 'wordpress_logged_in_' ) ) {
		define( 'QUERY_MONITOR_MEASURING', true );
		require $query_monitor_db;
		break;
	}
}

unset( $query_monitor_db, $query_monitor_cookie );
