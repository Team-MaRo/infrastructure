<?php
/**
 * Plugin Name: Login through OpenID Connect
 * Description: Roles and profile from the identity provider, verified addresses only, PKCE.
 */

// What the plugin OpenID Connect Generic Client does not do itself, for the login through Zitadel.
// Its settings are constants in WORDPRESS_CONFIG_EXTRA (charts/wordpress, README "Login through
// Zitadel"). No text people see names Zitadel - only "OpenID Connect" (user decision,
// 2026-10-21); the translations are in zitadel-login-languages/ (files/zitadel-login-*.l10n.php).
// One file per language, whatever the country: Polylang's Spanish is es_VE, the site's German
// de_CH. Loaded again at every locale change: Polylang blocks loading until it knows a page's
// language, then empties all translations and announces the new locale.
function zitadel_load_texts( $locale = null ) {
	$language = substr( $locale ? $locale : determine_locale(), 0, 2 );
	unload_textdomain( 'zitadel-login' );
	load_textdomain( 'zitadel-login', WPMU_PLUGIN_DIR . "/zitadel-login-languages/zitadel-login-$language.mo" );
}
add_action( 'init', 'zitadel_load_texts' );
add_action( 'change_locale', 'zitadel_load_texts' );

// The plugin's button keeps its English text, in our translations; a button that brings its own
// text (Create an account) passes through the same filter unchanged.
add_filter(
	'openid-connect-generic-login-button-text',
	function ( $text ) {
		return 'Login with OpenID Connect' === $text ? __( 'Login with OpenID Connect', 'zitadel-login' ) : $text;
	}
);

// The roles are Zitadel's alone: every role of this site's project, `<site>:<wordpress-role>`,
// whose WordPress role exists here - whatever WordPress or a plugin brings, no list to keep.
// Keys WordPress does not know are skipped.
function zitadel_site_roles( $user_claim ) {
	$prefix = '{{ .Release.Name }}:';
	$roles  = array();
	foreach ( array_keys( (array) ( $user_claim['urn:zitadel:iam:org:project:roles'] ?? array() ) ) as $key ) {
		$role = str_starts_with( $key, $prefix ) ? substr( $key, strlen( $prefix ) ) : '';
		if ( '' !== $role && get_role( $role ) ) {
			$roles[] = $role;
		}
	}
	return $roles;
}

// Nobody gets in, or is created, without a role for this site or with an address Zitadel has not
// verified. Zitadel refuses a missing role itself (role check); this is the second lock.
function zitadel_may_log_in( $allowed, $user_claim ) {
	return $allowed && ! empty( $user_claim['email_verified'] ) && array() !== zitadel_site_roles( $user_claim );
}
add_filter( 'openid-connect-generic-user-login-test', 'zitadel_may_log_in', 10, 2 );
add_filter( 'openid-connect-generic-user-creation-test', 'zitadel_may_log_in', 10, 2 );

// At creation and at every later login the account's roles become exactly Zitadel's, so a change
// there applies at the next login and one made in wp-admin is undone by it.
function zitadel_apply_roles( $user, $user_claim ) {
	$roles   = zitadel_site_roles( $user_claim );
	$current = array_values( $user->roles );
	sort( $current );
	$wanted = $roles;
	sort( $wanted );
	if ( array() === $roles || $current === $wanted ) {
		return;
	}
	$user->set_role( array_shift( $roles ) );
	foreach ( $roles as $role ) {
		$user->add_role( $role );
	}
}
add_filter(
	'openid-connect-generic-alter-user-data',
	function ( $user_data, $user_claim ) {
		$user_data['role'] = zitadel_site_roles( $user_claim )[0] ?? '';
		return $user_data;
	},
	10,
	2
);
add_action( 'openid-connect-generic-user-create', 'zitadel_apply_roles', 10, 2 );
add_action( 'openid-connect-generic-update-user-using-current-claim', 'zitadel_apply_roles', 10, 2 );

// The profile too: email (only a verified one), first and last name and the public name (Zitadel's
// free display name; WordPress's dropdown only suggests combinations and lists the current value
// anyway) from Zitadel at every login - the plugin sets them only when it creates the account, so
// a linked one would keep its old values. WooCommerce's billing and shipping data stays the
// customer's own.
function zitadel_apply_profile( $user, $user_claim ) {
	$wanted = array(
		'user_email'   => ! empty( $user_claim['email_verified'] ) ? (string) ( $user_claim['email'] ?? '' ) : '',
		'first_name'   => (string) ( $user_claim['given_name'] ?? '' ),
		'last_name'    => (string) ( $user_claim['family_name'] ?? '' ),
		'display_name' => (string) ( $user_claim['name'] ?? '' ),
	);
	$changes = array();
	foreach ( $wanted as $field => $value ) {
		if ( '' !== $value && $value !== (string) $user->get( $field ) ) {
			$changes[ $field ] = $value;
		}
	}
	if ( array() === $changes ) {
		return;
	}
	// No "your email was changed" mail to the old address at every sync, from the shared quota.
	add_filter( 'send_email_change_email', '__return_false' );
	$GLOBALS['zitadel_syncing_profile'] = true;
	$result                             = wp_update_user( array( 'ID' => $user->ID ) + $changes );
	$GLOBALS['zitadel_syncing_profile'] = false;
	remove_filter( 'send_email_change_email', '__return_false' );
	if ( is_wp_error( $result ) ) {
		// Most likely the address belongs to another account; the login goes on with the old one.
		error_log( sprintf( 'zitadel: profile of %s not updated: %s', $user->user_login, $result->get_error_message() ) );
	}
}
add_action( 'openid-connect-generic-user-create', 'zitadel_apply_profile', 10, 2 );
add_action( 'openid-connect-generic-update-user-using-current-claim', 'zitadel_apply_profile', 10, 2 );

// For a linked account those fields are Zitadel's: a change made here would last only until the
// next login. wp-admin's profile and WooCommerce's account details show them read only with a
// link to the account page there, and a change sent anyway is dropped - only the sync above may
// write them.
function zitadel_account_url() {
	$login = wp_parse_url( OIDC_ENDPOINT_LOGIN_URL );
	return $login['scheme'] . '://' . $login['host'] . '/ui/console/users/me';
}

function zitadel_profile_locked( $user_id ) {
	return $user_id && zitadel_linked_id( $user_id ) && empty( $GLOBALS['zitadel_syncing_profile'] );
}

add_filter(
	'wp_pre_insert_user_data',
	function ( $data, $update, $user_id ) {
		if ( $update && zitadel_profile_locked( $user_id ) ) {
			$user                 = get_userdata( $user_id );
			$data['user_email']   = $user->user_email;
			$data['display_name'] = $user->display_name;
		}
		return $data;
	},
	10,
	3
);
add_filter(
	'insert_user_meta',
	function ( $meta, $user, $update ) {
		if ( $update && zitadel_profile_locked( $user->ID ) ) {
			$meta['first_name'] = get_user_meta( $user->ID, 'first_name', true );
			$meta['last_name']  = get_user_meta( $user->ID, 'last_name', true );
		}
		return $meta;
	},
	10,
	3
);
// wp-admin would otherwise mail a confirmation link to a new address before saving anything.
add_action(
	'personal_options_update',
	function ( $user_id ) {
		if ( zitadel_profile_locked( $user_id ) ) {
			remove_action( 'personal_options_update', 'send_confirmation_on_profile_email' );
		}
	},
	0
);

// The fields read only, and right before them where to change them instead; wp-admin's hint that a
// new address gets a confirmation mail goes, since there is none to give.
function zitadel_profile_fields_locked( $user_id, $fields ) {
	if ( ! zitadel_profile_locked( $user_id ) ) {
		return;
	}
	printf(
		'<p class="zitadel-profile-note">%s <a href="%s" target="_blank" rel="noopener">%s</a></p>',
		esc_html__( 'Name and email address come from your login account - change them there:', 'zitadel-login' ),
		esc_url( zitadel_account_url() ),
		esc_html__( 'Account settings', 'zitadel-login' )
	);
	?>
	<script>
		document.addEventListener( 'DOMContentLoaded', function () {
			<?php echo wp_json_encode( $fields ); ?>.forEach( function ( id ) {
				var field = document.getElementById( id );
				if ( field && field.tagName === 'SELECT' ) {
					// A dropdown has no read-only state; disabled, it is greyed out and not sent at all.
					field.disabled = true;
				} else if ( field ) {
					field.readOnly = true;
					field.setAttribute( 'aria-readonly', 'true' );
				}
			} );
			var first = document.getElementById( <?php echo wp_json_encode( $fields[0] ); ?> );
			var note  = document.querySelector( '.zitadel-profile-note' );
			var row   = first && first.closest( 'tr, .form-row' );
			if ( row && note ) {
				if ( row.tagName === 'TR' ) {
					var cell     = document.createElement( 'td' );
					var noteRow  = document.createElement( 'tr' );
					cell.colSpan = 2;
					cell.appendChild( note );
					noteRow.appendChild( cell );
					row.parentNode.insertBefore( noteRow, row );
				} else {
					row.parentNode.insertBefore( note, row );
				}
			}
			var hint = document.getElementById( 'email-description' );
			if ( hint ) {
				hint.remove();
			}
		} );
	</script>
	<?php
}
add_action(
	'personal_options',
	function ( $user ) {
		zitadel_profile_fields_locked( $user->ID, array( 'first_name', 'last_name', 'display_name', 'email' ) );
	}
);
add_action(
	'woocommerce_edit_account_form_start',
	function () {
		zitadel_profile_fields_locked( get_current_user_id(), array( 'account_first_name', 'account_last_name', 'account_display_name', 'account_email' ) );
	}
);

// Accounts come from Zitadel only: WordPress's and WooCommerce's own registration would create
// them with a role Zitadel never gave.
add_filter( 'pre_option_users_can_register', '__return_zero' );
add_filter( 'pre_option_woocommerce_enable_myaccount_registration', fn() => 'no' );
add_filter( 'pre_option_woocommerce_enable_signup_and_login_from_checkout', fn() => 'no' );

// The plugin's button through its shortcode. On WooCommerce's own login form (My account,
// checkout) it leads back to the same page; on wp-login.php to the plugin's default.
function zitadel_button( $text, $scope = OIDC_CLIENT_SCOPE ) {
	$redirect = 'wp-login.php' === ( $GLOBALS['pagenow'] ?? '' ) ? '' : sprintf( ' redirect_to="%s"', esc_attr( home_url( add_query_arg( null, null ) ) ) );
	return do_shortcode(
		sprintf( '[openid_connect_generic_login_button button_text="%s" scope="%s"%s]', esc_attr( $text ), esc_attr( $scope ), $redirect )
	);
}
add_action(
	'woocommerce_login_form_end',
	function () {
		echo zitadel_button( __( 'Login with OpenID Connect', 'zitadel-login' ) ); // phpcs:ignore WordPress.Security.EscapeOutput.OutputNotEscaped -- the plugin's own markup.
	}
);

// wp-login.php puts the buttons first: they move into a box of their own, and the password form -
// only the break-glass administrator's - waits behind a link below them. Only with JavaScript (the
// styles need WordPress's `js` class), only on the login itself and not in wp-admin's re-login
// dialog; after a failed password login the form shows at once, next to its error.
add_action(
	'login_head',
	function () {
		?>
<style>
	.js.login-action-login:not(.interim-login):not(.zitadel-password-form) #loginform,
	.js.login-action-login:not(.interim-login):not(.zitadel-password-form) #nav {
		display: none;
	}
	.zitadel-login-box {
		margin: 24px 0 0;
		padding: 26px 24px;
		background: #fff;
		border: 1px solid #c3c4c7;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
	}
	.zitadel-login-box .openid-connect-login-button {
		margin: 0 0 12px !important;
	}
	.zitadel-login-box .openid-connect-login-button:last-of-type {
		margin-bottom: 0 !important;
	}
	/* wp-login.php floats .button-primary to the right of the form's checkbox. */
	.zitadel-login-box .openid-connect-login-button .button {
		float: none;
		display: block;
		width: 100%;
		text-align: center;
	}
	.zitadel-login-or {
		display: flex;
		align-items: center;
		gap: 12px;
		margin: 20px 0 12px;
		color: #646970;
		text-transform: uppercase;
	}
	.zitadel-login-or::before,
	.zitadel-login-or::after {
		content: "";
		flex: 1;
		border-top: 1px solid #dcdcde;
	}
	.zitadel-login-password {
		display: block;
		text-align: center;
	}
</style>
		<?php
	}
);
add_action(
	'login_footer',
	function () {
		?>
<script>
	( function () {
		var body    = document.body;
		var form    = document.getElementById( 'loginform' );
		var buttons = document.querySelectorAll( '.openid-connect-login-button' );
		if ( ! body.classList.contains( 'login-action-login' ) || body.classList.contains( 'interim-login' ) || ! form || ! buttons.length ) {
			return;
		}
		var box = document.createElement( 'div' );
		box.className = 'zitadel-login-box';
		buttons.forEach( function ( button ) {
			box.appendChild( button );
		} );
		buttons[0].querySelector( '.button' ).classList.add( 'button-primary' );
		var or = document.createElement( 'p' );
		or.className   = 'zitadel-login-or';
		or.textContent = <?php echo wp_json_encode( __( 'or', 'zitadel-login' ) ); ?>;
		var link = document.createElement( 'a' );
		link.className   = 'zitadel-login-password';
		link.href        = '#loginform';
		link.textContent = <?php echo wp_json_encode( __( 'Log in with username and password', 'zitadel-login' ) ); ?>;
		box.appendChild( or );
		box.appendChild( link );
		form.parentNode.insertBefore( box, form );
		function showForm() {
			body.classList.add( 'zitadel-password-form' );
			or.remove();
			link.remove();
		}
		link.addEventListener( 'click', function ( event ) {
			event.preventDefault();
			showForm();
			document.getElementById( 'user_login' ).focus();
		} );
		if ( document.getElementById( 'login_error' ) ) {
			showForm();
		}
	} )();
</script>
		<?php
	}
);
{{- with .Values.oidc.registerOrganisation }}

// Registration in Zitadel (values oidc.registerOrganisation): a second button asks Zitadel's
// registration page (prompt=create) for this organisation. Only this button names it - the scope
// also refuses users of every other organisation, so the login button names none.
const ZITADEL_REGISTER_SCOPE = OIDC_CLIENT_SCOPE . ' urn:zitadel:iam:org:id:{{ . }}';

function zitadel_register_button() {
	return zitadel_button( __( 'Create an account', 'zitadel-login' ), ZITADEL_REGISTER_SCOPE );
}
add_filter(
	'login_message',
	function ( $message ) {
		return $message . zitadel_register_button();
	},
	100
);
add_action(
	'woocommerce_login_form_end',
	function () {
		echo zitadel_register_button(); // phpcs:ignore WordPress.Security.EscapeOutput.OutputNotEscaped -- the plugin's own markup.
	},
	11
);
add_filter(
	'openid-connect-generic-auth-url',
	function ( $url ) {
		parse_str( (string) wp_parse_url( $url, PHP_URL_QUERY ), $query );
		return ( $query['scope'] ?? '' ) === ZITADEL_REGISTER_SCOPE ? $url . '&prompt=create' : $url;
	}
);
{{- end }}

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

// Which Zitadel user an account is linked to, for reading only: a row on the profile page and a
// column in the user list. The plugin keeps the ID out of sight; linking stays a deliberate step
// (README "Linking an existing account"). An account without one - the break-glass
// administrator - logs in with the form only.
function zitadel_linked_id( $user_id ) {
	return (string) get_user_option( 'openid-connect-generic-subject-identity', $user_id );
}

function zitadel_profile_row( $user ) {
	$id = zitadel_linked_id( $user->ID );
	?>
	<h2><?php esc_html_e( 'OpenID Connect', 'zitadel-login' ); ?></h2>
	<table class="form-table" role="presentation">
		<tr>
			<th><?php esc_html_e( 'OpenID Connect user ID', 'zitadel-login' ); ?></th>
			<td><?php echo $id ? '<code>' . esc_html( $id ) . '</code>' : esc_html__( 'Not linked - logs in with the password form only.', 'zitadel-login' ); ?></td>
		</tr>
	</table>
	<?php
}
add_action( 'show_user_profile', 'zitadel_profile_row' );
add_action( 'edit_user_profile', 'zitadel_profile_row' );

add_filter(
	'manage_users_columns',
	function ( $columns ) {
		$columns['zitadel'] = __( 'OpenID Connect', 'zitadel-login' );
		return $columns;
	}
);
add_filter(
	'manage_users_custom_column',
	function ( $output, $column, $user_id ) {
		if ( 'zitadel' !== $column ) {
			return $output;
		}
		$id = zitadel_linked_id( $user_id );
		return $id ? '<code>' . esc_html( $id ) . '</code>' : '&#8212;';
	},
	10,
	3
);

// After logging out the plugin sends people to Zitadel's end_session with WordPress's own target,
// wp-login.php?loggedout=true&wp_lang=<their language>. Zitadel accepts only post-logout addresses
// it knows exactly, and the language varies, so the target loses wp_lang before the plugin (99)
// builds the URL; the page still shows in their language. The app in Zitadel lists this address.
add_filter(
	'logout_redirect',
	function ( $redirect_to ) {
		if ( false !== strpos( $redirect_to, 'wp-login.php?loggedout=true' ) ) {
			return site_url( 'wp-login.php?loggedout=true' );
		}
		return $redirect_to;
	},
	98
);

// A linked account logs in through Zitadel only: the password form refuses it, it gets no
// password reset ("Lost your password?" would otherwise let whoever reads its mailbox set one and
// get past Zitadel and its second factor), and its profile offers no new password. Unlinked
// accounts - the break-glass administrator - keep both. The login through Zitadel sets the session
// itself and never passes these checks. Application Passwords stay (user decision, 2026-10-21:
// customers' apps, the WooCommerce app, may use them) - a key of WordPress's own that Zitadel never
// sees (README "Content through Claude").
add_filter(
	'wp_authenticate_user',
	function ( $user ) {
		if ( $user instanceof WP_User && zitadel_linked_id( $user->ID ) ) {
			return new WP_Error( 'zitadel_only', __( 'This account logs in with OpenID Connect - use "Login with OpenID Connect".', 'zitadel-login' ) );
		}
		return $user;
	}
);
// The password lives in Zitadel, so its reset does too; the message says where, instead of
// WordPress's bare "not allowed" (WordPress and WooCommerce show the error's text).
function zitadel_reset_error() {
	return new WP_Error( 'zitadel_only', __( 'This account logs in with OpenID Connect - use "Login with OpenID Connect" and choose "Reset Password" there.', 'zitadel-login' ) );
}
add_filter(
	'allow_password_reset',
	function ( $allowed, $user_id ) {
		return zitadel_linked_id( $user_id ) ? zitadel_reset_error() : $allowed;
	},
	10,
	2
);
// WordPress checks allow_password_reset only when it creates a reset link; a link sent before (or
// by another way) is refused here, when it is used.
add_action(
	'validate_password_reset',
	function ( $errors, $user ) {
		if ( $user instanceof WP_User && zitadel_linked_id( $user->ID ) ) {
			$errors->add( 'zitadel_only', zitadel_reset_error()->get_error_message() );
		}
	},
	10,
	2
);
add_filter(
	'show_password_fields',
	function ( $show, $user ) {
		return zitadel_linked_id( $user->ID ) ? false : $show;
	},
	10,
	2
);
