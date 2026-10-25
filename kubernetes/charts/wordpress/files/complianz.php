<?php
/**
 * Plugin Name: Complianz's files in the pod
 * Description: Keeps the files Complianz generates (its banner's CSS) in the pod instead of the uploads bucket.
 */

// Complianz writes its banner's CSS under the uploads and checks the file's age there on every page
// it builds - with S3-Uploads a request to the bucket per page, and one that hung kept the page
// waiting until nginx gave up (504, 2026-10-26). In wp-content/complianz/ (created at start) nginx
// serves it and Varnish keeps it by its ?ver=; after a restart Complianz rebuilds the missing file
// on the first page. Without Complianz nothing calls these filters.
add_filter( 'cmplz_upload_dir', fn() => WP_CONTENT_DIR );
add_filter( 'cmplz_upload_url', fn() => content_url() );

// A browser with a privacy signal (Global Privacy Control, Do Not Track) gets no banner and nothing
// optional - Complianz's "respect DNT/GPC" - but also no way to decide otherwise: it never sets its
// banner up, so its "Manage consent" button stays hidden and would open nothing. This shows the
// button and sets the banner up on its first click; an explicit choice then counts, since Complianz
// reads only the stored consent under a privacy signal (README "Complianz under a privacy signal"). It calls
// Complianz's global functions, no public interface - after an update, test the button with GPC.
add_action(
	'wp_footer',
	function () {
		if ( ! function_exists( 'cmplz_get_option' ) ) {
			return;
		}
		?>
<script>
document.addEventListener('cmplz_cookie_banner_data', function () {
	setTimeout(function () {
		if (typeof cmplz_do_not_track !== 'function' || !cmplz_do_not_track()) {
			return;
		}
		document.querySelectorAll('#cmplz-manage-consent .cmplz-manage-consent').forEach(function (button) {
			button.classList.remove('cmplz-hidden');
		});
	});
});
document.addEventListener('click', function (e) {
	if (e.target.closest('button.cmplz-manage-consent') && typeof cmplz_do_not_track === 'function' && cmplz_do_not_track()
		&& typeof show_cookie_banner === 'function' && !document.querySelector('.cmplz-cookiebanner:not(.cmplz-hidden)')) {
		show_cookie_banner();
	}
}, true);
</script>
		<?php
	},
	100
);
