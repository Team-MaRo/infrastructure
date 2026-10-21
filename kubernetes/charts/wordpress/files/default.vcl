vcl 4.1;
# The page cache in front of nginx, in the same pod (README "Page cache"). It keeps what an
# anonymous visitor sees; everything personal - logged in, a cart, wp-admin, checkout, the REST
# API with MCP - goes straight through (pass). WordPress empties it on every save through Proxy
# Cache Purge, which sends PURGE to 127.0.0.1:8080 (files/page-cache.php).

import std;
import header;

backend nginx {
	.host = "127.0.0.1";
	.port = "8081";
	# Pages take seconds on a cold PHP (arepazo: up to 9 s measured).
	.first_byte_timeout = 60s;
	# nginx's own answer, not PHP's: a slow PHP must not mark the backend sick, which would
	# serve every visitor from grace or an error.
	.probe = {
		.url = "/healthz";
		# nginx answers it with 204 (files/default.conf); Varnish expects 200 otherwise.
		.expected_response = 204;
		.interval = 5s;
		.timeout = 2s;
		.window = 5;
		.threshold = 3;
	}
}

# Only WordPress in this pod may empty the cache; Traefik reaches Varnish through the pod's own
# address, so a PURGE from outside is refused.
acl purge {
	"127.0.0.1";
	"::1";
}

sub vcl_recv {
	if (req.method == "PURGE") {
		if (client.ip !~ purge) {
			return (synth(405, "Not allowed"));
		}
		# Proxy Cache Purge: an exact URL, or a pattern with X-Purge-Method: regex (`/.*` empties
		# the whole site).
		if (req.http.X-Purge-Method == "regex") {
			ban("obj.http.x-host == " + req.http.host + " && obj.http.x-url ~ " + req.url);
		} else {
			ban("obj.http.x-host == " + req.http.host + " && obj.http.x-url == " + req.url);
		}
		return (synth(200, "Purged"));
	}

	# A site's basic auth is Traefik's and already checked; WordPress needs nothing of it, and
	# a Bearer token (Claude's MCP) stays.
	if (req.http.Authorization ~ "^Basic ") {
		unset req.http.Authorization;
	}

	if (req.method != "GET" && req.method != "HEAD") {
		return (pass);
	}
	if (req.http.Authorization) {
		return (pass);
	}
	if (req.url ~ "^/(wp-admin|wp-login\.php|wp-cron\.php|wp-comments-post\.php|xmlrpc\.php|wp-json/|wc-api/|\?rest_route=)"
	    || req.url ~ "[?&](add-to-cart|wc-ajax|wc-api|preview|nocache|s)="
	    || req.url ~ "^/([a-z]{2}/)?(cart|checkout|my-account|warenkorb|kasse|mein-konto)(/|\?|$)") {
		return (pass);
	}
	# Logged in, a cart, a comment author, a protected post: their pages are their own.
	if (req.http.Cookie ~ "(wordpress_logged_in_|wordpress_sec_|wp-postpass_|comment_author_|woocommerce_items_in_cart|woocommerce_cart_hash|wp_woocommerce_session_)") {
		return (pass);
	}

	# Files nginx serves itself and Cloudflare caches - nothing to gain here.
	if (req.url ~ "^[^?]*\.(css|js|mjs|map|png|jpe?g|gif|webp|avif|svg|ico|woff2?|ttf|eot|otf|mp4|webm|mp3|pdf|zip)(\?.*)?$") {
		return (pass);
	}

	# One cached copy per page: tracking parameters out, the rest sorted; every other cookie
	# (analytics, Polylang's language - the language is in the URL) is of no use to the page.
	set req.url = regsuball(req.url, "([?&])(utm_[a-z]+|gclid|fbclid|msclkid|mc_cid|mc_eid|_ga)=[^&]*", "\1");
	set req.url = regsuball(req.url, "&+", "&");
	set req.url = regsub(req.url, "\?&", "?");
	set req.url = regsub(req.url, "[?&]$", "");
	set req.url = std.querysort(req.url);
	unset req.http.Cookie;
	# WordPress answers every page with Vary: accept, content-type; a browser's page request
	# becomes one form, so browsers do not each get a copy of their own.
	if (req.http.Accept ~ "text/html") {
		set req.http.Accept = "text/html";
	}
	unset req.http.Content-Type;
	return (hash);
}

sub vcl_backend_response {
	# For PURGE: bans match on these, so the ban lurker can work through them in the background.
	set beresp.http.x-url = bereq.url;
	set beresp.http.x-host = bereq.http.host;

	if (bereq.uncacheable) {
		return (deliver);
	}
	# On a page fetched for the cache (anonymous by now), the cookies WordPress sets for nobody in
	# particular: Polylang's language and WooCommerce's empty guest session. A real session starts
	# with the first cart action, which is never cached.
	header.remove(beresp.http.Set-Cookie, "^(pll_language|wp_woocommerce_session_[^=]*)=");

	if (beresp.http.Set-Cookie || beresp.http.Cache-Control ~ "(private|no-store|no-cache)" || beresp.status >= 300 && beresp.status != 301 && beresp.status != 404) {
		set beresp.uncacheable = true;
		set beresp.ttl = 120s;
		return (deliver);
	}
	# Six hours unless WordPress empties it sooner, then up to twelve more while a fresh copy is
	# fetched in the background - well under WPForms' 48-hour form token.
	set beresp.ttl = 6h;
	set beresp.grace = 12h;
	if (bereq.url ~ "sitemap[^/]*\.xml|\.xsl$|/feed/?($|\?)") {
		set beresp.ttl = 1h;
	}
	if (beresp.status == 404) {
		set beresp.ttl = 5m;
	}
	return (deliver);
}

sub vcl_backend_error {
	# A background refresh that fails keeps the stale copy instead of caching the error.
	if (bereq.is_bgfetch) {
		return (abandon);
	}
}

sub vcl_deliver {
	unset resp.http.x-url;
	unset resp.http.x-host;
	if (obj.hits > 0) {
		set resp.http.X-Cache = "HIT";
	} else {
		set resp.http.X-Cache = "MISS";
	}
}
