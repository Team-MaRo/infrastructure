<?php
/**
 * Plugin Name: Page cache
 * Description: Empties the page cache where Proxy Cache Purge does not, and hides its add-on ads.
 */

// Proxy Cache Purge empties a page when it is saved, through wp-admin or Claude alike (README
// "Page cache"). A product's stock changes without a save, when an order takes the last one - so
// its page, the shop and its categories would keep "in stock" for hours. Here they are emptied on
// stock changes: the product (a variation shows on its parent's page), the shop page and every
// category it is listed in, with their parent categories, which list their subcategories'
// products too.
function page_cache_purge_product( $product ) {
	if ( ! class_exists( 'VarnishPurger' ) || ! $product instanceof WC_Product ) {
		return;
	}
	$id   = $product->get_parent_id() ? $product->get_parent_id() : $product->get_id();
	$urls = array( get_permalink( $id ) );
	if ( wc_get_page_id( 'shop' ) > 0 ) {
		$urls[] = get_permalink( wc_get_page_id( 'shop' ) );
	}
	foreach ( wc_get_product_term_ids( $id, 'product_cat' ) as $term_id ) {
		foreach ( array_merge( array( $term_id ), get_ancestors( $term_id, 'product_cat', 'taxonomy' ) ) as $category ) {
			$link = get_term_link( (int) $category, 'product_cat' );
			if ( ! is_wp_error( $link ) ) {
				$urls[] = $link;
			}
		}
	}
	foreach ( array_unique( array_filter( $urls ) ) as $url ) {
		VarnishPurger::purge_url( $url );
	}
}
add_action( 'woocommerce_product_set_stock', 'page_cache_purge_product' );
add_action( 'woocommerce_variation_set_stock', 'page_cache_purge_product' );
add_action(
	'woocommerce_product_set_stock_status',
	function ( $id, $status, $product ) {
		page_cache_purge_product( $product );
	},
	10,
	3
);

// A consent banner is in every cached page - its markup, its settings and the link to its CSS with
// a ?ver= - but its settings are no post, so saving them emptied nothing and visitors kept the old
// banner for hours (2026-10-26). Saving them empties the whole site, as the admin bar's "Purge
// Cache (All Pages)" does.
function page_cache_purge_all() {
	if ( class_exists( 'VarnishPurger' ) ) {
		VarnishPurger::purge_url( home_url( '/?vhp-regex' ) );
	}
}
// Complianz: once its banner's CSS is written - a purge before that kept the old ?ver= in the
// pages - but not for the preview wp-admin rebuilds while the banner is edited; and its other
// settings (consent categories, DNT/GPC), which change only what the page carries inline.
add_action(
	'cmplz_after_css_generation',
	function ( $file ) {
		if ( ! str_contains( basename( $file ), 'preview' ) ) {
			page_cache_purge_all();
		}
	}
);
add_action( 'cmplz_after_saved_fields', 'page_cache_purge_all' );
// WPConsent keeps all of its settings in one option.
add_action( 'add_option_wpconsent_settings', 'page_cache_purge_all' );
add_action( 'update_option_wpconsent_settings', 'page_cache_purge_all' );

// Proxy Cache Purge advertises its paid add-on in wp-admin notices until each user clicks them
// away, per site (it keeps that in the user's meta). Every user counts as having done so; nothing
// is stored. If a release renames the keys, the notice simply shows again.
add_filter(
	'get_user_metadata',
	function ( $value, $object_id, $meta_key ) {
		return in_array( $meta_key, array( 'vhp_dismissed_cacheability_notice', 'vhp_dismissed_woo_cache_notice' ), true ) ? true : $value;
	},
	10,
	3
);
