<?php
// Which version of a plugin or theme the init container `plugins` installs (README "Design"):
// wordpress.org's info answer on stdin, the version or range of the values as the argument.
// Prints the version, or names why there is none and exits 1.
//
//   (empty)            the release wordpress.org names
//   5, 5.12, 5.12.3    the newest release that is it or starts with it plus "."
//   ~5.12.3            the newest 5.12.x from 5.12.3 on
//   ^5.12.3            the newest 5.x from 5.12.3 on (^0.7.1: 0.7.x, as npm counts it)

$info    = json_decode( stream_get_contents( STDIN ) );
$range   = $argv[1] ?? '';
$release = (string) ( $info->version ?? '' );

if ( '' === $release ) {
	fwrite( STDERR, "wordpress.org names no release\n" );
	exit( 1 );
}
if ( '' === $range ) {
	echo $release;
	exit;
}

$operator = in_array( $range[0], array( '~', '^' ), true ) ? $range[0] : '';
$base     = ltrim( $range, '~^' );
$parts    = explode( '.', $base );
if ( '~' === $operator ) {
	$prefix = implode( '.', array_slice( $parts, 0, 2 ) );
} elseif ( '^' === $operator ) {
	$significant = count( $parts );
	foreach ( $parts as $i => $part ) {
		if ( '0' !== $part ) {
			$significant = $i + 1;
			break;
		}
	}
	$prefix = implode( '.', array_slice( $parts, 0, $significant ) );
} else {
	$prefix = $base;
}

// Tags newer than the release are not released yet; a letter marks a pre-release (16.4-beta).
$candidates = array_filter(
	array_map( 'strval', array_keys( (array) ( $info->versions ?? array() ) ) ),
	fn( $v ) => ! preg_match( '/[a-z]/i', $v )
		&& version_compare( $v, $release, '<=' )
		&& ( $v === $prefix || str_starts_with( $v, $prefix . '.' ) )
		&& ( '' === $operator || version_compare( $v, $base, '>=' ) )
);
if ( ! $candidates ) {
	fwrite( STDERR, "no release matches $range (release: $release)\n" );
	exit( 1 );
}
usort( $candidates, 'version_compare' );
echo end( $candidates );
