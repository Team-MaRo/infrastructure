<?php
/**
 * Plugin Name: Content through Claude
 * Description: Which abilities Claude may use over MCP, and how it logs in.
 */

// The settings of Enable Abilities for MCP, fixed here instead of in its settings page
// (charts/wordpress, README "Content through Claude"): pre_option answers before the database,
// so a click in wp-admin changes nothing. Its own OAuth server, site-wide Bearer token and
// client registration stay off: Claude logs in with Zitadel (below), so who may use it is decided
// there alone.
add_filter( 'pre_option_ewpa_oauth_enabled', fn() => '0' );
add_filter( 'pre_option_ewpa_bearer_enabled', fn() => '0' );
add_filter( 'pre_option_ewpa_oauth_connectors_enabled', fn() => '0' );

// What Claude may do: read and write content, never delete it, and nothing about people - no
// users, comments or orders (personal data would flow to Claude), no site-wide search and replace,
// no code. The plugin's defaults would allow nearly all of that. An ability of a plugin a site
// lacks (Polylang, WooCommerce, Yoast) simply is not registered there.
const MCP_ABILITIES = array(
	'core/get-site-info',
	'core/get-user-info',
	// Posts, pages, media, categories and tags.
	'ewpa/get-posts',
	'ewpa/get-post',
	'ewpa/get-pages',
	'ewpa/get-page',
	'ewpa/get-media',
	'ewpa/get-categories',
	'ewpa/get-tags',
	'ewpa/create-post',
	'ewpa/update-post',
	'ewpa/create-page',
	'ewpa/duplicate-post',
	'ewpa/upload-image',
	'ewpa/create-category',
	'ewpa/create-tag',
	'ewpa/assign-post-terms',
	// Custom post types (portfolio items).
	'ewpa/list-post-types',
	'ewpa/get-cpt-items',
	'ewpa/get-cpt-item',
	'ewpa/create-cpt-item',
	'ewpa/update-cpt-item',
	'ewpa/get-cpt-taxonomies',
	'ewpa/assign-cpt-terms',
	// Menus, reading only.
	'ewpa/list-menus',
	'ewpa/get-menu',
	// Translations (Polylang).
	'ewpa/list-languages',
	'ewpa/get-post-translations',
	'ewpa/create-post-translation',
	'ewpa/set-post-language',
	'ewpa/link-post-translation',
	'ewpa/get-term-translations',
	'ewpa/create-term-translation',
	'ewpa/set-term-language',
	'ewpa/link-term-translation',
	// Yoast SEO fields of a post.
	'ewpa/yoast-get-seo',
	'ewpa/yoast-update-seo',
	// WooCommerce products through the plugin, reading and editing.
	'ewpa/wc-get-products',
	'ewpa/wc-get-product',
	'ewpa/wc-update-product',
);
add_filter( 'pre_option_ewpa_enabled_abilities', fn() => MCP_ABILITIES );

// The list decides for every plugin, not only for Enable Abilities: WooCommerce, Yoast, WPForms,
// WP Mail SMTP and core mark abilities of their own public for MCP - orders, mail logs, site-wide
// SEO settings - and a plugin added later could do the same. Whatever is not listed is made
// private here; WooCommerce's product abilities are listed (creating a product exists only there)
// and made public, its order and delete abilities are not. Last, after every plugin's own filter.
const MCP_WOOCOMMERCE_ABILITIES = array(
	'woocommerce/products-query',
	'woocommerce/product-create',
	'woocommerce/product-update',
);
add_filter(
	'wp_register_ability_args',
	function ( $args, $name ) {
		if ( in_array( $name, MCP_WOOCOMMERCE_ABILITIES, true ) ) {
			$args['meta']['mcp']['public'] = true;
		} elseif ( ! in_array( $name, MCP_ABILITIES, true ) ) {
			$args['meta']['mcp']['public'] = false;
		}
		return $args;
	},
	PHP_INT_MAX,
	2
);

// Claude logs in with Zitadel: its app {{ .Values.mcp.clientId }} (tofu/zitadel) in the site's
// project, so the role check applies. The MCP Adapter's own server answers Claude; it knows only a
// logged-in WordPress user, which this makes out of Zitadel's token. A request without one gets
// 401 with the address of the metadata that tells Claude where to log in.
const MCP_ROUTE            = '/mcp/mcp-adapter-default-server';
const MCP_METADATA_ROUTE   = '/oidc-mcp/v1/resource';
const MCP_CLAUDE_CLIENT_ID = '{{ .Values.mcp.clientId }}';

function mcp_is_mcp_request() {
	// phpcs:ignore WordPress.Security.ValidatedSanitizedInput -- compared, never output.
	$uri = (string) ( $_SERVER['REQUEST_URI'] ?? '' );
	return false !== strpos( $uri, '/wp-json' . MCP_ROUTE ) || false !== strpos( rawurldecode( $uri ), 'rest_route=' . MCP_ROUTE );
}

// The WordPress user behind a token, asked of Zitadel (introspection, with the site's own app's
// credentials) rather than read from the token: a revoked token, a deactivated user and a session
// ended in Zitadel count at once. Cached a minute per token. Only tokens of the Claude app count,
// and only for someone who has logged in to the site once, whose account is linked to their
// Zitadel user; their roles come from the token, as at the login (oidc.php).
function mcp_user_for_token( $token ) {
	$key    = 'mcp_token_' . hash( 'sha256', $token );
	$cached = get_transient( $key );
	if ( false !== $cached ) {
		return (int) $cached;
	}
	$response = wp_remote_post(
		dirname( OIDC_ENDPOINT_TOKEN_URL ) . '/introspect',
		array(
			'timeout' => 5,
			'headers' => array( 'Authorization' => 'Basic ' . base64_encode( OIDC_CLIENT_ID . ':' . OIDC_CLIENT_SECRET ) ),
			'body'    => array( 'token' => $token ),
		)
	);
	if ( is_wp_error( $response ) || 200 !== wp_remote_retrieve_response_code( $response ) ) {
		// Not cached: Zitadel unreachable is no answer about the token.
		return 0;
	}
	$claim   = (array) json_decode( wp_remote_retrieve_body( $response ), true );
	$user_id = 0;
	if ( ! empty( $claim['active'] ) && MCP_CLAUDE_CLIENT_ID === ( $claim['client_id'] ?? '' ) && ! empty( $claim['sub'] ) ) {
		$ids  = get_users(
			array(
				'meta_key'   => 'openid-connect-generic-subject-identity', // phpcs:ignore WordPress.DB.SlowDBQuery
				'meta_value' => $claim['sub'], // phpcs:ignore WordPress.DB.SlowDBQuery
				'number'     => 1,
				'fields'     => 'ID',
			)
		);
		$user = $ids ? get_userdata( (int) $ids[0] ) : false;
		if ( $user && ! isset( $claim['urn:zitadel:iam:org:project:roles'] ) ) {
			$user_id = $user->ID;
		} elseif ( $user && array() !== zitadel_site_roles( $claim ) ) {
			zitadel_apply_roles( $user, $claim );
			$user_id = $user->ID;
		}
	}
	$ttl = isset( $claim['exp'] ) ? min( 60, max( 1, (int) $claim['exp'] - time() ) ) : 60;
	set_transient( $key, $user_id, $ttl );
	return $user_id;
}

add_filter(
	'determine_current_user',
	function ( $user_id ) {
		if ( $user_id || ! mcp_is_mcp_request() ) {
			return $user_id;
		}
		// phpcs:ignore WordPress.Security.ValidatedSanitizedInput -- a token, checked by Zitadel.
		$header = (string) ( $_SERVER['HTTP_AUTHORIZATION'] ?? '' );
		if ( ! preg_match( '/^Bearer\s+(\S+)$/i', $header, $match ) ) {
			return $user_id;
		}
		return mcp_user_for_token( $match[1] ) ?: $user_id;
	},
	30
);

// Where Claude learns to log in with Zitadel (OAuth protected resource metadata, RFC 9728):
// pointed at from the 401, so no /.well-known path is needed.
add_action(
	'rest_api_init',
	function () {
		register_rest_route(
			trim( dirname( MCP_METADATA_ROUTE ), '/' ),
			'/' . basename( MCP_METADATA_ROUTE ),
			array(
				'methods'             => 'GET',
				'permission_callback' => '__return_true',
				'callback'            => function () {
					$login = wp_parse_url( OIDC_ENDPOINT_LOGIN_URL );
					return array(
						'resource'                 => rest_url( ltrim( MCP_ROUTE, '/' ) ),
						'authorization_servers'    => array( $login['scheme'] . '://' . $login['host'] ),
						'scopes_supported'         => array( 'openid', 'profile', 'email', 'offline_access' ),
						'bearer_methods_supported' => array( 'header' ),
					);
				},
			)
		);
	}
);

add_filter(
	'rest_post_dispatch',
	function ( $response, $server, $request ) {
		if ( MCP_ROUTE === $request->get_route() && 401 === $response->get_status() ) {
			$response->header( 'WWW-Authenticate', sprintf( 'Bearer resource_metadata="%s"', rest_url( ltrim( MCP_METADATA_ROUTE, '/' ) ) ) );
		}
		return $response;
	},
	10,
	3
);
