<?php
/**
 * Plugin Name: Plugin data
 * Description: Deletes an inactive plugin's data through its own uninstall routine, which
 * DISALLOW_FILE_MODS otherwise hides (README "Plugins").
 */

// WordPress runs a plugin's uninstall routine (uninstall.php or register_uninstall_hook) only from
// its "Delete" link, which DISALLOW_FILE_MODS hides - so a removed plugin would leave its options
// and tables behind. This puts the routine back as "Delete data" on inactive plugins that have
// one; the files stay until the plugin leaves the site's values. Done in wp-admin, where the
// routines expect to run (MonsterInsights' crashes on the command line).
const PLUGIN_DATA_DELETED = 'plugin_data_deleted';

// Texts in German and Spanish (files/plugin-data-*.l10n.php), one file per language whatever
// the country, like the login's.
add_action(
	'init',
	function () {
		$language = substr( determine_locale(), 0, 2 );
		load_textdomain( 'plugin-data', WPMU_PLUGIN_DIR . "/plugin-data-languages/plugin-data-$language.mo" );
	}
);

function plugin_data_deletable( $file ) {
	return current_user_can( 'activate_plugins' )
		&& file_exists( WP_PLUGIN_DIR . '/' . $file )
		&& ! is_plugin_active( $file )
		&& is_uninstallable_plugin( $file );
}

add_filter(
	'plugin_action_links',
	function ( $actions, $file, $data ) {
		if ( ! plugin_data_deletable( $file ) ) {
			return $actions;
		}
		$deleted = get_option( PLUGIN_DATA_DELETED, array() );
		if ( isset( $deleted[ $file ] ) ) {
			/* translators: %s: date the data was deleted. */
			$actions['plugin_data'] = esc_html( sprintf( __( 'Data deleted on %s', 'plugin-data' ), wp_date( get_option( 'date_format' ), $deleted[ $file ] ) ) );
			return $actions;
		}
		$url = wp_nonce_url( admin_url( 'admin-post.php?action=plugin_data_delete&plugin=' . rawurlencode( $file ) ), 'plugin_data_delete_' . $file );
		/* translators: %s: plugin name. */
		$question = sprintf( __( 'Delete all settings and data of %s? Its files stay until it is removed from the site. This cannot be undone.', 'plugin-data' ), $data['Name'] );
		// Red like WordPress's own delete link, but not its `delete` class: the plugin list's script
		// takes over every such link in a row and tries to delete the plugin's files instead.
		$actions['plugin_data'] = sprintf(
			'<a href="%s" style="color: #b32d2e;" onclick="return confirm(%s);">%s</a>',
			esc_url( $url ),
			esc_attr( wp_json_encode( $question ) ),
			esc_html__( 'Delete data', 'plugin-data' )
		);
		return $actions;
	},
	10,
	3
);

// Everything checked again: the link is only a request.
add_action(
	'admin_post_plugin_data_delete',
	function () {
		$file = isset( $_GET['plugin'] ) ? sanitize_text_field( wp_unslash( $_GET['plugin'] ) ) : '';
		check_admin_referer( 'plugin_data_delete_' . $file );
		if ( validate_file( $file ) !== 0 || ! plugin_data_deletable( $file ) ) {
			wp_die( esc_html__( 'This plugin\'s data cannot be deleted here: it must be installed, inactive and have an uninstall routine.', 'plugin-data' ), 403 );
		}
		try {
			uninstall_plugin( $file );
			$deleted          = get_option( PLUGIN_DATA_DELETED, array() );
			$deleted[ $file ] = time();
			update_option( PLUGIN_DATA_DELETED, $deleted, false );
			$result = array( 'plugin_data' => 'deleted' );
		} catch ( Throwable $e ) {
			error_log( sprintf( 'plugin-data: uninstalling %s failed: %s', $file, $e->getMessage() ) );
			$result = array(
				'plugin_data'       => 'failed',
				'plugin_data_error' => rawurlencode( $e->getMessage() ),
			);
		}
		wp_safe_redirect( add_query_arg( $result + array( 'plugin_data_file' => rawurlencode( $file ) ), admin_url( 'plugins.php' ) ) );
		exit;
	}
);

// Activated again, the plugin writes new data, so the link comes back.
add_action(
	'activated_plugin',
	function ( $file ) {
		$deleted = get_option( PLUGIN_DATA_DELETED, array() );
		if ( isset( $deleted[ $file ] ) ) {
			unset( $deleted[ $file ] );
			update_option( PLUGIN_DATA_DELETED, $deleted, false );
		}
	}
);

add_action(
	'admin_notices',
	function () {
		// phpcs:disable WordPress.Security.NonceVerification.Recommended -- only shows the outcome.
		if ( 'plugins' !== get_current_screen()->id || empty( $_GET['plugin_data'] ) || empty( $_GET['plugin_data_file'] ) ) {
			return;
		}
		$file    = sanitize_text_field( wp_unslash( $_GET['plugin_data_file'] ) );
		$plugins = get_plugins();
		$name    = $plugins[ $file ]['Name'] ?? $file;
		if ( 'deleted' === $_GET['plugin_data'] ) {
			/* translators: %s: plugin name. */
			$message = sprintf( __( 'The data of %s was deleted.', 'plugin-data' ), $name );
			$class   = 'notice-success';
		} else {
			$error = sanitize_text_field( wp_unslash( $_GET['plugin_data_error'] ?? '' ) );
			/* translators: 1: plugin name, 2: error message. */
			$message = sprintf( __( 'Deleting the data of %1$s failed: %2$s', 'plugin-data' ), $name, $error );
			$class   = 'notice-error';
		}
		// phpcs:enable
		printf( '<div class="notice %s is-dismissible"><p>%s</p></div>', esc_attr( $class ), esc_html( $message ) );
	}
);
