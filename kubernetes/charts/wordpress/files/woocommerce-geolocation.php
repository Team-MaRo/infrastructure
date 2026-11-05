<?php
/**
 * Plugin Name: WooCommerce geolocation in the pod
 * Description: Finds a visitor's country from Cloudflare's header or a country database in the pod, never from the uploads bucket or an outside service.
 */

// WooCommerce's own order (class-wc-geolocation.php): a proxy's country header, then its MaxMind
// integration - whose database it keeps under the uploads, so in the bucket: a request to S3 and a
// read of the file on every lookup, 0.3-0.6 s per request without the header (2026-11-05) - then
// ipinfo.io or api.country.is, handing the visitor's address to a third party. This answers first,
// which ends the search: Cloudflare's CF-IPCountry, else DB-IP's country database in the pod
// (templates/cronjob-geoip.yaml, copied at every start), else no country.
const GEOLOCATION_DATABASE = '/var/lib/geoip/dbip-country-lite.mmdb';

add_filter(
	'woocommerce_geolocate_ip',
	function ( $country, $ip_address ) {
		if ( false !== $country ) {
			return $country;
		}
		if ( '' === (string) $ip_address ) {
			// XX: Cloudflare does not know; T1: a Tor exit node.
			$header = strtoupper( (string) ( $_SERVER['HTTP_CF_IPCOUNTRY'] ?? '' ) );
			if ( 2 === strlen( $header ) && ! in_array( $header, array( 'XX', 'T1' ), true ) ) {
				return $header;
			}
			$ip_address = WC_Geolocation::get_ip_address();
		}
		return geolocation_from_database( (string) $ip_address );
	},
	10,
	2
);

function geolocation_from_database( string $ip_address ): string {
	static $reader = null;
	if ( '' === $ip_address || ! class_exists( 'MaxMind\Db\Reader' ) || ! is_readable( GEOLOCATION_DATABASE ) ) {
		return '';
	}
	try {
		$reader ??= new MaxMind\Db\Reader( GEOLOCATION_DATABASE );
		$record   = $reader->get( $ip_address );
	} catch ( Exception $e ) {
		return '';
	}
	return is_array( $record ) && isset( $record['country']['iso_code'] ) ? (string) $record['country']['iso_code'] : '';
}

// Without its integration WooCommerce no longer downloads MaxMind's database into the bucket every
// month, and no longer asks for a MaxMind licence key.
add_filter(
	'woocommerce_integrations',
	function ( $integrations ) {
		return array_values( array_diff( $integrations, array( 'WC_Integration_MaxMind_Geolocation' ) ) );
	}
);
