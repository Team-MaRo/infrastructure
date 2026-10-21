<?php
/**
 * Plugin Name: Page cache
 * Description: Empties the page cache where Proxy Cache Purge does not.
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
