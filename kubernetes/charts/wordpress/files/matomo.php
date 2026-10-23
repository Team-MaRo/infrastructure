<?php
/**
 * Plugin Name: Web analytics through Matomo
 * Description: Counts visits, products, carts and orders in the cluster's Matomo.
 */

// README "Web analytics": the site's visitors load Matomo's tracker from the site's own address
// (nginx passes /m/a.js and /m/e on to Matomo, default.conf), so ad blockers that know Matomo's
// file names let it through. Everyone is counted without cookies; whoever agrees to statistics in
// the site's consent banner (through the WP Consent API) is also recognised on later visits.
// Orders go to Matomo from the server, so an ad blocker cannot lose a sale.
const MATOMO_SITE_ID = {{ .Values.matomo.siteId }};
// Inside the cluster, past Cloudflare; Matomo answers only to its own host name.
const MATOMO_TRACKER = 'http://matomo.matomo.svc/matomo.php';
const MATOMO_HOST    = 'matomo.d3strukt0r.dev';

// The tracker on every page, after the scripts so the WP Consent API's functions exist. Pages come
// from the page cache, so nothing in here may depend on the visitor: their consent is read in the
// browser.
add_action(
	'wp_footer',
	function () {
		$calls = array();
		if ( function_exists( 'is_product' ) && is_product() ) {
			$product = wc_get_product( get_queried_object_id() );
			if ( $product ) {
				$calls[] = array( 'setEcommerceView', matomo_sku( $product ), $product->get_name(), matomo_categories( $product ), (float) wc_get_price_to_display( $product ) );
			}
		} elseif ( function_exists( 'is_product_category' ) && is_product_category() ) {
			$calls[] = array( 'setEcommerceView', false, false, single_term_title( '', false ) );
		}
		if ( function_exists( 'is_order_received_page' ) && is_order_received_page() ) {
			// The order key in the address is the customer's access to their order.
			$calls[] = array( 'setCustomUrl', strtok( home_url( add_query_arg( array() ) ), '?' ) );
		}
		$calls = array_merge( $calls, matomo_cart_calls() );
		?>
<script>
var _paq = window._paq = window._paq || [];
_paq.push(['setTrackerUrl', '/m/e']);
_paq.push(['setSiteId', '<?php echo (int) MATOMO_SITE_ID; ?>']);
_paq.push(['requireCookieConsent']);
if (typeof wp_has_consent === 'function' && window.wp_consent_type && wp_has_consent('statistics')) {
	_paq.push(['rememberCookieConsentGiven']);
}
document.addEventListener('wp_listen_for_consent_change', function (e) {
	if (e.detail && e.detail.statistics) {
		_paq.push([e.detail.statistics === 'allow' ? 'rememberCookieConsentGiven' : 'forgetCookieConsentGiven']);
	}
});
<?php foreach ( $calls as $call ) : ?>
_paq.push(<?php echo wp_json_encode( $call ); ?>);
<?php endforeach; ?>
_paq.push(['trackPageView']);
_paq.push(['enableLinkTracking']);
(function () {
	var s = document.createElement('script');
	s.async = true;
	s.src = '/m/a.js';
	document.head.appendChild(s);
})();
</script>
		<?php
	},
	100
);

function matomo_sku( WC_Product $product ): string {
	return $product->get_sku() ? $product->get_sku() : (string) $product->get_id();
}

function matomo_categories( WC_Product $product ): array {
	$id = $product->get_parent_id() ? $product->get_parent_id() : $product->get_id();
	$terms = get_the_terms( $id, 'product_cat' );
	// Matomo keeps up to five categories per product.
	return is_array( $terms ) ? array_slice( wp_list_pluck( $terms, 'name' ), 0, 5 ) : array();
}

// Matomo's cart is the whole cart at once: every line, then the total. The cart changes in
// requests that print no page (add to cart through AJAX), so the change is noted in the
// WooCommerce session and sent with the next page - which is never from the page cache while the
// cart holds something (default.vcl).
add_action(
	'woocommerce_after_calculate_totals',
	function () {
		if ( WC()->session ) {
			WC()->session->set( 'matomo_cart_changed', true );
		}
	}
);

function matomo_cart_calls(): array {
	if ( ! function_exists( 'WC' ) || ! WC()->session || ! WC()->session->get( 'matomo_cart_changed' ) || is_order_received_page() ) {
		return array();
	}
	WC()->session->set( 'matomo_cart_changed', false );
	$calls = array();
	foreach ( WC()->cart->get_cart() as $line ) {
		$product = $line['data'];
		$calls[] = array( 'addEcommerceItem', matomo_sku( $product ), $product->get_name(), matomo_categories( $product ), (float) wc_get_price_to_display( $product ), (int) $line['quantity'] );
	}
	$calls[] = array( 'trackEcommerceCartUpdate', (float) WC()->cart->get_total( 'edit' ) );
	return $calls;
}

// The visitor behind an order, noted when it is placed: their address and browser, and Matomo's
// visitor ID only if they agreed to cookies (without consent there is no _pk_id cookie).
function matomo_note_visitor( WC_Order $order ) {
	$order->update_meta_data( '_matomo_ip', sanitize_text_field( wp_unslash( $_SERVER['HTTP_X_REAL_IP'] ?? '' ) ) );
	$order->update_meta_data( '_matomo_ua', sanitize_text_field( wp_unslash( $_SERVER['HTTP_USER_AGENT'] ?? '' ) ) );
	foreach ( $_COOKIE as $name => $value ) {
		if ( str_starts_with( $name, '_pk_id_' . MATOMO_SITE_ID . '_' ) && preg_match( '/^[0-9a-f]{16}/', $value, $id ) ) {
			$order->update_meta_data( '_matomo_cid', $id[0] );
		}
	}
	$order->save();
}
add_action( 'woocommerce_checkout_order_created', 'matomo_note_visitor' );
add_action( 'woocommerce_store_api_checkout_order_processed', 'matomo_note_visitor' );

// The order, once paid, from the server: with a token of a Matomo user that may only write to this
// site (MATOMO_TOKEN, README "Web analytics"), Matomo takes the visitor's address from here instead
// of the shop's. Sent once per order.
function matomo_track_order( $order_id ) {
	$order = wc_get_order( $order_id );
	$token = getenv( 'MATOMO_TOKEN' );
	if ( ! $order || ! $token || $order->get_meta( '_matomo_tracked' ) ) {
		return;
	}
	$items = array();
	foreach ( $order->get_items() as $item ) {
		$product = $item->get_product();
		if ( $product ) {
			$items[] = array( matomo_sku( $product ), $product->get_name(), matomo_categories( $product ), (float) $order->get_item_subtotal( $item, true ), (int) $item->get_quantity() );
		}
	}
	$params   = array_filter(
		array(
			'idsite'     => MATOMO_SITE_ID,
			'rec'        => 1,
			'send_image' => 0,
			'url'        => $order->get_checkout_order_received_url() ? strtok( $order->get_checkout_order_received_url(), '?' ) : home_url(),
			'idgoal'     => 0,
			'ec_id'      => $order->get_order_number(),
			'revenue'    => (float) $order->get_total(),
			'ec_st'      => (float) $order->get_subtotal(),
			'ec_tx'      => (float) $order->get_total_tax(),
			'ec_sh'      => (float) $order->get_shipping_total(),
			'ec_dt'      => (float) $order->get_discount_total(),
			'ec_items'   => wp_json_encode( $items ),
			'cid'        => $order->get_meta( '_matomo_cid' ),
			'cip'        => $order->get_meta( '_matomo_ip' ),
			'ua'         => $order->get_meta( '_matomo_ua' ),
			'token_auth' => $token,
		),
		fn( $value ) => '' !== $value && null !== $value
	);
	$response = wp_remote_post(
		MATOMO_TRACKER,
		array(
			'headers' => array(
				'Host'              => MATOMO_HOST,
				'X-Forwarded-Proto' => 'https',
			),
			'body'    => $params,
			'timeout' => 5,
		)
	);
	if ( is_wp_error( $response ) || 200 !== wp_remote_retrieve_response_code( $response ) ) {
		// Not marked, so the next status change (completed after processing) tries again.
		error_log( 'Matomo: order ' . $order->get_order_number() . ' not tracked: ' . ( is_wp_error( $response ) ? $response->get_error_message() : wp_remote_retrieve_response_code( $response ) ) );
		return;
	}
	$order->update_meta_data( '_matomo_tracked', time() );
	$order->save();
}
add_action( 'woocommerce_order_status_processing', 'matomo_track_order' );
add_action( 'woocommerce_order_status_completed', 'matomo_track_order' );
