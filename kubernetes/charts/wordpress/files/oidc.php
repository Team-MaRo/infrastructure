<?php
/**
 * Plugin Name: Login through Zitadel
 * Description: What the plugin OpenID Connect Generic Client does not do itself: the WordPress role
 * from the Zitadel role, refusing unverified email addresses, and PKCE. Its settings are
 * constants in WORDPRESS_CONFIG_EXTRA (charts/wordpress, README "Login through Zitadel").
 */

// A Zitadel role of this site's project, `<site>:<wordpress-role>`, gives that WordPress role;
// with several, the first of this list wins.
const ZITADEL_ROLE_ORDER = array( 'administrator', 'shop_manager', 'editor', 'author', 'contributor', 'subscriber' );

function zitadel_site_role( $user_claim ) {
	$prefix  = '{{ .Release.Name }}:';
	$granted = array();
	foreach ( array_keys( (array) ( $user_claim['urn:zitadel:iam:org:project:roles'] ?? array() ) ) as $key ) {
		if ( str_starts_with( $key, $prefix ) ) {
			$granted[] = substr( $key, strlen( $prefix ) );
		}
	}
	foreach ( ZITADEL_ROLE_ORDER as $role ) {
		if ( in_array( $role, $granted, true ) && get_role( $role ) ) {
			return $role;
		}
	}
	return null;
}

// Nobody gets in, or is created, without a role for this site or with an address Zitadel has not
// verified. Zitadel refuses a missing role itself (role check); this is the second lock.
function zitadel_may_log_in( $allowed, $user_claim ) {
	return $allowed && ! empty( $user_claim['email_verified'] ) && null !== zitadel_site_role( $user_claim );
}
add_filter( 'openid-connect-generic-user-login-test', 'zitadel_may_log_in', 10, 2 );
add_filter( 'openid-connect-generic-user-creation-test', 'zitadel_may_log_in', 10, 2 );

// The role from Zitadel at creation and at every later login, so a change there applies at the
// next login.
add_filter(
	'openid-connect-generic-alter-user-data',
	function ( $user_data, $user_claim ) {
		$user_data['role'] = zitadel_site_role( $user_claim );
		return $user_data;
	},
	10,
	2
);
add_action(
	'openid-connect-generic-update-user-using-current-claim',
	function ( $user, $user_claim ) {
		$role = zitadel_site_role( $user_claim );
		if ( $role && ! in_array( $role, $user->roles, true ) ) {
			$user->set_role( $role );
		}
	},
	10,
	2
);

// PKCE, which the plugin lacks: a random verifier stored with the login's state, its SHA-256 as
// the challenge in the authorize URL, the verifier itself with the code at the token endpoint.
// Zitadel then hands out tokens only to whoever started the login.
function zitadel_pkce_verifier( $state ) {
	$value = get_transient( 'openid-connect-generic-state--' . $state );
	return is_array( $value ) ? ( $value[ $state ]['code_verifier'] ?? null ) : null;
}

function zitadel_base64url( $bytes ) {
	return rtrim( strtr( base64_encode( $bytes ), '+/', '-_' ), '=' );
}

add_filter(
	'openid-connect-generic-new-state-value',
	function ( $state_value ) {
		$state                                   = array_key_first( $state_value );
		$state_value[ $state ]['code_verifier'] = zitadel_base64url( random_bytes( 32 ) );
		return $state_value;
	}
);

add_filter(
	'openid-connect-generic-auth-url',
	function ( $url ) {
		parse_str( (string) wp_parse_url( $url, PHP_URL_QUERY ), $query );
		$verifier = zitadel_pkce_verifier( $query['state'] ?? '' );
		if ( ! $verifier ) {
			return $url;
		}
		return $url . '&code_challenge=' . zitadel_base64url( hash( 'sha256', $verifier, true ) ) . '&code_challenge_method=S256';
	}
);

add_filter(
	'openid-connect-generic-alter-request',
	function ( $request, $operation ) {
		// phpcs:ignore WordPress.Security.NonceVerification.Recommended -- the plugin checks the state itself.
		$state = isset( $_GET['state'] ) ? sanitize_text_field( wp_unslash( $_GET['state'] ) ) : '';
		if ( 'get-authentication-token' === $operation && $state ) {
			$verifier = zitadel_pkce_verifier( $state );
			if ( $verifier ) {
				$request['body']['code_verifier'] = $verifier;
			}
		}
		return $request;
	},
	10,
	2
);
